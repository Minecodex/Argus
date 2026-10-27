package authorization

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var ErrAccessDenied = errors.New("object access denied")

// Actor is the authenticated subject, not an identity supplied in a request body.
type Actor struct {
	RunID                uuid.UUID
	EnterpriseID         uuid.UUID
	SubjectID            uuid.UUID
	SubjectType          string
	AuthorizationVersion int64
}

// Authorize checks current membership, function permission and revocation version.
func Authorize(ctx context.Context, q *db.Queries, actor Actor, permission string) error {
	if actor.EnterpriseID == uuid.Nil || actor.SubjectID == uuid.Nil || permission == "" {
		return ErrAccessDenied
	}
	enterprise, err := q.GetEnterprise(ctx, actor.EnterpriseID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err != nil || enterprise.Status != "active" {
		return ErrAccessDenied
	}
	var permissions []string
	var version int64
	switch actor.SubjectType {
	case "user":
		user, e := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: actor.SubjectID, EnterpriseID: actor.EnterpriseID})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e != nil || user.Status != "active" {
			return ErrAccessDenied
		}
		department, e := q.GetDepartment(ctx, db.GetDepartmentParams{ID: user.DepartmentID, EnterpriseID: actor.EnterpriseID})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e != nil || department.Status != "active" {
			return ErrAccessDenied
		}
		version = user.AuthorizationVersion
		permissions, err = q.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, DepartmentID: user.DepartmentID})
	case "service_account":
		account, e := q.GetServiceAccount(ctx, db.GetServiceAccountParams{ID: actor.SubjectID, EnterpriseID: actor.EnterpriseID})
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if e != nil || account.Status != "active" {
			return ErrAccessDenied
		}
		version = account.AuthorizationVersion
		permissions, err = q.ListEffectiveServiceAccountPermissions(ctx, db.ListEffectiveServiceAccountPermissionsParams{EnterpriseID: actor.EnterpriseID, ServiceAccountID: actor.SubjectID})
	default:
		return ErrAccessDenied
	}
	if err != nil {
		return err
	}
	if version != actor.AuthorizationVersion || (!slices.Contains(permissions, permission) && !slices.Contains(permissions, "*")) {
		return ErrAccessDenied
	}
	return nil
}

func ValidResourceType(kind string) bool {
	return slices.Contains([]string{"host", "kubernetes_cluster", "dashboard", "application", "monitoring_target", "kubernetes_namespace"}, kind)
}

func AuthorizedResourceIDs(ctx context.Context, q *db.Queries, actor Actor, kind string) ([]uuid.UUID, error) {
	if !ValidResourceType(kind) {
		return nil, ErrAccessDenied
	}
	ids, err := directAndInheritedIDs(ctx, q, actor, kind)
	if err != nil || kind != "kubernetes_namespace" {
		return ids, err
	}
	clusters, err := directAndInheritedIDs(ctx, q, actor, "kubernetes_cluster")
	if err != nil {
		return nil, err
	}
	rows, err := q.ListAuthorizedKubernetesNamespaceResources(ctx, db.ListAuthorizedKubernetesNamespaceResourcesParams{EnterpriseID: actor.EnterpriseID, ClusterIds: clusters, NamespaceIds: ids})
	if err != nil {
		return nil, err
	}
	result := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.ID)
	}
	return result, nil
}

func directAndInheritedIDs(ctx context.Context, q *db.Queries, actor Actor, kind string) ([]uuid.UUID, error) {
	switch actor.SubjectType {
	case "user":
		return q.ListUserAuthorizedResourceIDs(ctx, db.ListUserAuthorizedResourceIDsParams{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, ResourceType: kind})
	case "service_account":
		return q.ListServiceAccountAuthorizedResourceIDs(ctx, db.ListServiceAccountAuthorizedResourceIDsParams{EnterpriseID: actor.EnterpriseID, ServiceAccountID: actor.SubjectID, ResourceType: kind})
	default:
		return nil, ErrAccessDenied
	}
}

func RequireResource(ctx context.Context, q *db.Queries, actor Actor, kind string, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrAccessDenied
	}
	ids, err := AuthorizedResourceIDs(ctx, q, actor, kind)
	if err != nil {
		return err
	}
	if !slices.Contains(ids, id) {
		return ErrAccessDenied
	}
	return nil
}

// GrantCreatedResource uses the existing explicit-grant ledger for new objects.
// Granting the object never grants its dependencies or execution Collector.
func GrantCreatedResource(ctx context.Context, q *db.Queries, actor Actor, kind string, id uuid.UUID) error {
	if !ValidResourceType(kind) || (actor.SubjectType != "user" && actor.SubjectType != "service_account") {
		return ErrAccessDenied
	}
	_, err := q.AddDataAuthorizationGrant(ctx, db.AddDataAuthorizationGrantParams{ID: uuid.New(), EnterpriseID: actor.EnterpriseID, SubjectType: actor.SubjectType, SubjectID: actor.SubjectID, ResourceType: kind, ResourceID: id})
	// Match Host/Dashboard creation: this grants a newly-created object only.
	// Revoking the subject's existing remote sessions here would make creating an
	// application interrupt unrelated work. Explicit grant edits still use the
	// normal authorization-version and revocation workflow.
	return err
}
