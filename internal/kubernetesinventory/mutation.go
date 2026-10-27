package kubernetesinventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var ErrMutationConflict = errors.New("KUBERNETES_MUTATION_CONFLICT")
var ErrMutationInvalid = errors.New("KUBERNETES_MUTATION_INVALID")
var ErrMutationUnknown = errors.New("KUBERNETES_MUTATION_OUTCOME_UNKNOWN")

const operationAnnotation = "argus.io/operation-id"
const MutationSchemaVersion = "argus.kubernetes_mutation/v6"

// Policy is frozen by the control plane, never accepted from a portal form.
// A resource grant describes scope; it is not a privilege escalation permit.
type MutationPolicy struct {
	PrivilegedWorkloads bool `json:"privileged_workloads"`
	ServiceAccountUse   bool `json:"service_account_use"`
	RBACDelegation      bool `json:"rbac_delegation"`
}

type Mutation struct {
	ExpectedDigest  string                  `json:"expected_digest,omitempty"`
	Policy          MutationPolicy          `json:"policy"`
	PrivateInputID  string                  `json:"private_input_id,omitempty"`
	OperationRef    string                  `json:"operation_ref"`
	ID              string                  `json:"id"`
	Operation       string                  `json:"operation"`
	Query           Query                   `json:"query"`
	ExpectedVersion string                  `json:"expected_version"`
	Object          json.RawMessage         `json:"object,omitempty"`
	Replicas        *int64                  `json:"replicas,omitempty"`
	RestartAt       string                  `json:"restart_at,omitempty"`
	Rollback        *RollbackReference      `json:"rollback,omitempty"`
	CronJobSource   *ObjectVersionReference `json:"cronjob_source,omitempty"`
	ClusterWide     bool                    `json:"cluster_wide"`
	DrainPlan       *DrainPlan              `json:"drain_plan,omitempty"`
}
type MutationResult struct {
	Accepted        bool           `json:"accepted"`
	DeleteRequested bool           `json:"delete_requested"`
	Before          *Object        `json:"before,omitempty"`
	After           *Object        `json:"after,omitempty"`
	Digest          string         `json:"digest"`
	Deleted         bool           `json:"deleted"`
	DryRun          bool           `json:"dry_run"`
	DrainPlan       *DrainPlan     `json:"drain_plan,omitempty"`
	Drain           *DrainProgress `json:"drain,omitempty"`
}

