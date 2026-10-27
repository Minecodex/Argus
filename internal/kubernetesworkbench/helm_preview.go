package kubernetesworkbench

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (s Service) PreviewHelm(ctx context.Context, a Actor, clusterID uuid.UUID, in HelmInput, chart, values []byte, key string) (db.PendingAction, error) {
	return s.PreviewHelmProtected(ctx, a, clusterID, in, kuberneteshelm.Material{Chart: chart, Values: values}, key)
}

func (s Service) PreviewHelmProtected(ctx context.Context, a Actor, clusterID uuid.UUID, in HelmInput, material kuberneteshelm.Material, key string) (db.PendingAction, error) {
	if in.Source != nil {
		clone := *in.Source
		in.Source = &clone
	}
	if material.SourceCredentials != nil {
		clone := *material.SourceCredentials
		material.SourceCredentials = &clone
		defer material.SourceCredentials.Clear()
	}
	var empty db.PendingAction
	if s.Store == nil || len(s.Actions.Key) != 32 || len(material.Key) != 0 || key == "" || len(key) > 200 || len(material.Chart) > kuberneteshelm.MaxArchiveBytes || len(material.Values) > kuberneteshelm.MaxValuesBytes || !slices.Contains([]string{"install", "upgrade", "rollback", "uninstall"}, in.Operation) || in.Reason == "" || len([]rune(in.Reason)) > 500 || kuberneteshelm.ValidateChartSource(in.Source) != nil || kuberneteshelm.ValidateSourceCredentials(material.SourceCredentials) != nil {
		return empty, kuberneteshelm.ErrInvalid
	}
	if in.Operation == "install" || in.Operation == "upgrade" {
		if (in.Source == nil) == (len(material.Chart) == 0) || in.Source == nil && material.SourceCredentials != nil {
			return empty, kuberneteshelm.ErrInvalid
		}
	} else if len(material.Chart) > 0 || len(material.Values) > 0 || in.Source != nil || material.SourceCredentials != nil {
		return empty, kuberneteshelm.ErrInvalid
	}
	backend, ok := s.Backend.(helmBackend)
	if !ok || s.Credentials.Store == nil {
		return empty, kuberneteshelm.ErrUnavailable
	}
	id := uuid.NewSHA1(a.EnterpriseID, []byte("helm:"+a.SubjectType+":"+a.SubjectID.String()+":"+key))
	plan, cluster, e := s.prepareHelm(ctx, a, clusterID, in, id, true)
	if e != nil {
		return empty, e
	}
	mac := hmac.New(sha256.New, s.Actions.Key)
	mac.Write([]byte("helm-input/v2\x00"))
	privateInput, _ := json.Marshal(material)
	mac.Write(privateInput)
	clear(privateInput)
	request := struct {
		Cluster     uuid.UUID
		Input       HelmInput
		PrivateHash string
	}{clusterID, in, hex.EncodeToString(mac.Sum(nil))}
	raw, _ := json.Marshal(request)
	replay, e := s.Actions.Idempotency.Lookup(ctx, s.Store.Queries, "enterprise", a.SubjectType+":"+a.SubjectID.String(), "kubernetes.helm.preview", key, raw)
	if e != nil {
		return empty, e
	}
	if replay.Replay {
		var old db.PendingAction
		e = json.Unmarshal(replay.Body, &old)
		return old, e
	}
	private := uuid.New()
	plan.Request.PrivateInputID = private.String()
	material.Key = make([]byte, 32)
	if _, e = rand.Read(material.Key); e != nil {
		return empty, e
	}
	defer clear(material.Key)
	if e = s.Credentials.StageHelmInput(ctx, a, clusterID, in.NamespaceID, plan.Request, material); e != nil {
		return empty, e
	}
	bound := false
	defer func() {
		if !bound {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.Store.Queries.DeleteKubernetesHelmInput(cleanup, db.DeleteKubernetesHelmInputParams{EnterpriseID: a.EnterpriseID, ID: private})
		}
	}()
	validated, e := backend.PreviewHelm(ctx, cluster, plan.Request)
	if e != nil {
		return empty, e
	}
	if !validated.DryRun || validated.Accepted || validated.SchemaVersion != kuberneteshelm.SchemaVersion || len(validated.SealedPlan) == 0 || len(validated.SealedPlan) > kuberneteshelm.MaxManifestBytes+64 {
		return empty, kuberneteshelm.ErrInvalid
	}
	defer clear(validated.EncryptedChart)
	if in.Source != nil {
		resolved, e := kuberneteshelm.ResolvePreviewMaterial(plan.Request, material, validated)
		if e != nil {
			return empty, e
		}
		defer clear(resolved.Chart)
		if e = s.Credentials.ResolveHelmInput(ctx, a, clusterID, in.NamespaceID, plan.Request, resolved); e != nil {
			return empty, e
		}
		material = resolved
	}
	plan.Source = validated.Source
	plan.Request.SealedPlan = validated.SealedPlan
	// The common Connector contract is bounded even for Direct previews. Reject
	// combinations that cannot later be dispatched before asking for approval.
	requestWire, _ := json.Marshal(plan.Request)
	materialWire, _ := json.Marshal(material)
	defer clear(materialWire)
	if len(requestWire)+len(materialWire)+8192 > kuberneteshelm.MaxCommandBytes || (len(requestWire)+2)/3*4+8192 > kuberneteshelm.MaxCommandBytes {
		return empty, kuberneteshelm.ErrBudget
	}
	plan.Objects = validated.Objects
	if e = authorizeHelmObjects(ctx, s.Store.Queries, plan); e != nil {
		return empty, e
	}
	diff := []DiffLine{}
	for _, object := range plan.Objects {
		diff = append(diff, DiffLine{Kind: "change", Text: fmt.Sprintf("%s %s %s/%s", object.Action, object.Definition.Kind, object.Namespace, object.Name)})
		for _, path := range object.Changes {
			if len(diff) >= 256 {
				break
			}
			diff = append(diff, DiffLine{Kind: "change", Text: path + " = [protected chart/value diff]"})
		}
	}
	if len(diff) > 256 {
		return empty, kuberneteshelm.ErrBudget
	}
	action, e := postgres.ExecuteIdempotent(ctx, s.Store, s.Actions.Idempotency, "enterprise", a.SubjectType+":"+a.SubjectID.String(), "kubernetes.helm.preview", key, request, 201, func(q *db.Queries) (db.PendingAction, error) {
		current, _, e := s.prepareHelm(ctx, a, clusterID, in, id, true)
		if e != nil {
			return empty, e
		}
		if current.Scope.ClusterVersion != plan.Scope.ClusterVersion || current.Request.Namespace != plan.Request.Namespace || current.Route != plan.Route {
			return empty, resource.ErrActionInvalidated
		}
		if e = authorizeHelmObjects(ctx, q, plan); e != nil {
			return empty, e
		}
		risk := "write"
		if in.Operation == "uninstall" {
			risk = "dangerous"
		}
		prepared, e := s.Actions.PrepareWithQueries(ctx, q, resource.ActionSubject{ID: a.SubjectID.String(), Type: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ConfirmationRequired: true}, a.EnterpriseID, resource.PrepareActionInput{ActionType: "kubernetes.helm." + in.Operation, Title: "Helm " + in.Operation + " " + in.Name, Summary: in.Reason, Risk: risk, ResourceType: "kubernetes_cluster", ResourceID: uuid.NullUUID{UUID: clusterID, Valid: true}, AuthorizationVersion: a.AuthorizationVersion, RunID: uuid.NullUUID{UUID: a.RunID, Valid: a.RunID != uuid.Nil}, Preview: map[string]any{"operation": in.Operation, "cluster_id": clusterID, "namespace": plan.Request.Namespace.Name, "namespace_uid": plan.Request.Namespace.UID, "name": in.Name, "expected_revision": in.ExpectedRevision, "rollback_revision": in.RollbackRevision, "objects": plan.Objects, "source": plan.Source, "dry_run": true, "keep_history": true}, Diff: diff, ImmutablePlan: plan, ResourceScopeSnapshot: helmSnapshot(plan), CommitHandler: "kubernetes.helm.commit"})
		if e != nil {
			return empty, e
		}
		encoded, _ := json.Marshal(plan)
		_, e = q.CreateKubernetesOperation(ctx, db.CreateKubernetesOperationParams{ID: id, EnterpriseID: a.EnterpriseID, ClusterID: clusterID, NamespaceID: uuid.NullUUID{UUID: in.NamespaceID, Valid: true}, NamespaceUid: plan.Request.Namespace.UID, ActorID: a.SubjectID, ActorType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, ClusterVersion: plan.Scope.ClusterVersion, PendingActionID: uuid.NullUUID{UUID: prepared.ID, Valid: true}, Operation: "helm_" + in.Operation, TypeKey: "helm_release", TargetName: in.Name, ExpectedResourceVersion: fmt.Sprint(in.ExpectedRevision), PrivatePlan: encoded})
		if e != nil {
			return empty, e
		}
		n, e := q.BindKubernetesHelmInput(ctx, db.BindKubernetesHelmInputParams{EnterpriseID: a.EnterpriseID, ID: private, OperationID: id})
		if e != nil {
			return empty, e
		}
		if n != 1 {
			return empty, resource.ErrActionInvalidated
		}
		return prepared, nil
	})
	if e == nil {
		row, err := s.Store.Queries.GetKubernetesHelmInput(ctx, db.GetKubernetesHelmInputParams{EnterpriseID: a.EnterpriseID, ID: private})
		bound = err == nil && row.Status == "bound"
	}
	return action, e
}
func helmSnapshotHash(value any) []byte {
	raw, _ := json.Marshal(value)
	hash := sha256.Sum256(raw)
	return hash[:]
}
