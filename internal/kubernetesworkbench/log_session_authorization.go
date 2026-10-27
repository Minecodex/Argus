package kubernetesworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func authorizeLogPermission(ctx context.Context, q *db.Queries, a Actor, httpID uuid.UUID) error {
	if a.SubjectType != "user" || httpID == uuid.Nil {
		return authorization.ErrAccessDenied
	}
	for _, permission := range []string{"kubernetes.read", "kubernetes.operational_logs.read"} {
		if err := authorization.Authorize(ctx, q, a, permission); err != nil {
			return err
		}
	}
	if _, err := q.ValidateKubernetesLogHTTPSession(ctx, db.ValidateKubernetesLogHTTPSessionParams{ID: httpID, EnterpriseID: uuid.NullUUID{UUID: a.EnterpriseID, Valid: true}, UserID: a.SubjectID}); err != nil {
		return authorization.ErrAccessDenied
	}
	return nil
}

func authorizeLogRow(ctx context.Context, q *db.Queries, a Actor, httpID uuid.UUID, row db.KubernetesLogSession, active bool) (db.KubernetesCluster, kubernetespodlogs.Query, error) {
	var query kubernetespodlogs.Query
	fail := func(err error) (db.KubernetesCluster, kubernetespodlogs.Query, error) {
		return db.KubernetesCluster{}, query, err
	}
	if row.ActorID != a.SubjectID || row.HttpSessionID != httpID || row.EnterpriseID != a.EnterpriseID || row.AuthorizationVersion != a.AuthorizationVersion {
		return fail(authorization.ErrAccessDenied)
	}
	if err := authorizeLogPermission(ctx, q, a, httpID); err != nil {
		return fail(err)
	}
	if !time.Now().Before(row.ExpiresAt.Time) {
		return fail(kubernetesinventory.ErrExpired)
	}
	if active && row.Status != "active" || !active && row.Status != "pending" && row.Status != "active" {
		return fail(authorization.ErrAccessDenied)
	}
	if json.Unmarshal(row.Binding, &query) != nil || query.Validate() != nil || query.Resume != nil || row.NamespaceUid != query.Pin.NamespaceUID {
		return fail(kubernetesinventory.ErrProtocol)
	}
	cluster, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: a.EnterpriseID, ID: row.ClusterID})
	if err != nil || cluster.Status != "active" || cluster.ResourceVersion != row.ClusterVersion || cluster.ApiServer != query.APIServer {
		return fail(kubernetesinventory.ErrIdentityChanged)
	}
	namespace, err := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: a.EnterpriseID, ID: row.NamespaceID})
	if err != nil || namespace.Status != "active" || namespace.ClusterID != row.ClusterID || namespace.Name != query.Pin.Namespace || namespace.NamespaceUid != query.Pin.NamespaceUID {
		return fail(kubernetesinventory.ErrIdentityChanged)
	}
	if err = authorization.RequireResource(ctx, q, a, "kubernetes_cluster", row.ClusterID); err != nil {
		if err = authorization.RequireResource(ctx, q, a, "kubernetes_namespace", row.NamespaceID); err != nil {
			return fail(err)
		}
	}
	return cluster, query, nil
}

func (s Service) logSession(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID, active bool) (db.KubernetesLogSession, kubernetespodlogs.Query, error) {
	row, err := s.Store.Queries.GetKubernetesLogSession(ctx, db.GetKubernetesLogSessionParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil || row.ClusterID != cluster || row.ActorID != a.SubjectID || row.HttpSessionID != httpID {
		return row, kubernetespodlogs.Query{}, authorization.ErrAccessDenied
	}
	if row.Status == "closed" || row.Status == "invalidated" {
		return row, kubernetespodlogs.Query{}, authorization.ErrAccessDenied
	}
	if active && row.Status == "pending" {
		return row, kubernetespodlogs.Query{}, kubernetesinventory.ErrMutationConflict
	}
	_, query, err := authorizeLogRow(ctx, s.Store.Queries, a, httpID, row, active)
	if err != nil && row.ActorID == a.SubjectID && row.HttpSessionID == httpID {
		s.invalidateLogSession(ctx, row)
	}
	return row, query, err
}
func (s Service) invalidateLogSession(ctx context.Context, row db.KubernetesLogSession) {
	_ = s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if err := q.InvalidateKubernetesLogSession(ctx, db.InvalidateKubernetesLogSessionParams{EnterpriseID: row.EnterpriseID, ID: row.ID}); err != nil {
			return err
		}
		return logAudit(ctx, q, row, "invalidated", nil)
	})
}

// ValidateLogDelivery is shared by Gateway dispatch and domain tests. The
// durable session, not the queued payload, determines actor and resource scope.
func ValidateLogDelivery(ctx context.Context, q *db.Queries, enterprise, cluster, id uuid.UUID, request kubernetespodlogs.Query) error {
	row, err := q.GetKubernetesLogSession(ctx, db.GetKubernetesLogSessionParams{EnterpriseID: enterprise, ID: id})
	if err != nil || row.ClusterID != cluster {
		return authorization.ErrAccessDenied
	}
	actor := Actor{EnterpriseID: enterprise, SubjectID: row.ActorID, SubjectType: "user", AuthorizationVersion: row.AuthorizationVersion}
	_, expected, err := authorizeLogRow(ctx, q, actor, row.HttpSessionID, row, true)
	if err != nil {
		return err
	}
	if request.Pin != expected.Pin || request.APIServer != expected.APIServer || !request.Since.Equal(expected.Since) || request.TailLines != expected.TailLines || !bytes.Equal(request.ContinuityKey, expected.ContinuityKey) {
		return kubernetesinventory.ErrIdentityChanged
	}
	return request.Validate()
}
