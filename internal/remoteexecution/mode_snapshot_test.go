package remoteexecution

import (
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestModeSwitchKeepsRunSnapshotAndApprovedBatch(t *testing.T) {
	f := newFixture(t)
	view, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("pwd"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(view)
	mode := "plan"
	service := conversation.Service{Store: f.store}
	current, err := f.store.Queries.GetConversation(t.Context(), db.GetConversationParams{ID: f.c, EnterpriseID: f.e, OwnerUserID: f.u})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := service.Update(t.Context(), f.e, f.u, f.c, current.Version, nil, nil, nil, nil, &mode)
	if err != nil || updated.Mode != "plan" {
		t.Fatalf("mode switch: %s %v", updated.Mode, err)
	}
	original, err := f.store.Queries.GetRun(t.Context(), db.GetRunParams{ID: f.r, EnterpriseID: f.e})
	if err != nil || original.ModeSnapshot != "execute" {
		t.Fatalf("mode switch rewrote running context: %s %v", original.ModeSnapshot, err)
	}
	if err = f.run(view); err != nil {
		t.Fatal(err)
	}
	if f.transport.batches.Load() != 1 {
		t.Fatal("mode switch cancelled or duplicated an approved batch")
	}
	f.exec("UPDATE runs SET status='succeeded' WHERE id=$1", f.r)
	next, err := f.store.Queries.CreateRun(t.Context(), db.CreateRunParams{ID: uuid.New(), ConversationID: f.c, EnterpriseID: f.e, ActorUserID: f.u, ModelID: f.m, ModelRevision: 1, Locale: "zh-CN", AuthorizationVersion: 1, Checkpoint: []byte(`{}`)})
	if err != nil || next.ModeSnapshot != "plan" {
		t.Fatalf("new Run did not freeze the selected mode: %s %v", next.ModeSnapshot, err)
	}
	invalid := "readonly"
	if _, err = service.Update(t.Context(), f.e, f.u, f.c, updated.Version, nil, nil, nil, nil, &invalid); err == nil {
		t.Fatal("unknown conversation mode accepted")
	}
}
