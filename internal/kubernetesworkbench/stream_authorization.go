package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func authorizeInteractivePermission(ctx context.Context, q *db.Queries, a Actor, httpID uuid.UUID, permission string) error {
	if a.SubjectType != "user" || httpID == uuid.Nil || permission == "" {
		return authorization.ErrAccessDenied
	}
	for _, p := range []string{"kubernetes.read", permission} {
		if err := authorization.Authorize(ctx, q, a, p); err != nil {
			return err
		}
	}
	if _, err := q.ValidateKubernetesLogHTTPSession(ctx, db.ValidateKubernetesLogHTTPSessionParams{ID: httpID, EnterpriseID: uuid.NullUUID{UUID: a.EnterpriseID, Valid: true}, UserID: a.SubjectID}); err != nil {
		return authorization.ErrAccessDenied
	}
	return nil
}
func authorizeStreamRow(ctx context.Context, q *db.Queries, row db.KubernetesStreamSession) (StreamConnection, error) {
	return authorizeStream(ctx, q, row, true)
}
func authorizeStream(ctx context.Context, q *db.Queries, row db.KubernetesStreamSession, live bool) (StreamConnection, error) {
	result := StreamConnection{Row: row}
	a := Actor{EnterpriseID: row.EnterpriseID, SubjectID: row.ActorID, SubjectType: "user", AuthorizationVersion: row.AuthorizationVersion}
	if err := authorizeInteractivePermission(ctx, q, a, row.HttpSessionID, streamPermission(row.Mode)); err != nil {
		return result, err
	}
	if live && !time.Now().Before(row.ExpiresAt.Time) {
		return result, kubernetesinventory.ErrExpired
	}
	if json.Unmarshal(row.Binding, &result.Binding) != nil || result.Binding.Target.Validate() != nil || result.Binding.Target.Mode != row.Mode || result.Binding.Target.Pin.NamespaceUID != row.NamespaceUid {
		return result, kubernetesinventory.ErrProtocol
	}
	cluster, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: row.EnterpriseID, ID: row.ClusterID})
	if err != nil || cluster.Status != "active" || cluster.ResourceVersion != row.ClusterVersion || cluster.ApiServer != result.Binding.Target.APIServer {
		return result, kubernetesinventory.ErrIdentityChanged
	}
	result.Cluster = cluster
	ns, err := q.GetKubernetesNamespaceResource(ctx, db.GetKubernetesNamespaceResourceParams{EnterpriseID: row.EnterpriseID, ID: row.NamespaceID})
	if err != nil || ns.Status != "active" || ns.ClusterID != cluster.ID || ns.NamespaceUid != row.NamespaceUid || ns.Name != result.Binding.Target.Pin.Namespace {
		return result, kubernetesinventory.ErrIdentityChanged
	}
	if err = authorization.RequireResource(ctx, q, a, "kubernetes_cluster", cluster.ID); err != nil {
		if result.Binding.Target.Policy.PrivilegedWorkloads {
			return result, err
		}
		if err = authorization.RequireResource(ctx, q, a, "kubernetes_namespace", ns.ID); err != nil {
			return result, err
		}
	}
	if result.Binding.Target.Policy.PrivilegedWorkloads {
		if err = authorization.Authorize(ctx, q, a, "kubernetes.workload.privileged"); err != nil {
			return result, err
		}
	}
	if row.Mode == "exec" && result.Binding.Target.Policy.ServiceAccountUse {
		if err = authorization.Authorize(ctx, q, a, "kubernetes.service_account.use"); err != nil {
			return result, err
		}
	}
	if !live {
		return result, nil
	}
	route, err := currentStreamRoute(ctx, q, cluster)
	if err != nil {
		return result, err
	}
	if route != result.Binding.Route {
		return result, kubernetesinventory.ErrIdentityChanged
	}
	if row.CredentialLeaseID.Valid {
		recipientType, recipientID := "connector", route.ConnectorID.String()
		if route.Mode == "direct" {
			recipientType, recipientID = "connector_gateway", row.OwnerGateway
		}
		lease, err := q.GetCredentialLease(ctx, db.GetCredentialLeaseParams{EnterpriseID: row.EnterpriseID, ID: row.CredentialLeaseID.UUID})
		if err != nil || lease.Status != "active" || !time.Now().Before(lease.ExpiresAt.Time) || lease.CredentialID != route.CredentialID || lease.RecipientType != recipientType || lease.RecipientID != recipientID || lease.SecretVersionID != route.SecretVersionID || lease.OperationRef != row.ID.String() || lease.TargetResourceType != "kubernetes_stream_session" || lease.TargetResourceID != row.ID {
			return result, secret.ErrInvalidLease
		}
	}
	return result, nil
}

func ValidateStreamCredentialDelivery(ctx context.Context, q *db.Queries, enterprise, id, connector uuid.UUID, epoch int64, leaseID uuid.UUID) error {
	row, err := q.GetKubernetesStreamSession(ctx, id)
	if err != nil || row.EnterpriseID != enterprise || !row.ConnectorID.Valid || row.ConnectorID.UUID != connector || !row.ConnectorEpoch.Valid || row.ConnectorEpoch.Int64 != epoch || !row.CredentialLeaseID.Valid || row.CredentialLeaseID.UUID != leaseID || row.Status != "connecting" && row.Status != "active" {
		return authorization.ErrAccessDenied
	}
	_, err = authorizeStreamRow(ctx, q, row)
	return err
}
