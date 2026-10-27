package kubernetesinventory

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func validDrainInput(input Mutation) bool {
	return input.Operation == "drain" && input.Query.Type == "node" && input.ClusterWide && input.ID != "" && len(input.ID) <= 128 && input.Query.Name != "" && input.Query.UID != "" && input.ExpectedVersion != "" && input.Query.Namespace == "" && input.Query.NamespaceUID == "" && !input.Query.AllNamespaces && input.Query.Continue == "" && len(input.Object) == 0 && input.Replicas == nil && input.Rollback == nil && input.CronJobSource == nil && input.PrivateInputID == ""
}

func (c Client) drainNode(ctx context.Context, input Mutation, initial bool) (*unstructured.Unstructured, error) {
	if !validDrainInput(input) || c.Dynamic == nil {
		return nil, ErrMutationInvalid
	}
	node, err := c.Dynamic.Resource(drainNodes).Get(ctx, input.Query.Name, metav1.GetOptions{})
	if err != nil {
		return nil, apiError(err)
	}
	if string(node.GetUID()) != input.Query.UID || initial && node.GetResourceVersion() != input.ExpectedVersion {
		return nil, ErrMutationConflict
	}
	if !initial {
		cordoned, _, _ := unstructured.NestedBool(node.Object, "spec", "unschedulable")
		if !cordoned || node.GetAnnotations()[operationAnnotation] != input.ID {
			return nil, ErrMutationUnknown
		}
	}
	return node, nil
}

func cordonForDrain(node *unstructured.Unstructured, id string) *unstructured.Unstructured {
	desired := node.DeepCopy()
	_ = unstructured.SetNestedField(desired.Object, true, "spec", "unschedulable")
	annotations := desired.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[operationAnnotation] = id
	desired.SetAnnotations(annotations)
	return desired
}

func (c Client) mutateDrain(ctx context.Context, input Mutation, dryRun bool) (MutationResult, error) {
	result := MutationResult{DryRun: dryRun}
	node, err := c.drainNode(ctx, input, true)
	if err != nil {
		return result, err
	}
	current, objects, err := c.drainSnapshot(ctx, input.Query.Name)
	if err != nil {
		return result, err
	}
	if input.DrainPlan != nil {
		if err = validateDrainSubset(input.DrainPlan, current); err != nil {
			return result, err
		}
	} else if !dryRun {
		return result, ErrMutationInvalid
	} else {
		input.DrainPlan = current
	}
	progress, err := c.drainProgress(ctx, input, objects, false, true)
	if err != nil {
		return result, err
	}
	definition, _ := Lookup("node")
	before, err := Project(definition, *node, "", c.now())
	if err != nil {
		return result, err
	}
	desired := cordonForDrain(node, input.ID)
	validated, err := c.Dynamic.Resource(drainNodes).Update(ctx, desired, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}, FieldManager: "argus-workbench"})
	if err != nil {
		return result, mutationError(err)
	}
	after, err := Project(definition, *validated, "", c.now())
	if err != nil {
		return result, err
	}
	result.Before, result.After, result.Drain, result.DrainPlan = &before, &after, progress, input.DrainPlan
	result.Digest = drainHash([]any{stableObjectDigest(validated), input.DrainPlan})
	if dryRun {
		return result, nil
	}
	if input.ExpectedDigest != "" && input.ExpectedDigest != result.Digest {
		return result, ErrMutationConflict
	}
	for _, p := range input.DrainPlan.Pods {
		if p.Disposition == "local_storage" || p.Disposition == "unmanaged" {
			return result, ErrDrainUnsafe
		}
	}
	_, err = c.Dynamic.Resource(drainNodes).Update(ctx, desired, metav1.UpdateOptions{FieldManager: "argus-workbench"})
	if err != nil {
		failure := mutationError(err)
		if errors.Is(failure, ErrUnavailable) {
			failure = ErrMutationUnknown
		}
		return result, failure
	}
	// The node marker establishes that this drain began. Subsequent batches are
	// UID-bound eviction attempts, separately authorized by the control plane.
	return c.AdvanceDrain(ctx, input)
}

// AdvanceDrain is an explicit write phase. ReadMutationOutcome never calls it.
// Repeating a batch cannot evict a replacement Pod: every API request includes
// the live Pod's UID and resourceVersion and is restricted to the approved set.
func (c Client) AdvanceDrain(ctx context.Context, input Mutation) (MutationResult, error) {
	return c.readDrain(ctx, input, true)
}

