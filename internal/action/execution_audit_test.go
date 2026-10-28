package action

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"testing"
)

func TestExecutionAuditTargetUsesRealSourceOrCreatedResource(t *testing.T) {
	source, created, executionID := uuid.New(), uuid.New(), uuid.New()
	execution := db.Execution{ID: executionID}
	action := db.PendingAction{ResourceType: "host", ResourceID: uuid.NullUUID{UUID: source, Valid: true}, ResultResourceType: pgtype.Text{String: "remote_execution", Valid: true}, ResultResourceID: uuid.NullUUID{UUID: created, Valid: true}}
	if kind, id := executionAuditTarget(action, execution); kind != "host" || id != source {
		t.Fatal("lost actual operation target")
	}
	action.ResourceID = uuid.NullUUID{}
	if kind, id := executionAuditTarget(action, execution); kind != "remote_execution" || id != created {
		t.Fatal("lost newly created result identity")
	}
	action.ResultResourceID = uuid.NullUUID{}
	if kind, id := executionAuditTarget(action, execution); kind != "execution" || id != executionID {
		t.Fatal("must not emit a fabricated zero resource ID")
	}
}
