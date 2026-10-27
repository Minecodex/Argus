package kubernetesworkbench

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
)

const OperationQueue = "kubernetes_operation"

type MutationInput struct {
	ClusterID       uuid.UUID                                   `json:"cluster_id"`
	NamespaceID     uuid.UUID                                   `json:"namespace_id,omitempty"`
	Operation       string                                      `json:"operation"`
	Type            string                                      `json:"type"`
	Name            string                                      `json:"name"`
	UID             string                                      `json:"uid,omitempty"`
	ExpectedVersion string                                      `json:"expected_resource_version,omitempty"`
	Object          json.RawMessage                             `json:"object,omitempty"`
	Replicas        *int64                                      `json:"replicas,omitempty"`
	Rollback        *kubernetesinventory.RollbackReference      `json:"rollback,omitempty"`
	CronJobSource   *kubernetesinventory.ObjectVersionReference `json:"cronjob_source,omitempty"`
}
type mutationBackend interface {
	MutateInventory(context.Context, db.KubernetesCluster, kubernetesinventory.Mutation, bool) (kubernetesinventory.MutationResult, error)
	ObserveInventoryMutation(context.Context, db.KubernetesCluster, kubernetesinventory.Mutation, string) (kubernetesinventory.MutationResult, error)
}
type mutationPlan struct {
	SchemaVersion string                         `json:"schema_version"`
	ID            uuid.UUID                      `json:"id"`
	Actor         Actor                          `json:"actor"`
	Input         MutationInput                  `json:"input"`
	Mutation      kubernetesinventory.Mutation   `json:"mutation"`
	Scope         Scope                          `json:"scope"`
	Definition    kubernetesinventory.Definition `json:"definition"`
	DesiredDigest string                         `json:"desired_digest"`
}

