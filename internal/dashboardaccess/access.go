package dashboardaccess

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"slices"
)

var ErrDenied = errors.New("DASHBOARD_DENIED")

type Actor struct {
	RunID                uuid.UUID
	EnterpriseID         uuid.UUID
	SubjectID            uuid.UUID
	SubjectType          string
	AuthorizationVersion int64
}

func Authorize(ctx context.Context, q *db.Queries, actor Actor, permission string) error {
	if actor.EnterpriseID == uuid.Nil || actor.SubjectID == uuid.Nil {
		return ErrDenied
	}
	enterprise, err := q.GetEnterprise(ctx, actor.EnterpriseID)
	if err != nil || enterprise.Status != "active" {
		return ErrDenied
	}
	var permissions []string
	var version int64
	switch actor.SubjectType {
	case "user":
		user, e := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: actor.SubjectID, EnterpriseID: actor.EnterpriseID})
		if e != nil || user.Status != "active" {
			return ErrDenied
		}
		department, e := q.GetDepartment(ctx, db.GetDepartmentParams{ID: user.DepartmentID, EnterpriseID: actor.EnterpriseID})
		if e != nil || department.Status != "active" {
			return ErrDenied
		}
		version = user.AuthorizationVersion
		permissions, err = q.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, DepartmentID: user.DepartmentID})
	case "service_account":
		account, e := q.GetServiceAccount(ctx, db.GetServiceAccountParams{ID: actor.SubjectID, EnterpriseID: actor.EnterpriseID})
		if e != nil || account.Status != "active" {
			return ErrDenied
		}
		version = account.AuthorizationVersion
		permissions, err = q.ListEffectiveServiceAccountPermissions(ctx, db.ListEffectiveServiceAccountPermissionsParams{EnterpriseID: actor.EnterpriseID, ServiceAccountID: actor.SubjectID})
	default:
		return ErrDenied
	}
	if err != nil || version != actor.AuthorizationVersion || !slices.Contains(permissions, permission) && !slices.Contains(permissions, "*") {
		return ErrDenied
	}
	return nil
}

func AuthorizedIDs(ctx context.Context, q *db.Queries, actor Actor, kind string) ([]uuid.UUID, error) {
	switch actor.SubjectType {
	case "user":
		return q.ListUserAuthorizedResourceIDs(ctx, db.ListUserAuthorizedResourceIDsParams{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, ResourceType: kind})
	case "service_account":
		return q.ListServiceAccountAuthorizedResourceIDs(ctx, db.ListServiceAccountAuthorizedResourceIDsParams{EnterpriseID: actor.EnterpriseID, ServiceAccountID: actor.SubjectID, ResourceType: kind})
	default:
		return nil, ErrDenied
	}
}

func RequireObject(ctx context.Context, q *db.Queries, actor Actor, kind string, id uuid.UUID) error {
	ids, err := AuthorizedIDs(ctx, q, actor, kind)
	if err != nil {
		return err
	}
	if !slices.Contains(ids, id) {
		return ErrDenied
	}
	return nil
}
