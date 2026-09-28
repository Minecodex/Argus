package presentation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Scope fingerprints current permissions AND resource grants. Dynamic group or
// label changes can alter resource access without changing the Tool version.
func Scope(ctx context.Context, store *postgres.Store, enterprise, user uuid.UUID) (string, error) {
	actor, err := store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: user, EnterpriseID: enterprise})
	if err != nil {
		return "", err
	}
	if actor.Status != "active" {
		return "", toolruntime.Error{Kind: "PRESENTATION_FORBIDDEN"}
	}
	permissions, err := store.Queries.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{EnterpriseID: enterprise, UserID: user, DepartmentID: actor.DepartmentID})
	if err != nil {
		return "", err
	}
	sort.Strings(permissions)
	ids := []string{}
	for _, kind := range []string{"host", "kubernetes_cluster", "dashboard"} {
		values, err := store.Queries.ListUserAuthorizedResourceIDs(ctx, db.ListUserAuthorizedResourceIDsParams{EnterpriseID: enterprise, UserID: user, ResourceType: kind})
		if err != nil {
			return "", err
		}
		for _, value := range values {
			ids = append(ids, kind+":"+value.String())
		}
	}
	sort.Strings(ids)
	encoded, _ := json.Marshal([]any{datapolicy.Version, enterprise, user, actor.AuthorizationVersion, permissions, ids})
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), nil
}
