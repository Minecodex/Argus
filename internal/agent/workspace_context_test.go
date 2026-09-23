package agent

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/config"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/presentation"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"github.com/kakj-go/Argus/internal/workspace"
)

func TestWorkspaceLifecycleAndFileReferencesReachActualModelRequest(t *testing.T) {
	f := newRecoveryFixture(t)
	ctx := t.Context()
	loop := Loop{Store: f.store}
	f.exec("UPDATE runs SET status='succeeded' WHERE id=$1", f.r)
	principal := toolruntime.Principal{EnterpriseID: f.e, UserID: f.u, ConversationID: f.c, AuthorizationVersion: 1, Permissions: []string{"workspace.use"}}
	service := workspace.Service{Store: f.store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{9}, 32)}, Config: config.Workspace{Enabled: true}}
	createWorkspace := func() uuid.UUID {
		t.Helper()
		id := uuid.New()
		f.exec("INSERT INTO workspaces(id,enterprise_id,conversation_id,status,pvc_name,namespace,capacity_bytes,environment_version) VALUES($1,$2,$3,'ready',$4,'private-review-namespace',2147483648,'private-runtime-identity')", id, f.e, f.c, "private-pvc-"+id.String())
		return id
	}
	createFile := func(id uuid.UUID) db.WorkspaceFile {
		t.Helper()
		file := uuid.New()
		f.exec("INSERT INTO workspace_files(id,enterprise_id,workspace_id,conversation_id,name,path,byte_size,content_hash,media_type) VALUES($1,$2,$3,$4,'input.csv','input.csv',24,repeat('0',64),'text/csv')", file, f.e, id, f.c)
		value, err := f.store.Queries.GetWorkspaceFile(ctx, db.GetWorkspaceFileParams{ID: file, EnterpriseID: f.e})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	firstID := createWorkspace()
	firstFile := createFile(firstID)
	appendMessage := func(file db.WorkspaceFile) db.ConversationEvent {
		t.Helper()
		value, err := conversation.AppendEvent(ctx, f.store.Queries, conversation.EventInput{EnterpriseID: f.e, ConversationID: f.c, RunID: uuid.NullUUID{UUID: f.r, Valid: true}, Type: "user_message", ActorType: "user", Payload: map[string]any{"content": "Analyze my file", "files": []map[string]any{conversation.FileReference(file)}}, Classification: "internal"})
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	appendMessage(firstFile)
	scope, err := presentation.Scope(ctx, f.store, f.e, f.u)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, _ := json.Marshal(scopedCheckpoint{AuthorizationScope: scope, Execution: json.RawMessage(`{}`)})
	_, err = f.store.Queries.CreateContextSnapshot(ctx, db.CreateContextSnapshotParams{ID: uuid.New(), EnterpriseID: f.e, ConversationID: f.c, RunID: f.r, Revision: 1, SourceFromSequence: 1, SourceThroughSequence: 1, FirstKeptSequence: 2, TypedCheckpoint: checkpoint, NarrativeSummary: "Earlier workspace is ready and /workspace/input.csv exists.", CompactionModelID: f.m, CompactionModelRevision: 1, PromptVersion: "test", SourceHash: make([]byte, 32), SnapshotHash: make([]byte, 32), Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	original := appendMessage(firstFile)
	request := func(run db.Run, status, id string, available map[string]bool) string {
		t.Helper()
		messages, input, _, source, err := loop.messages(ctx, run)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := loop.context(ctx, run, db.AiModelRevision{ContextWindowTokens: 32768, MaxOutputTokens: 1024}, messages, input, nil, source)
		if err != nil {
			t.Fatal(err)
		}
		var facts struct {
			Workspace conversation.WorkspaceContext `json:"workspace"`
		}
		prefix := "Server execution facts (data): "
		if err := json.Unmarshal([]byte(strings.TrimPrefix(projection.Messages[1].Content, prefix)), &facts); err != nil {
			t.Fatal(err)
		}
		if facts.Workspace.Status != status || facts.Workspace.ID != id {
			t.Fatalf("workspace facts=%+v", facts.Workspace)
		}
		seen := map[string]bool{}
		for _, message := range projection.Messages {
			if _, raw, ok := strings.Cut(message.Content, "\nWorkspace attachments: "); ok {
				var files []map[string]any
				if err := json.Unmarshal([]byte(raw), &files); err != nil {
					t.Fatal(err)
				}
				for _, file := range files {
					key := file["id"].(string)
					state, _ := file["available"].(bool)
					if expected, exists := available[key]; exists && expected != state {
						t.Fatalf("reference %s availability=%t", key, state)
					}
					if !state && file["path"] != nil {
						t.Fatal("unavailable reference retained a usable path")
					}
					seen[key] = true
				}
			}
		}
		for key := range available {
			if !seen[key] {
				t.Fatalf("missing reference %s", key)
			}
		}
		wire, _ := json.Marshal(projection.Messages)
		for _, private := range []string{"private-review-namespace", "private-pvc-", "private-runtime-identity"} {
			if bytes.Contains(wire, []byte(private)) {
				t.Fatal("private runtime metadata reached model")
			}
		}
		hash, err := (modelprovider.Provider{Protocol: modelprovider.ProtocolChatCompletions}).RequestHash(modelprovider.Request{Model: "test", Messages: projection.Messages, MaxTokens: 1024})
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	before := request(f.run(), "ready", firstID.String(), map[string]bool{firstFile.ID.String(): true})
	part, err := service.BuildTools(ctx, principal)
	if err != nil || part.WorkspaceID != firstID.String() {
		t.Fatalf("capability snapshot lost identity: %v", err)
	}
	if _, err := service.Delete(ctx, principal, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	during := request(f.run(), "deleting", firstID.String(), map[string]bool{firstFile.ID.String(): false})
	if during == before {
		t.Fatal("delete intent did not change provider request")
	}
	f.exec("UPDATE workspaces SET status='deleted',deleted_at=now() WHERE id=$1", firstID)
	f.exec("UPDATE workspace_files SET deleted_at=now() WHERE workspace_id=$1", firstID)
	next := uuid.New()
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'running')", next, f.c, f.e, f.u, f.m)
	nextRun, err := f.store.Queries.GetRun(ctx, db.GetRunParams{ID: next, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	deleted := request(nextRun, "deleted", firstID.String(), map[string]bool{firstFile.ID.String(): false})
	if deleted == during {
		t.Fatal("completed deletion did not change context")
	}
	secondID := createWorkspace()
	secondFile := createFile(secondID)
	appendMessage(secondFile)
	recreated := request(nextRun, "ready", secondID.String(), map[string]bool{firstFile.ID.String(): false, secondFile.ID.String(): true})
	if recreated == deleted {
		t.Fatal("new workspace not represented")
	}
	var stored []byte
	if err := f.store.Pool.QueryRow(ctx, "SELECT payload FROM conversation_events WHERE id=$1", original.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, original.Payload) {
		t.Fatal("projection rewrote original history")
	}
}
