package workspace

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func TestBackgroundTransferKeepsRunOwnershipAndCancellation(t *testing.T) {
	p := toolruntime.Principal{EnterpriseID: uuid.New(), UserID: uuid.New(), ConversationID: uuid.New()}
	for _, status := range []string{"running", "waiting_system", "succeeded", "failed", "cancelled", "timed_out"} {
		run := db.Run{EnterpriseID: p.EnterpriseID, ActorUserID: p.UserID, ConversationID: p.ConversationID, Status: status}
		background := workspaceRunAllowed(run, p, accessScope{BackgroundTransfer: true})
		tool := workspaceRunAllowed(run, p, accessScope{})
		if background != (status != "cancelled" && status != "timed_out") {
			t.Fatalf("background status %s: %t", status, background)
		}
		if tool != (status == "running" || status == "waiting_system") {
			t.Fatalf("terminal model tool escaped its Run: %s", status)
		}
		run.ActorUserID = uuid.New()
		if workspaceRunAllowed(run, p, accessScope{BackgroundTransfer: true}) {
			t.Fatal("foreign owner accepted")
		}
		run.ActorUserID = p.UserID
		run.ConversationID = uuid.New()
		if workspaceRunAllowed(run, p, accessScope{BackgroundTransfer: true}) {
			t.Fatal("foreign conversation accepted")
		}
		run.ConversationID = p.ConversationID
		run.EnterpriseID = uuid.New()
		if workspaceRunAllowed(run, p, accessScope{BackgroundTransfer: true}) {
			t.Fatal("foreign enterprise accepted")
		}
	}
}