func mutationError(err error) error {
	if err == nil {
		return nil
	}
	if apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err) {
		return ErrMutationConflict
	}
	if apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) {
		return ErrMutationInvalid
	}
	return apiError(err)
}
func stableObjectDigest(value *unstructured.Unstructured) string {
	copy := value.DeepCopy()
	normalizeGeneratedJobIdentity(copy)
	delete(copy.Object, "status")
	metadata, _, _ := unstructured.NestedMap(copy.Object, "metadata")
	for _, key := range []string{"uid", "resourceVersion", "generation", "creationTimestamp", "managedFields", "selfLink"} {
		delete(metadata, key)
	}
	_ = unstructured.SetNestedMap(copy.Object, metadata, "metadata")
	raw, _ := json.Marshal(copy.Object)
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func StableDesiredDigest(value, desired *unstructured.Unstructured, kind string, create bool) string {
	raw, _ := json.Marshal(desired.Object)
	operation := "patch"
	if create {
		operation = "create"
	}
	return stableMutationDigest(value, Mutation{Object: raw, Operation: operation, Query: Query{Type: kind}})
}

// Mutate validates with the same server-side request used for execution. Raw
// objects and their digest stay in the execution/control plane; public preview
// DTOs expose only the projected before/after views.
func (c Client) Mutate(ctx context.Context, input Mutation, dryRun bool) (MutationResult, error) {
	if input.Operation == "drain" {
		return c.mutateDrain(ctx, input, dryRun)
	}
	if input.DrainPlan != nil {
		return MutationResult{}, ErrMutationInvalid
	}
	if !dryRun {
		preflight := input
		preflight.ExpectedDigest = ""
		validation, err := c.Mutate(ctx, preflight, true)
		if err != nil {
			return MutationResult{}, err
		}
		if input.ExpectedDigest != "" && validation.Digest != input.ExpectedDigest {
			return MutationResult{}, ErrMutationConflict
		}
	}
	result := MutationResult{DryRun: dryRun}
	if input.CronJobSource != nil && (input.Operation != "create" || input.Query.Type != "job" || len(input.Object) > 0 || input.Rollback != nil || input.Replicas != nil) {
		return result, ErrMutationInvalid
	}
	if input.ID == "" || len(input.ID) > 128 || input.Query.Name == "" || input.Query.AllNamespaces || input.Query.Continue != "" {
		return result, ErrMutationInvalid
	}
	definition, source, namespaceUID, err := c.scope(ctx, input.Query)
	if err != nil {
		return result, err
	}
	if definition.ManagePermission == "" {
		return result, ErrUnsupported
	}
	if !definition.Namespaced && !input.ClusterWide {
		return result, ErrForbidden
	}
	var before *unstructured.Unstructured
	if input.Operation != "create" {
		if input.Query.UID == "" || input.ExpectedVersion == "" {
			return result, ErrMutationInvalid
		}
		before, err = source.Get(ctx, input.Query.Name, metav1.GetOptions{})
		if err != nil {
			return result, mutationError(err)
		}
		if string(before.GetUID()) != input.Query.UID || before.GetResourceVersion() != input.ExpectedVersion {
			return result, ErrMutationConflict
		}
		projected, e := Project(definition, *before, namespaceUID, c.now())
		if e != nil {
			return result, e
		}
		result.Before = &projected
	} else if input.Query.UID != "" || input.ExpectedVersion != "" {
		return result, ErrMutationInvalid
	}
	options := []string{}
	if dryRun {
		options = []string{metav1.DryRunAll}
	}
	if input.Operation == "delete" {
		if err = c.CheckDeletionDependencies(ctx, definition, before); err != nil {
			return result, err
		}
		uid, version := before.GetUID(), before.GetResourceVersion()
		policy := metav1.DeletePropagationForeground
		err = source.Delete(ctx, input.Query.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version}, DryRun: options, PropagationPolicy: &policy})
		if err != nil {
			failure := mutationError(err)
			if !dryRun && errors.Is(failure, ErrUnavailable) {
				failure = ErrMutationUnknown
			}
			return result, failure
		}
		result.DeleteRequested = true
		result.Accepted = !dryRun
		return result, nil
	}
	var desired *unstructured.Unstructured
	if input.Operation == "rollback" {
		desired, err = c.rollbackObject(ctx, input, definition, before)
		if err == nil {
			desired, err = finalizeMutationObject(input, definition, before, desired)
		}
	} else {
		if input.Rollback != nil {
			return result, ErrMutationInvalid
		}
		resolved := input
		if input.CronJobSource != nil {
			var job *unstructured.Unstructured
			job, err = c.jobFromCronJob(ctx, input)
			if err != nil {
				return result, err
			}
			resolved.Object, err = job.MarshalJSON()
			if err != nil {
				return result, ErrMutationInvalid
			}
		}
		desired, err = mutationObject(resolved, definition, before)
	}
	if err != nil {
		return result, err
	}
	if before == nil {
		defaultWorkloadIdentity(definition, desired, input.Policy)
	}
	if definition.Key == "pvc" {
		if err = validatePVCRequestedMetadata(before, desired); err != nil {
			return result, err
		}
	}
	if err = c.validatePVCResize(ctx, definition, before, desired); err != nil {
		return result, err
	}
	if err = validateMutationPolicy(definition, desired, input.ClusterWide, input.Policy); err != nil {
		return result, err
	}
	if input.Query.Namespace != "" {
		if _, err = c.namespaceUID(ctx, input.Query.Namespace, namespaceUID); err != nil {
			return result, err
		}
	}
	var after *unstructured.Unstructured
	if input.Operation == "create" {
		after, err = source.Create(ctx, desired, metav1.CreateOptions{DryRun: options, FieldManager: "argus-workbench"})
	} else {
		after, err = source.Update(ctx, desired, metav1.UpdateOptions{DryRun: options, FieldManager: "argus-workbench"})
	}
	if err != nil {
		failure := mutationError(err)
		if !dryRun && errors.Is(failure, ErrUnavailable) {
			failure = ErrMutationUnknown
		}
		return result, failure
	}
	if err = validateMutationPolicy(definition, after, input.ClusterWide, input.Policy); err != nil {
		if !dryRun {
			return result, ErrMutationUnknown
		}
		return result, err
	}
	if err = c.validatePVCResize(ctx, definition, before, after); err != nil {
		if !dryRun {
			return result, ErrMutationUnknown
		}
		return result, err
	}
	result.Accepted = !dryRun
	result.Digest = stableMutationDigest(after, input)
	projected, err := projectObject(definition, *after, namespaceUID, c.now(), !dryRun)
	if err != nil {
		if !dryRun {
			return result, ErrMutationUnknown
		}
		return result, err
	}
	result.After = &projected
	if projected.Redacted {
		result.Digest = ""
	}
	return result, nil
}

