package remoteexecution

import (
	"context"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

// Native action lookup must not let a model use the general workflow catalog
// to read a different conversation's scripts or bypass revoked source grants.
func (s Service) AuthorizeModelActionRead(ctx context.Context, subject resource.Subject, action db.PendingAction) error {
	user, err := uuid.Parse(subject.ActorID)
	if err != nil || subject.ActorType == "service_account" || !subject.RunID.Valid {
		return Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	run, err := s.Store.Queries.GetRun(ctx, db.GetRunParams{ID: subject.RunID.UUID, EnterpriseID: action.EnterpriseID})
	if err != nil || run.ActorUserID != user {
		return Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	request, err := s.Store.Queries.GetRemoteRequestByAction(ctx, db.GetRemoteRequestByActionParams{ActionRef: action.ActionRef, EnterpriseID: action.EnterpriseID})
	if err != nil || request.OwnerUserID != user || request.ConversationID != run.ConversationID {
		return Error("REMOTE_EXECUTION_FORBIDDEN")
	}
	_, err = s.Get(ctx, toolruntime.Principal{EnterpriseID: action.EnterpriseID, UserID: user, ConversationID: run.ConversationID}, request.ID)
	return err
}
