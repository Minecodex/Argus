package kubernetesworkbench

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Inventory synchronization uses the enrolled Cluster's existing read identity.
// It maintains metadata only and never creates a user or resource grant. The
// lease and route version fence let multiple workers reconcile without races.
func (s Service) ReconcileNamespaceInventory(ctx context.Context) error {
	if s.Store == nil || s.Backend == nil {
		return kubernetesinventory.ErrUnavailable
	}
	var failures []error
	for i := 0; i < 4 && ctx.Err() == nil; i++ {
		claim, err := s.Store.Queries.ClaimKubernetesNamespaceSync(ctx, uuid.NullUUID{UUID: uuid.New(), Valid: true})
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		call, cancel := context.WithTimeout(ctx, 45*time.Second)
		err = s.syncClaimedNamespaces(call, claim)
		cancel()
		if err != nil {
			failures = append(failures, err)
			// Do not store endpoints, response bodies, credentials or raw errors.
			_, updateErr := s.Store.Queries.FinishKubernetesNamespaceSync(ctx, db.FinishKubernetesNamespaceSyncParams{ClusterID: claim.ClusterID, EnterpriseID: claim.EnterpriseID, LeaseToken: claim.LeaseToken, Status: "error", ErrorCode: kubernetesinventory.ErrorCode(err)})
			if updateErr != nil {
				failures = append(failures, updateErr)
			}
		}
	}
	return errors.Join(failures...)
}

func (s Service) syncClaimedNamespaces(ctx context.Context, claim db.KubernetesNamespaceSyncState) error {
	cluster, err := s.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: claim.EnterpriseID, ID: claim.ClusterID})
	if err != nil {
		return err
	}
	if cluster.Status != "active" || cluster.ResourceVersion != claim.ObservedClusterVersion {
		return kubernetesinventory.ErrIdentityChanged
	}
	objects := []kubernetesinventory.Object{}
	cursor, version := "", ""
	seenCursors, names, uids := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for {
		page, err := s.Backend.ListInventory(ctx, cluster, kubernetesinventory.Query{Type: "namespace", Limit: 200, Continue: cursor})
		if err != nil {
			return err
		}
		if len(objects)+len(page.Items) > 10000 || len(seenCursors) >= 50 || len(page.Items) > 200 {
			return kubernetesinventory.ErrBudget
		}
		if version != "" && page.ResourceVersion != version {
			return kubernetesinventory.ErrIdentityChanged
		}
		version = page.ResourceVersion
		for _, object := range page.Items {
			if object.Definition.Key != "namespace" || object.Namespace != "" || object.Name == "" || object.UID == "" || names[object.Name] || uids[object.UID] {
				return kubernetesinventory.ErrInvalid
			}
			names[object.Name], uids[object.UID] = true, true
			objects = append(objects, object)
		}
		if page.Continue == "" {
			if !page.Complete {
				return kubernetesinventory.ErrInvalid
			}
			break
		}
		if seenCursors[page.Continue] {
			return kubernetesinventory.ErrInvalid
		}
		cursor, seenCursors[page.Continue] = page.Continue, true
	}
	identities, err := s.readNamespaceObjectIdentities(ctx, cluster)
	if err != nil {
		return err
	}
	return s.applyNamespaceSnapshot(ctx, claim, objects, version, identities)
}

func (s Service) applyNamespaceSnapshot(ctx context.Context, claim db.KubernetesNamespaceSyncState, objects []kubernetesinventory.Object, resourceVersion string, identities []kubernetesinventory.Object) error {
	return s.Store.InTx(ctx, func(q *db.Queries) error {
		cluster, err := q.LockNamespaceInventoryCluster(ctx, db.LockNamespaceInventoryClusterParams{EnterpriseID: claim.EnterpriseID, ID: claim.ClusterID})
		if err != nil {
			return err
		}
		if cluster.Status != "active" || cluster.ResourceVersion != claim.ObservedClusterVersion {
			return kubernetesinventory.ErrIdentityChanged
		}
		prior, err := q.ListClusterNamespaceInventory(ctx, db.ListClusterNamespaceInventoryParams{EnterpriseID: claim.EnterpriseID, ClusterID: claim.ClusterID})
		if err != nil {
			return err
		}
		wanted := []string{}
		recentNames, recentUIDs := map[string]bool{}, map[string]bool{}
		for _, row := range prior {
			if row.ObservedAt.Time.After(claim.SnapshotStartedAt.Time) {
				recentNames[row.Name], recentUIDs[row.NamespaceUid] = true, true
			}
		}
		for _, object := range objects {
			wanted = append(wanted, object.UID)
			if recentNames[object.Name] || recentUIDs[object.UID] {
				continue // A foreground read already observed a newer generation.
			}
			if err := q.RetireReplacedKubernetesNamespace(ctx, db.RetireReplacedKubernetesNamespaceParams{EnterpriseID: claim.EnterpriseID, ClusterID: claim.ClusterID, Name: object.Name, NamespaceUid: object.UID}); err != nil {
				return err
			}
			if _, err := q.UpsertKubernetesNamespaceResource(ctx, db.UpsertKubernetesNamespaceResourceParams{ID: uuid.NewSHA1(claim.ClusterID, []byte(object.UID)), EnterpriseID: claim.EnterpriseID, ClusterID: claim.ClusterID, Name: object.Name, NamespaceUid: object.UID, ResourceVersion: object.ResourceVersion}); err != nil {
				return err
			}
		}
		if err := q.RetireMissingKubernetesNamespaces(ctx, db.RetireMissingKubernetesNamespacesParams{EnterpriseID: claim.EnterpriseID, ClusterID: claim.ClusterID, NamespaceUids: wanted, SnapshotStartedAt: claim.SnapshotStartedAt}); err != nil {
			return err
		}
		if err := applyNamespaceObjectIdentities(ctx, q, claim.EnterpriseID, claim.ClusterID, identities); err != nil {
			return err
		}
		count, err := q.FinishKubernetesNamespaceSync(ctx, db.FinishKubernetesNamespaceSyncParams{ClusterID: claim.ClusterID, EnterpriseID: claim.EnterpriseID, LeaseToken: claim.LeaseToken, Status: "healthy", SnapshotResourceVersion: resourceVersion, NamespaceCount: int32(len(objects))})
		if err != nil {
			return err
		}
		if count != 1 {
			return kubernetesinventory.ErrExpired // Roll back all changes on lost lease.
		}
		return nil
	})
}

func (w Worker) RunNamespaceReconciler(ctx context.Context) error {
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		if err := w.Service.ReconcileNamespaceInventory(ctx); err != nil && ctx.Err() == nil {
			logger.Warn("Kubernetes namespace inventory reconciliation failed", "error_code", kubernetesinventory.ErrorCode(err))
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
