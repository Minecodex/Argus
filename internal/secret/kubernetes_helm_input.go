package secret

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func helmInputAAD(enterprise, id, operation uuid.UUID) []byte {
	return []byte("argus.kubernetes_helm_input/v1\x00" + enterprise.String() + "\x00" + id.String() + "\x00" + operation.String())
}
func helmRequestHash(r kuberneteshelm.Request) []byte {
	r.SealedPlan = nil
	raw, _ := json.Marshal(r)
	hash := sha256.Sum256(raw)
	return hash[:]
}
func authorizeHelmInput(ctx context.Context, q *db.Queries, a authorization.Actor, cluster, namespace uuid.UUID, uid string) error {
	if e := authorization.Authorize(ctx, q, a, "kubernetes.helm.manage"); e != nil {
		return e
	}
	ns, e := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: a.EnterpriseID, ID: namespace})
	if e != nil || ns.Status != "active" || ns.ClusterID != cluster || ns.NamespaceUid != uid {
		return ErrKubernetesPrivateInput
	}
	if e = authorization.RequireResource(ctx, q, a, "kubernetes_cluster", cluster); e != nil {
		if e = authorization.RequireResource(ctx, q, a, "kubernetes_namespace", namespace); e != nil {
			return e
		}
	}
	return nil
}
func (s Service) StageHelmInput(ctx context.Context, a authorization.Actor, cluster, namespace uuid.UUID, r kuberneteshelm.Request, m kuberneteshelm.Material) error {
	id, e := uuid.Parse(r.PrivateInputID)
	if e != nil {
		return ErrKubernetesPrivateInput
	}
	operation, e := uuid.Parse(r.OperationID)
	if e != nil || len(m.Key) != 32 || len(m.Chart) > kuberneteshelm.MaxArchiveBytes || len(m.Values) > kuberneteshelm.MaxValuesBytes || kuberneteshelm.ValidateSourceCredentials(m.SourceCredentials) != nil {
		return ErrKubernetesPrivateInput
	}
	if e = authorizeHelmInput(ctx, s.Store.Queries, a, cluster, namespace, r.Namespace.UID); e != nil {
		return e
	}
	raw, e := json.Marshal(m)
	if e != nil {
		return ErrKubernetesPrivateInput
	}
	defer clear(raw)
	envelope, e := s.Keyring.EncryptContext(ctx, raw, helmInputAAD(a.EnterpriseID, id, operation))
	if e != nil {
		return e
	}
	encoded, e := json.Marshal(envelope)
	if e != nil {
		return e
	}
	return s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if e := authorizeHelmInput(ctx, q, a, cluster, namespace, r.Namespace.UID); e != nil {
			return e
		}
		if e := q.CreateKubernetesHelmInput(ctx, db.CreateKubernetesHelmInputParams{ID: id, EnterpriseID: a.EnterpriseID, OperationID: operation, ClusterID: cluster, NamespaceID: namespace, NamespaceUid: r.Namespace.UID, ActorID: a.SubjectID, ActorType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, RequestHash: helmRequestHash(r), Envelope: encoded}); e != nil {
			return e
		}
		typeName := "enterprise_user"
		if a.SubjectType == "service_account" {
			typeName = "service_account"
		}
		_, e := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: a.EnterpriseID, Valid: true}, ActorType: typeName, ActorID: a.SubjectID.String(), Action: "kubernetes.helm_input.stage", ResourceType: "kubernetes_cluster", ResourceID: cluster.String(), Result: "success", Details: map[string]any{"operation_id": operation.String()}})
		return e
	})
}
func (s Service) LoadHelmInput(ctx context.Context, enterprise, cluster uuid.UUID, r kuberneteshelm.Request, preview bool, recipientType, recipientID string) (kuberneteshelm.Material, error) {
	var out kuberneteshelm.Material
	if recipientID == "" || recipientType != "server" && recipientType != "worker" && recipientType != "connector" {
		return out, ErrKubernetesPrivateInput
	}
	id, e := uuid.Parse(r.PrivateInputID)
	if e != nil {
		return out, ErrKubernetesPrivateInput
	}
	row, e := s.Store.Queries.GetKubernetesHelmInput(ctx, db.GetKubernetesHelmInputParams{EnterpriseID: enterprise, ID: id})
	if e != nil || row.ClusterID != cluster || row.NamespaceUid != r.Namespace.UID || row.OperationID.String() != r.OperationID || !time.Now().Before(row.ExpiresAt.Time) || !bytes.Equal(row.RequestHash, helmRequestHash(r)) {
		return out, ErrKubernetesPrivateInput
	}
	actor := authorization.Actor{EnterpriseID: enterprise, SubjectID: row.ActorID, SubjectType: row.ActorType, AuthorizationVersion: row.AuthorizationVersion}
	if e = authorizeHelmInput(ctx, s.Store.Queries, actor, cluster, row.NamespaceID, row.NamespaceUid); e != nil {
		return out, e
	}
	if !preview {
		op, e := s.Store.Queries.GetKubernetesOperation(ctx, db.GetKubernetesOperationParams{EnterpriseID: enterprise, ID: row.OperationID})
		if e != nil || row.Status != "bound" || op.TypeKey != "helm_release" || op.Status != "running" && op.Status != "observing" && op.Status != "result_unknown" {
			return out, ErrKubernetesPrivateInput
		}
	}
	if recipientType == "connector" {
		connector, e := uuid.Parse(recipientID)
		if e != nil {
			return out, ErrKubernetesPrivateInput
		}
		target, e := s.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: enterprise, ID: cluster})
		if e != nil {
			return out, e
		}
		expected := target.ConnectorID
		if target.ConnectionMode == "via_bastion" {
			if !target.BastionScopeID.Valid {
				return out, ErrKubernetesPrivateInput
			}
			scope, e := s.Store.Queries.GetBastionScope(ctx, db.GetBastionScopeParams{EnterpriseID: enterprise, ID: target.BastionScopeID.UUID})
			if e != nil {
				return out, e
			}
			expected = scope.ActiveConnectorID
		}
		if !expected.Valid || expected.UUID != connector {
			return out, ErrKubernetesPrivateInput
		}
	}
	var envelope Envelope
	if json.Unmarshal(row.Envelope, &envelope) != nil {
		return out, ErrKubernetesPrivateInput
	}
	raw, e := s.Keyring.DecryptContext(ctx, envelope, helmInputAAD(enterprise, id, row.OperationID))
	if e != nil {
		return out, e
	}
	defer clear(raw)
	if json.Unmarshal(raw, &out) != nil || len(out.Key) != 32 {
		return kuberneteshelm.Material{}, ErrKubernetesPrivateInput
	}
	return out, nil
}

