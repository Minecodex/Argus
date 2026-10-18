package remoteexecution

import (
	"context"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func principalWithQueries(ctx context.Context, q *db.Queries, enterprise, user, conversation uuid.UUID, permission string) (toolruntime.Principal, error) {
	p := toolruntime.Principal{EnterpriseID: enterprise, UserID: user, ConversationID: conversation}
	e, err := q.GetEnterprise(ctx, enterprise)
	if err != nil || e.Status != "active" {
		return p, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	u, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{ID: user, EnterpriseID: enterprise})
	if err != nil || u.Status != "active" {
		return p, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	d, err := q.GetDepartment(ctx, db.GetDepartmentParams{ID: u.DepartmentID, EnterpriseID: enterprise})
	if err != nil || d.Status != "active" {
		return p, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	if _, err = q.GetConversation(ctx, db.GetConversationParams{ID: conversation, EnterpriseID: enterprise, OwnerUserID: user}); err != nil {
		return p, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	p.AuthorizationVersion = u.AuthorizationVersion
	p.Permissions, err = q.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{EnterpriseID: enterprise, UserID: user, DepartmentID: u.DepartmentID})
	if err != nil {
		return p, err
	}
	if !p.Allows(permission) {
		return p, Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	return p, nil
}

func editableInput(spec Spec) Input {
	steps := make([]Step, len(spec.Steps))
	for n, s := range spec.Steps {
		steps[n] = Step{Description: s.Description, Target: targetScope(s.Target), Interpreter: s.Interpreter, Script: s.Script, TimeoutSeconds: s.TimeoutSeconds}
	}
	return Input{Title: spec.Title, Description: spec.Description, Steps: steps, SupersedesID: spec.SupersedesID, RecoveryOfID: spec.RecoveryOfID}
}
