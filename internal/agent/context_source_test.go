package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestModelCallKeepsAssembledSnapshotAcrossConcurrentReplacement(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx := t.Context()
	scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, _ := json.Marshal(scopedCheckpoint{AuthorizationScope: scope})
	for _, kind := range []string{"user_message", "assistant_message", "user_message"} {
		if _, err := conversation.AppendEvent(ctx, f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: kind, ActorType: "user", Payload: map[string]any{"content": "history", "authorization_scope": scope}, Classification: "internal"}); err != nil {
			t.Fatal(err)
		}
	}
	createSnapshot := func(revision int32, text string) db.ContextSnapshot {
		t.Helper()
		hash := sha256.Sum256([]byte(text))
		value, err := f.store.Queries.CreateContextSnapshot(ctx, db.CreateContextSnapshotParams{ID: uuid.New(), EnterpriseID: f.e, ConversationID: f.c, RunID: f.r, Revision: revision, SourceFromSequence: 1, SourceThroughSequence: 2, FirstKeptSequence: 3, TypedCheckpoint: checkpoint, NarrativeSummary: text, CompactionModelID: f.m, CompactionModelRevision: 1, PromptVersion: "test", SourceHash: hash[:], SnapshotHash: hash[:], Status: "active"})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	a := createSnapshot(1, "summary-version-A")
	loop := Loop{Store: f.store}
	messages, input, _, origin, err := loop.messages(ctx, f.run())
	if err != nil {
		t.Fatal(err)
	}
	if origin.snapshotID().UUID != a.ID || origin.ThroughSequence != 3 {
		t.Fatal("assembly did not capture its origin")
	}
	if err := f.store.Queries.SupersedeContextSnapshots(ctx, db.SupersedeContextSnapshotsParams{ConversationID: f.c, EnterpriseID: f.e}); err != nil {
		t.Fatal(err)
	}
	b := createSnapshot(2, "summary-version-B")
	if _, err := conversation.AppendEvent(ctx, f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, Type: "workspace_file_added", ActorType: "service", Payload: map[string]any{"name": "later"}, Classification: "internal"}); err != nil {
		t.Fatal(err)
	}
	model, err := f.store.Queries.GetAIModel(ctx, db.GetAIModelParams{ID: f.m, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	revision := db.AiModelRevision{ContextWindowTokens: 32768, MaxOutputTokens: 1024, InputPricePerMillion: model.InputPricePerMillion, OutputPricePerMillion: model.OutputPricePerMillion}
	projection, err := loop.context(ctx, f.run(), revision, messages, input, nil, origin)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(projection.Messages)
	if !strings.Contains(string(encoded), a.NarrativeSummary) || strings.Contains(string(encoded), b.NarrativeSummary) {
		t.Fatal("assembled messages switched snapshots")
	}
	step, err := f.store.Queries.CreateRunStep(ctx, db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	call, err := loop.createModelCall(ctx, f.run(), step, revision, projection, origin)
	if err != nil {
		t.Fatal(err)
	}
	provider := modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: "https://model.example.test"}
	actual, err := provider.RequestHash(modelprovider.Request{Model: "model", Messages: projection.Messages, MaxTokens: 1024})
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := hex.DecodeString(actual)
	if err := persistModelDispatch(ctx, f.store, db.MarkModelDispatchedParams{ID: call.ID, EnterpriseID: f.e, ProjectionHash: digest, CapabilitySnapshot: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	var hash, projectionHash []byte
	var through int64
	if err := f.store.Pool.QueryRow(ctx, "SELECT context_snapshot_id,context_snapshot_hash,context_through_sequence,projection_hash FROM model_calls WHERE id=$1", call.ID).Scan(&id, &hash, &through, &projectionHash); err != nil {
		t.Fatal(err)
	}
	if id != a.ID || !bytes.Equal(hash, a.SnapshotHash) || through != 3 || !bytes.Equal(projectionHash, digest) {
		t.Fatal("dispatch receipt changed the assembled context identity or range")
	}
	_, _, _, next, err := loop.messages(ctx, f.run())
	if err != nil || next.snapshotID().UUID != b.ID {
		t.Fatal("next assembly did not pick up the new snapshot")
	}
	if _, err := f.store.Pool.Exec(ctx, "UPDATE model_calls SET context_snapshot_hash=$2 WHERE id=$1", call.ID, b.SnapshotHash); err == nil {
		t.Fatal("database accepted a mismatched snapshot hash")
	}
}