func mutationSnapshot(plan mutationPlan) any {
	return struct {
		ID                 uuid.UUID
		Cluster            uuid.UUID
		ClusterVersion     int64
		ClusterWide        bool
		Policy             kubernetesinventory.MutationPolicy
		Namespace          uuid.UUID
		NamespaceUID       string
		Type, UID, Version string
		Rollback           *kubernetesinventory.RollbackReference
		CronJobSource      *kubernetesinventory.ObjectVersionReference
	}{plan.ID, plan.Input.ClusterID, plan.Scope.ClusterVersion, plan.Scope.ClusterWide, plan.Mutation.Policy, plan.Input.NamespaceID, plan.Mutation.Query.NamespaceUID, plan.Input.Type, plan.Input.UID, plan.Input.ExpectedVersion, plan.Input.Rollback, plan.Input.CronJobSource}
}
func (s Service) prepareMutation(ctx context.Context, a Actor, in MutationInput, id uuid.UUID, marker string) (mutationPlan, db.KubernetesCluster, error) {
	plan := mutationPlan{SchemaVersion: kubernetesinventory.MutationSchemaVersion, ID: id, Actor: a, Input: in}
	cluster, scope, err := s.scope(ctx, a, in.ClusterID)
	if err != nil {
		return plan, cluster, err
	}
	definition, err := s.definition(ctx, cluster, in.Type)
	if err != nil {
		return plan, cluster, err
	}
	if definition.ManagePermission == "" {
		return plan, cluster, kubernetesinventory.ErrForbidden
	}
	if err = authorization.Authorize(ctx, s.Store.Queries, a, definition.ManagePermission); err != nil {
		return plan, cluster, err
	}
	if in.Operation == "drain" {
		if in.Type != "node" || !scope.ClusterWide {
			return plan, cluster, authorization.ErrAccessDenied
		}
		if err = authorization.Authorize(ctx, s.Store.Queries, a, "kubernetes.node.drain"); err != nil {
			return plan, cluster, err
		}
	}
	query := kubernetesinventory.Query{Type: in.Type, Name: in.Name, UID: in.UID}
	if definition.Namespaced {
		if in.NamespaceID == uuid.Nil {
			return plan, cluster, kubernetesinventory.ErrInvalid
		}
		namespaces, _, err := s.selectedNamespaces(ctx, a, scope, Selection{Mode: "explicit", IDs: []uuid.UUID{in.NamespaceID}})
		if err != nil {
			return plan, cluster, err
		}
		query.Namespace, query.NamespaceUID = namespaces[0].Name, namespaces[0].UID
	} else if !scope.ClusterWide || in.NamespaceID != uuid.Nil {
		return plan, cluster, authorization.ErrAccessDenied
	}
	plan.Scope, plan.Definition = scope, definition
	plan.Mutation = kubernetesinventory.Mutation{OperationRef: id.String(), ID: marker, Operation: in.Operation, Query: query, ExpectedVersion: in.ExpectedVersion, Object: in.Object, Replicas: in.Replicas, Rollback: in.Rollback, CronJobSource: in.CronJobSource, ClusterWide: scope.ClusterWide}
	plan.Mutation.Policy, err = mutationPermissions(ctx, s.Store.Queries, a, scope.ClusterWide)
	if err != nil {
		return plan, cluster, err
	}
	return plan, cluster, nil
}
func previewManifest(object *kubernetesinventory.Object) any {
	if object == nil {
		return nil
	}
	var value map[string]any
	if json.Unmarshal(object.Data, &value) != nil {
		return nil
	}
	if metadata, ok := value["metadata"].(map[string]any); ok {
		for _, key := range []string{"uid", "resourceVersion", "generation", "creationTimestamp", "managedFields"} {
			delete(metadata, key)
		}
		if annotations, ok := metadata["annotations"].(map[string]any); ok {
			delete(annotations, "argus.io/operation-id")
		}
	}
	delete(value, "status")
	return value
}
func (s Service) PreviewMutation(ctx context.Context, a Actor, in MutationInput, key string) (db.PendingAction, error) {
	return s.PreviewMutationWithSecretValues(ctx, a, in, nil, key)
}
func (s Service) PreviewMutationWithSecretValues(ctx context.Context, a Actor, in MutationInput, values []byte, key string) (db.PendingAction, error) {
	if s.Store == nil || len(s.Actions.Key) != 32 || key == "" || len(key) > 200 {
		return db.PendingAction{}, kubernetesinventory.ErrInvalid
	}
	backend, ok := s.Backend.(mutationBackend)
	if !ok {
		return db.PendingAction{}, kubernetesinventory.ErrProtocol
	}
	privateKeys := []string{}
	if len(values) > 0 {
		if in.Type != "secret" || in.Operation != "create" && in.Operation != "patch" {
			return db.PendingAction{}, kubernetesinventory.ErrMutationInvalid
		}
		var err error
		privateKeys, err = kubernetesinventory.SecretValueKeys(values)
		if err != nil {
			return db.PendingAction{}, err
		}
	}
	// Cleartext credentials may only enter through the separate protected input.
	if len(in.Object) > 0 {
		var value map[string]any
		if json.Unmarshal(in.Object, &value) != nil {
			return db.PendingAction{}, kubernetesinventory.ErrInvalid
		}
		_, sensitive, err := datapolicy.Project(value)
		if err != nil || sensitive {
			return db.PendingAction{}, kubernetesinventory.ErrMutationInvalid
		}
		if in.Type == "secret" {
			if value["stringData"] != nil {
				return db.PendingAction{}, kubernetesinventory.ErrMutationInvalid
			}
			data, ok := value["data"].(map[string]any)
			if value["data"] != nil && !ok {
				return db.PendingAction{}, kubernetesinventory.ErrMutationInvalid
			}
			for _, item := range data {
				if item != nil && item != datapolicy.Marker {
					return db.PendingAction{}, kubernetesinventory.ErrMutationInvalid
				}
			}
			if len(privateKeys) > 0 {
				if data == nil {
					data = map[string]any{}
				}
				for _, name := range privateKeys {
					data[name] = datapolicy.Marker
				}
				value["data"] = data
				in.Object, _ = json.Marshal(value)
			}
		}
	}
	id := uuid.NewSHA1(a.EnterpriseID, []byte("kubernetes:"+a.SubjectType+":"+a.SubjectID.String()+":"+key))
	mac := hmac.New(sha256.New, s.Actions.Key)
	mac.Write([]byte("kubernetes-operation-marker/v1:" + id.String()))
	marker := hex.EncodeToString(mac.Sum(nil))
	plan, cluster, err := s.prepareMutation(ctx, a, in, id, marker)
	if err != nil {
		return db.PendingAction{}, err
	}
	if in.Operation == "restart" {
		plan.Mutation.RestartAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	request := mutationIdempotencyRequest(s.Actions.Key, in, values)
	requestJSON, _ := json.Marshal(request)
	replay, err := s.Actions.Idempotency.Lookup(ctx, s.Store.Queries, "enterprise", a.SubjectType+":"+a.SubjectID.String(), "kubernetes.object.preview", key, requestJSON)
	if err != nil {
		return db.PendingAction{}, err
	}
	if replay.Replay {
		var action db.PendingAction
		err = json.Unmarshal(replay.Body, &action)
		return action, err
	}
	privateID := uuid.Nil
	bound := false
	if len(privateKeys) > 0 {
		if s.Credentials.Store == nil {
			return db.PendingAction{}, kubernetesinventory.ErrUnavailable
		}
		privateID = uuid.New()
		plan.Mutation.PrivateInputID = privateID.String()
		if err = s.Credentials.StageKubernetesInput(ctx, a, in.ClusterID, in.NamespaceID, plan.Mutation, values); err != nil {
			return db.PendingAction{}, err
		}
		defer func() {
			if !bound {
				cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				_ = s.Store.Queries.DeleteKubernetesPrivateInput(cleanup, db.DeleteKubernetesPrivateInputParams{ID: privateID, EnterpriseID: a.EnterpriseID})
			}
		}()
	}
	validation, err := backend.MutateInventory(ctx, cluster, plan.Mutation, true)
	if err != nil {
		return db.PendingAction{}, err
	}
	if !validation.DryRun || validation.Accepted {
		return db.PendingAction{}, kubernetesinventory.ErrProtocol
	}
	if err = kubernetesinventory.ValidateMutationResult(plan.Definition, validation, plan.Scope.ClusterWide, plan.Mutation.Policy); err != nil {
		return db.PendingAction{}, err
	}
	plan.DesiredDigest = validation.Digest
	if in.Operation == "drain" {
		if validation.DrainPlan == nil || validation.Drain == nil {
			return db.PendingAction{}, kubernetesinventory.ErrProtocol
		}
		plan.Mutation.DrainPlan = validation.DrainPlan
	}
	// Private values were staged before dry-run and remain bound to that exact
	// input. Their admission projection deliberately has no content fingerprint.
	if plan.Mutation.PrivateInputID == "" {
		plan.Mutation.ExpectedDigest = validation.Digest
	}
	diff, err := PublicMutationDiff(validation)
	if err != nil {
		return db.PendingAction{}, err
	}
	for _, name := range privateKeys {
		diff = append(diff, DiffLine{Kind: "change", Text: "data/" + name + " = " + datapolicy.Marker})
	}
	if len(diff) > 256 {
		return db.PendingAction{}, kubernetesinventory.ErrBudget
	}
	action, resultErr := postgres.ExecuteIdempotent(ctx, s.Store, s.Actions.Idempotency, "enterprise", a.SubjectType+":"+a.SubjectID.String(), "kubernetes.object.preview", key, request, 201, func(q *db.Queries) (db.PendingAction, error) {
		if err := authorization.Authorize(ctx, q, a, plan.Definition.ManagePermission); err != nil {
			return db.PendingAction{}, err
		}
		current, _, err := s.prepareMutation(ctx, a, in, id, marker)
		if err != nil {
			return db.PendingAction{}, err
		}
		if current.Scope.ClusterVersion != plan.Scope.ClusterVersion || current.Mutation.Query.NamespaceUID != plan.Mutation.Query.NamespaceUID {
			return db.PendingAction{}, resource.ErrActionInvalidated
		}
		input := resource.PrepareActionInput{ActionType: "kubernetes.object." + in.Operation, Title: in.Operation + " " + definitionTitle(plan.Definition, in.Name), Summary: "Validated Kubernetes object change", Risk: "write", ResourceType: "kubernetes_cluster", ResourceID: uuid.NullUUID{UUID: in.ClusterID, Valid: true}, AuthorizationVersion: a.AuthorizationVersion, RunID: uuid.NullUUID{UUID: a.RunID, Valid: a.RunID != uuid.Nil}, Preview: map[string]any{"operation": in.Operation, "cluster_id": in.ClusterID, "namespace": plan.Mutation.Query.Namespace, "namespace_uid": plan.Mutation.Query.NamespaceUID, "name": in.Name, "type": in.Type, "uid": in.UID, "resource_version": in.ExpectedVersion, "dry_run": true}, Diff: diff, ImmutablePlan: plan, ResourceScopeSnapshot: mutationSnapshot(plan), CommitHandler: "kubernetes.object.commit"}
		if in.Replicas != nil {
			input.Preview.(map[string]any)["replicas"] = *in.Replicas
		}
		if in.Operation == "delete" || in.Operation == "drain" {
			input.Risk = "dangerous"
		}
		if validation.Drain != nil {
			input.Preview.(map[string]any)["drain"] = validation.Drain
		}
		if in.Rollback != nil {
			input.Preview.(map[string]any)["rollback"] = in.Rollback
		}
		if in.CronJobSource != nil {
			input.Preview.(map[string]any)["cronjob_source"] = in.CronJobSource
		}
		action, err := s.Actions.PrepareWithQueries(ctx, q, resource.ActionSubject{ID: a.SubjectID.String(), Type: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ConfirmationRequired: true}, a.EnterpriseID, input)
		if err != nil {
			return action, err
		}
		raw, _ := json.Marshal(plan)
		_, err = q.CreateKubernetesOperation(ctx, db.CreateKubernetesOperationParams{ID: id, EnterpriseID: a.EnterpriseID, ClusterID: in.ClusterID, NamespaceID: uuid.NullUUID{UUID: in.NamespaceID, Valid: in.NamespaceID != uuid.Nil}, NamespaceUid: plan.Mutation.Query.NamespaceUID, ActorID: a.SubjectID, ActorType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ClusterVersion: plan.Scope.ClusterVersion, PendingActionID: uuid.NullUUID{UUID: action.ID, Valid: true}, Operation: in.Operation, TypeKey: in.Type, TargetName: in.Name, TargetUid: in.UID, ExpectedResourceVersion: in.ExpectedVersion, PrivatePlan: raw})
		if err == nil && privateID != uuid.Nil {
			var count int64
			count, err = q.BindKubernetesPrivateInput(ctx, db.BindKubernetesPrivateInputParams{ID: privateID, EnterpriseID: a.EnterpriseID, OperationID: id, ActorID: a.SubjectID})
			if err == nil && count != 1 {
				err = resource.ErrActionInvalidated
			}
		}
		return action, err
	})
	if resultErr == nil && privateID != uuid.Nil {
		row, e := s.Store.Queries.GetKubernetesPrivateInput(ctx, db.GetKubernetesPrivateInputParams{ID: privateID, EnterpriseID: a.EnterpriseID})
		bound = e == nil && row.Status == "bound"
	}
	return action, resultErr
}
func definitionTitle(d kubernetesinventory.Definition, name string) string {
	return d.Kind + "/" + name
}
func mutationIdempotencyRequest(key []byte, in MutationInput, values []byte) any {
	privateHash := ""
	if len(values) > 0 {
		mac := hmac.New(sha256.New, key)
		mac.Write([]byte("kubernetes-private-request/v1\x00"))
		mac.Write(values)
		privateHash = hex.EncodeToString(mac.Sum(nil))
	}
	return struct {
		Input       MutationInput
		PrivateHash string
	}{in, privateHash}
}

type ActionExtension struct {
	Service Service
	Next    resource.ActionExtension
}

func (e ActionExtension) RevalidateAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) ([]byte, error) {
	if action.ActionType == drainCancelAction {
		return e.revalidateDrainCancel(ctx, q, action, raw)
	}
	if strings.HasPrefix(action.ActionType, "kubernetes.helm.") {
		return e.revalidateHelm(ctx, q, action, raw)
	}
	if !strings.HasPrefix(action.ActionType, "kubernetes.object.") {
		if e.Next != nil {
			return e.Next.RevalidateAction(ctx, q, action, raw)
		}
		return nil, resource.ErrActionInvalidated
	}
	var plan mutationPlan
	if json.Unmarshal(raw, &plan) != nil || plan.SchemaVersion != kubernetesinventory.MutationSchemaVersion || plan.Actor.EnterpriseID != action.EnterpriseID || plan.Actor.SubjectID != action.CreatorSubjectID || plan.Actor.SubjectType != action.CreatorSubjectType || action.ActionType != "kubernetes.object."+plan.Input.Operation {
		return nil, resource.ErrActionInvalidated
	}
	current, _, err := e.Service.prepareMutation(ctx, plan.Actor, plan.Input, plan.ID, plan.Mutation.ID)
	if err != nil {
		return nil, err
	}
	if current.Definition != plan.Definition {
		return nil, resource.ErrActionInvalidated
	}
	if err := checkMutationPermissions(ctx, q, plan.Actor, plan.Scope.ClusterWide, plan.Mutation.Policy); err != nil {
		return nil, err
	}
	value, err := json.Marshal(mutationSnapshot(current))
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(value)
	return hash[:], nil
}
func (e ActionExtension) CommitAction(ctx context.Context, q *db.Queries, action db.PendingAction, raw json.RawMessage) (resource.ActionCommitResult, error) {
	if action.ActionType == drainCancelAction {
		return e.commitDrainCancel(ctx, q, action, raw)
	}
	if strings.HasPrefix(action.ActionType, "kubernetes.helm.") {
		return e.commitHelm(ctx, q, action, raw)
	}
	if !strings.HasPrefix(action.ActionType, "kubernetes.object.") {
		if e.Next != nil {
			return e.Next.CommitAction(ctx, q, action, raw)
		}
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	var plan mutationPlan
	if json.Unmarshal(raw, &plan) != nil {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	count, err := q.QueueKubernetesOperation(ctx, db.QueueKubernetesOperationParams{EnterpriseID: action.EnterpriseID, ID: plan.ID})
	if err != nil {
		return resource.ActionCommitResult{}, err
	}
	if count != 1 {
		return resource.ActionCommitResult{}, resource.ErrActionInvalidated
	}
	payload, _ := json.Marshal(map[string]uuid.UUID{"enterprise_id": action.EnterpriseID, "operation_id": plan.ID})
	_, err = q.CreateRuntimeTask(ctx, db.CreateRuntimeTaskParams{ID: uuid.New(), EnterpriseID: uuid.NullUUID{UUID: action.EnterpriseID, Valid: true}, Queue: OperationQueue, RunID: action.RunID, Payload: payload, MaxAttempts: 1, AvailableAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	return resource.ActionCommitResult{ResourceType: "kubernetes_operation", ResourceID: plan.ID, Summary: "Kubernetes operation queued", KubernetesOperationID: uuid.NullUUID{UUID: plan.ID, Valid: true}}, err
}
