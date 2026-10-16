package httpapi

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
	"sort"
)

func alertAuditResources(ctx context.Context, q *db.Queries, p identity.Principal) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	for _, kind := range []struct{ name, permission string }{{"host", "host.read"}, {"kubernetes_cluster", "kubernetes.read"}} {
		if !slices.Contains(p.Permissions, "*") && !slices.Contains(p.Permissions, kind.permission) {
			continue
		}
		var current []uuid.UUID
		var err error
		if p.ServiceAccount != nil {
			current, err = q.ListServiceAccountAuthorizedResourceIDs(ctx, db.ListServiceAccountAuthorizedResourceIDsParams{EnterpriseID: p.EnterpriseIDValue(), ServiceAccountID: p.ServiceAccount.ID, ResourceType: kind.name})
		} else {
			current, err = q.ListUserAuthorizedResourceIDs(ctx, db.ListUserAuthorizedResourceIDsParams{EnterpriseID: p.EnterpriseIDValue(), UserID: p.EnterpriseUser.ID, ResourceType: kind.name})
		}
		if err != nil {
			return nil, err
		}
		ids = append(ids, current...)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	return slices.Compact(ids), nil
}