func (c Client) readDrain(ctx context.Context, input Mutation, advance bool) (MutationResult, error) {
	result := MutationResult{}
	node, err := c.drainNode(ctx, input, false)
	if err != nil {
		return result, err
	}
	current, objects, err := c.drainSnapshot(ctx, input.Query.Name)
	if err != nil {
		return result, err
	}
	if err = validateDrainSubset(input.DrainPlan, current); err != nil {
		return result, err
	}
	progress, err := c.drainProgress(ctx, input, objects, advance, false)
	if err != nil {
		return result, err
	}
	definition, _ := Lookup("node")
	projected, err := Project(definition, *node, "", c.now())
	if err != nil {
		return result, err
	}
	result.Accepted, result.After, result.Drain = true, &projected, progress
	return result, nil
}

func (c Client) drainProgress(ctx context.Context, input Mutation, objects map[string]*unstructured.Unstructured, advance, dryRun bool) (*DrainProgress, error) {
	if input.DrainPlan == nil {
		return nil, ErrMutationInvalid
	}
	progress := &DrainProgress{Pods: []DrainPodProgress{}}
	attempts := 0
	for _, entry := range input.DrainPlan.Pods {
		if ctx.Err() != nil {
			return nil, ErrMutationUnknown
		}
		status := drainPublicPod(entry)
		pod := objects[entry.UID]
		if pod == nil {
			status.State = "gone"
		} else {
			switch entry.Disposition {
			case "daemonset", "mirror", "completed":
				status.State, status.Reason = "skipped", entry.Disposition
			case "local_storage", "unmanaged":
				status.State, status.Reason = "blocked", entry.Disposition
			case "evict":
				var err error
				status.Budgets, err = c.drainBudgets(ctx, pod)
				if err != nil {
					return nil, err
				}
				if pod.GetDeletionTimestamp() != nil {
					status.State = "terminating"
				} else if dryRun || advance && attempts < 20 {
					attempts++
					status.State, status.Reason, err = c.evictDrainPod(ctx, input, entry, dryRun)
					if err != nil {
						return nil, err
					}
				}
			default:
				return nil, ErrProtocol
			}
		}
		progress.Pods = append(progress.Pods, status)
	}
	drainPhase(progress)
	return progress, nil
}

func (c Client) evictDrainPod(ctx context.Context, input Mutation, entry DrainPod, dryRun bool) (string, string, error) {
	// Recheck the node and namespace immediately before each physical attempt,
	// including batches resumed after a lost response or a PDB denial.
	if _, err := c.drainNode(ctx, input, dryRun); err != nil {
		return "", "", err
	}
	if _, err := c.namespaceUID(ctx, entry.Namespace, entry.NamespaceUID); err != nil {
		return "", "", err
	}
	pod, err := c.Dynamic.Resource(drainPods).Namespace(entry.Namespace).Get(ctx, entry.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return "gone", "", nil
	}
	if err != nil {
		return "", "", apiError(err)
	}
	if string(pod.GetUID()) != entry.UID {
		return "", "", ErrMutationConflict
	}
	assigned, _, _ := unstructured.NestedString(pod.Object, "spec", "nodeName")
	if assigned != input.Query.Name {
		return "", "", ErrMutationConflict
	}
	if pod.GetDeletionTimestamp() != nil {
		return "terminating", "", nil
	}
	if drainHash([]any{pod.Object["spec"], pod.GetOwnerReferences(), pod.GetLabels(), pod.GetAnnotations()["kubernetes.io/config.mirror"]}) != entry.SpecDigest {
		return "", "", ErrMutationConflict
	}
	volumes, err := c.drainVolumes(ctx, pod)
	if err != nil {
		return "", "", err
	}
	if drainHash(volumes) != drainHash(entry.Volumes) {
		return "", "", ErrMutationConflict
	}
	options := map[string]any{"preconditions": map[string]any{"uid": entry.UID, "resourceVersion": pod.GetResourceVersion()}}
	create := metav1.CreateOptions{}
	if dryRun {
		options["dryRun"] = []any{metav1.DryRunAll}
		create.DryRun = []string{metav1.DryRunAll}
	}
	eviction := &unstructured.Unstructured{Object: map[string]any{"apiVersion": "policy/v1", "kind": "Eviction", "metadata": map[string]any{"name": entry.Name, "namespace": entry.Namespace}, "deleteOptions": options}}
	_, err = c.Dynamic.Resource(drainPods).Namespace(entry.Namespace).Create(ctx, eviction, create, "eviction")
	if apierrors.IsTooManyRequests(err) {
		return "blocked", "eviction_budget_or_throttle", nil
	}
	if apierrors.IsNotFound(err) {
		return "gone", "", nil
	}
	if err != nil {
		return "", "", mutationError(err)
	}
	if dryRun {
		return "pending", "", nil
	}
	return "eviction_requested", "", nil
}
