package agent

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPrivateRemoteActivityEvidenceMatchesActualModelProjection(t *testing.T) {
	f := newRecoveryFixture(t)
	projection, err := AssembleContext(ContextInput{ContextWindow: 32768, MaxOutput: 1024, System: modelprovider.Message{Role: "system", Content: "Use current authorized server facts."}, Checkpoint: map[string]any{"remote_execution": map[string]any{
		"concurrent_activity": map[string]any{"schema_version": "argus.remote_activity/v1", "status": "observed", "authorization_version": int64(7), "activities": []any{map[string]any{"command_summary": []string{"sleep (arguments withheld)"}}}},
		"executions":          []any{map[string]any{"stdout_preview": "private-self-output"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Messages) != 2 || !strings.Contains(projection.Messages[1].Content, "sleep (arguments withheld)") {
		t.Fatal("activity did not reach the provider message")
	}
	step, err := f.store.Queries.CreateRunStep(t.Context(), db.CreateRunStepParams{ID: uuid.New(), RunID: f.r, EnterpriseID: f.e, Sequence: 1, StepType: "model_call", Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := hex.DecodeString(projection.Hash)
	zero := pgtype.Numeric{Int: big.NewInt(0), Valid: true}
	call, err := f.store.Queries.CreateModelCall(t.Context(), db.CreateModelCallParams{ID: uuid.New(), EnterpriseID: f.e, RunID: f.r, StepID: step.ID, ModelID: f.m, ModelRevision: 1, CallKind: "inference", ProjectionHash: digest, InputPriceSnapshot: zero, OutputPriceSnapshot: zero})
	if err != nil {
		t.Fatal(err)
	}
	if err = recordRemoteModelContext(t.Context(), f.store.Queries, call, f.run(), projection); err != nil {
		t.Fatal(err)
	}
	var raw, recordedHash string
	var version int64
	if err = f.store.Pool.QueryRow(t.Context(), "SELECT activity::text,authorization_version,encode(projection_hash,'hex') FROM remote_execution_context_observations WHERE model_call_id=$1 AND enterprise_id=$2", call.ID, f.e).Scan(&raw, &version, &recordedHash); err != nil {
		t.Fatal(err)
	}
	if recordedHash != projection.Hash || version != 7 || !strings.Contains(raw, "sleep (arguments withheld)") || strings.Contains(raw, "private-self-output") {
		t.Fatal("private evidence does not match the selected activity projection")
	}
	provider := modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions, BaseURL: "https://model.example.test"}
	wireHash, err := provider.RequestHash(modelprovider.Request{Model: "actual-model", Messages: projection.Messages, MaxTokens: 1024})
	if err != nil {
		t.Fatal(err)
	}
	wireDigest, _ := hex.DecodeString(wireHash)
	if err = persistModelDispatch(t.Context(), f.store, db.MarkModelDispatchedParams{ID: call.ID, EnterpriseID: f.e, ProjectionHash: wireDigest, CapabilitySnapshot: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	var modelHash string
	if err = f.store.Pool.QueryRow(t.Context(), "SELECT encode(o.projection_hash,'hex'),encode(c.projection_hash,'hex') FROM remote_execution_context_observations o JOIN model_calls c ON c.id=o.model_call_id WHERE c.id=$1", call.ID).Scan(&recordedHash, &modelHash); err != nil {
		t.Fatal(err)
	}
	if recordedHash != wireHash || modelHash != wireHash {
		t.Fatalf("private evidence is not bound to the actual provider request: observation=%s model=%s wire=%s", recordedHash, modelHash, wireHash)
	}
	if f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1", f.c) != 0 {
		t.Fatal("activity evidence leaked into chat history")
	}
}