func mutationObject(input Mutation, definition Definition, before *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var desired *unstructured.Unstructured
	switch input.Operation {
	case "create":
		desired = &unstructured.Unstructured{}
		if desired.UnmarshalJSON(input.Object) != nil {
			return nil, ErrMutationInvalid
		}
	case "patch":
		if before == nil || len(input.Object) == 0 {
			return nil, ErrMutationInvalid
		}
		raw, _ := before.MarshalJSON()
		merged, err := jsonpatch.MergePatch(raw, input.Object)
		if err != nil {
			return nil, ErrMutationInvalid
		}
		desired = &unstructured.Unstructured{}
		if desired.UnmarshalJSON(merged) != nil {
			return nil, ErrMutationInvalid
		}
	case "scale":
		if before == nil || input.Replicas == nil || *input.Replicas < 0 || *input.Replicas > 10000 || definition.Key != "deployment" && definition.Key != "statefulset" && definition.Key != "replicaset" {
			return nil, ErrMutationInvalid
		}
		desired = before.DeepCopy()
		_ = unstructured.SetNestedField(desired.Object, *input.Replicas, "spec", "replicas")
	case "restart":
		if before == nil || definition.Key != "deployment" && definition.Key != "statefulset" && definition.Key != "daemonset" {
			return nil, ErrMutationInvalid
		}
		at, err := time.Parse(time.RFC3339Nano, input.RestartAt)
		if err != nil || at.IsZero() {
			return nil, ErrMutationInvalid
		}
		desired = before.DeepCopy()
		annotations, _, _ := unstructured.NestedStringMap(desired.Object, "spec", "template", "metadata", "annotations")
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations["kubectl.kubernetes.io/restartedAt"] = at.UTC().Format(time.RFC3339Nano)
		_ = unstructured.SetNestedStringMap(desired.Object, annotations, "spec", "template", "metadata", "annotations")
	case "cordon", "uncordon":
		if before == nil || definition.Key != "node" || !input.ClusterWide {
			return nil, ErrForbidden
		}
		desired = before.DeepCopy()
		_ = unstructured.SetNestedField(desired.Object, input.Operation == "cordon", "spec", "unschedulable")
	default:
		return nil, ErrUnsupported
	}
	return finalizeMutationObject(input, definition, before, desired)
}

func finalizeMutationObject(input Mutation, definition Definition, before, desired *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	apiVersion := definition.Version
	if definition.Group != "" {
		apiVersion = definition.Group + "/" + definition.Version
	}
	if desired.GetName() != input.Query.Name || desired.GetNamespace() != input.Query.Namespace || desired.GetKind() != definition.Kind || desired.GetAPIVersion() != apiVersion {
		return nil, ErrMutationInvalid
	}
	delete(desired.Object, "status")
	desired.SetManagedFields(nil)
	if before != nil {
		if desired.GetUID() != before.GetUID() || desired.GetResourceVersion() != before.GetResourceVersion() {
			return nil, ErrMutationConflict
		}
		if err := restoreRedactedFields(desired.Object, before.Object); err != nil {
			return nil, err
		}
	} else if desired.GetUID() != "" || desired.GetResourceVersion() != "" {
		return nil, ErrMutationInvalid
	}
	annotations := desired.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[operationAnnotation] = input.ID
	desired.SetAnnotations(annotations)
	return desired, nil
}

// ReadMutationOutcome settles a lost response by observation only. The caller
// must not blindly repeat a create/update after an unknown transport outcome.
func (c Client) ReadMutationOutcome(ctx context.Context, input Mutation, expectedDigest string) (MutationResult, error) {
	if input.Operation == "drain" {
		return c.readDrain(ctx, input, false)
	}
	result := MutationResult{}
	definition, source, namespaceUID, err := c.scope(ctx, input.Query)
	if err != nil {
		return result, err
	}
	value, err := source.Get(ctx, input.Query.Name, metav1.GetOptions{})
	if input.Operation == "delete" && (apierrors.IsNotFound(err) || err == nil && string(value.GetUID()) != input.Query.UID) {
		result.Deleted = true
		return result, nil
	}
	if err != nil {
		return result, ErrMutationUnknown
	}
	if value.GetAnnotations()[operationAnnotation] != input.ID || expectedDigest != "" && stableMutationDigest(value, input) != expectedDigest {
		return result, ErrMutationUnknown
	}
	result.Digest = stableMutationDigest(value, input)
	projected, err := Project(definition, *value, namespaceUID, c.now())
	if err != nil {
		return result, err
	}
	result.After = &projected
	if projected.Redacted {
		result.Digest = ""
	}
	return result, nil
}