// ResolveHelmInput replaces one staged origin credential envelope with its
// authenticated chart bytes. Execution receives no repository credentials.
func (s Service) ResolveHelmInput(ctx context.Context, a authorization.Actor, cluster, namespace uuid.UUID, r kuberneteshelm.Request, m kuberneteshelm.Material) error {
	id, e := uuid.Parse(r.PrivateInputID)
	if e != nil || m.SourceCredentials != nil || len(m.Chart) == 0 || len(m.Chart) > kuberneteshelm.MaxArchiveBytes || len(m.Key) != 32 {
		return ErrKubernetesPrivateInput
	}
	operation, e := uuid.Parse(r.OperationID)
	if e != nil {
		return ErrKubernetesPrivateInput
	}
	raw, e := json.Marshal(m)
	if e != nil {
		return ErrKubernetesPrivateInput
	}
	defer clear(raw)
	envelope, e := s.Keyring.EncryptContext(ctx, raw, helmInputAAD(a.EnterpriseID, id, operation))
	if e != nil {
		return e
	}
	encoded, e := json.Marshal(envelope)
	if e != nil {
		return ErrKubernetesPrivateInput
	}
	return s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if e := authorizeHelmInput(ctx, q, a, cluster, namespace, r.Namespace.UID); e != nil {
			return e
		}
		row, e := q.GetKubernetesHelmInput(ctx, db.GetKubernetesHelmInputParams{EnterpriseID: a.EnterpriseID, ID: id})
		if e != nil || row.ActorID != a.SubjectID || row.ActorType != a.SubjectType || row.ClusterID != cluster || row.NamespaceID != namespace || row.AuthorizationVersion != a.AuthorizationVersion {
			return ErrKubernetesPrivateInput
		}
		n, e := q.ResolveKubernetesHelmInput(ctx, db.ResolveKubernetesHelmInputParams{EnterpriseID: a.EnterpriseID, ID: id, OperationID: operation, RequestHash: helmRequestHash(r), Envelope: encoded})
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrKubernetesPrivateInput
		}
		return nil
	})
}
