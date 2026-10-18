package remoteexecution

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestNativeActionReadCannotCrossConversationOrRetainRevokedAccess(t *testing.T) {
	f := newFixture(t)
	v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("inspect-private"))
	if err != nil {
		t.Fatal(err)
	}
	action, err := f.store.Queries.GetPendingAction(t.Context(), db.GetPendingActionParams{ActionRef: v.ActionRef, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	subject := resource.Subject{ActorID: f.u.String(), RunID: uuid.NullUUID{UUID: f.r, Valid: true}}
	if err = f.service.AuthorizeModelActionRead(t.Context(), subject, action); err != nil {
		t.Fatal(err)
	}
	otherConversation, otherRun := uuid.New(), uuid.New()
	f.exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Other chat',$4)", otherConversation, f.e, f.u, f.m)
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'running')", otherRun, otherConversation, f.e, f.u, f.m)
	subject.RunID.UUID = otherRun
	if err = f.service.AuthorizeModelActionRead(t.Context(), subject, action); !errorsIsCode(err, "REMOTE_EXECUTION_FORBIDDEN") {
		t.Fatalf("other chat read private plan: %v", err)
	}
	subject.RunID.UUID = f.r
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1 AND subject_id=$2", f.e, f.u)
	if err = f.service.AuthorizeModelActionRead(t.Context(), subject, action); !errorsIsCode(err, "REMOTE_RESOURCE_FORBIDDEN") {
		t.Fatalf("revoked resource still readable: %v", err)
	}
}
