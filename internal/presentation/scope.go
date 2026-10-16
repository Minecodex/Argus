package presentation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Scope fingerprints current permissions AND resource grants. Dynamic group or
// label changes can alter resource access without changing the Tool version.
type Access struct {
	Scope       string
	Permissions []string
	ResourceIDs map[string][]uuid.UUID
}

// AllowsResource requires both the directory permission and a grant of the
// matching resource type. Connector visibility alone does not reveal names of
// its associated hosts or clusters.
func (access Access) AllowsResource(ref toolruntime.ResourceRef) bool {
	permission := map[string]string{"host": "host.read", "kubernetes_cluster": "kubernetes.read"}[ref.Type]
	id, err := uuid.Parse(ref.ID)
	return err == nil && permission != "" && slices.Contains(access.Permissions, permission) && slices.Contains(access.ResourceIDs[ref.Type], id)
}

func Scope(ctx context.Context, store *postgres.Store, enterprise, user uuid.UUID) (string, error) {
	access, err := CurrentAccess(ctx, store, enterprise, user)
	return access.Scope, err
}

// CurrentAccess supplies the same authorization snapshot used by the stored
// fingerprint, so enrichment does not query permissions separately per row.
func CurrentAccess(ctx context.Context, store *postgres.Store, enterprise, user uuid.UUID) (Access, error) {
	actor, err := store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: user, EnterpriseID: enterprise})
	if err != nil {
		return Access{}, err
	}
	if actor.Status != "active" {
		return Access{}, toolruntime.Error{Kind: "PRESENTATION_FORBIDDEN"}
	}
	permissions, err := store.Queries.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{EnterpriseID: enterprise, UserID: user, DepartmentID: actor.DepartmentID})
	if err != nil {
		return Access{}, err
	}
	sort.Strings(permissions)
	ids := []string{}
	access := Access{Permissions: permissions, ResourceIDs: map[string][]uuid.UUID{}}
	for _, kind := range []string{"host", "kubernetes_cluster", "dashboard"} {
		values, err := store.Queries.ListUserAuthorizedResourceIDs(ctx, db.ListUserAuthorizedResourceIDsParams{EnterpriseID: enterprise, UserID: user, ResourceType: kind})
		if err != nil {
			return Access{}, err
		}
		access.ResourceIDs[kind] = values
		for _, value := range values {
			ids = append(ids, kind+":"+value.String())
		}
	}
	sort.Strings(ids)
	encoded, _ := json.Marshal([]any{datapolicy.Version, enterprise, user, actor.AuthorizationVersion, permissions, ids})
	hash := sha256.Sum256(encoded)
	access.Scope = hex.EncodeToString(hash[:])
	return access, nil
}
