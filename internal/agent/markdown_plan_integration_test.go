package agent

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/interaction"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestMarkdownPlanConfirmationAtomicallyActivatesExactCommands(t *testing.T) {
	f, s, p, set, _ := interactionFixture(t)
	host, connector := uuid.New(), uuid.New()
	f.exec("INSERT INTO hosts(id,enterprise_id,name,platform,environment,labels_hash,role,control_path) VALUES($1,$2,'Markdown fixture','linux','development',$3,'managed_host','direct')", host, f.e, bytes.Repeat([]byte{1}, 32))
	f.exec("INSERT INTO connectors(id,enterprise_id,role,host_id,name,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at,status,connection_epoch) VALUES($1,$2,'host',$3,'Fixture connector',$4,$5,$5,now()+interval '1 day','online',1)", connector, f.e, host, uuid.NewString(), bytes.Repeat([]byte{2}, 32))
	f.exec("UPDATE hosts SET connector_id=$2 WHERE id=$1", host, connector)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), f.e, f.u, host)
	f.exec("INSERT INTO remote_execution_identities(id,enterprise_id,resource_type,resource_id,username,platform,connector_id,binding_source,resource_version) VALUES($1,$2,'host',$3,'fixture-account','linux',$4,'verified_local_account',1)", uuid.New(), f.e, host, connector)
	plan := interaction.Plan{Title: "Repair fixture", Markdown: "# Repair fixture\n\nPreserve the original.\n\n```bash argus:backup\nprintf 'backup\\n'\n```\n", CommandBindings: []remoteexecution.CommandBinding{{BlockID: "backup", Description: "Back up", Target: remoteexecution.Target{ResourceType: "host", ResourceID: host}, Interpreter: "bash"}}}
	result, _, cancel := runInteractionCall(t, f, set, p, 1, "tool.invoke", map[string]any{"category": "workflow", "name": "plan.propose", "arguments": plan}, true)
	cancel()
	if len(result) != 1 || !result[0].paused {
		t.Fatalf("plan did not pause: %#v", result)
	}
	items, err := s.List(t.Context(), p)
	if err != nil || len(items) != 1 {
		t.Fatalf("missing plan: %v", err)
	}
	draft := items[0]
	if f.count("SELECT count(*) FROM plan_command_authorizations WHERE plan_id=$1 AND implementation_run_id IS NULL", draft.ID) != 1 || f.count("SELECT count(*) FROM remote_execution_requests WHERE conversation_id=$1", f.c) != 0 {
		t.Fatal("preview granted or executed commands")
	}
	raw, _ := json.Marshal(draft.Content)
	if bytes.Contains(raw, []byte(`"execution"`)) || bytes.Contains(raw, []byte(`"request_hash"`)) {
		t.Fatal("private manifest leaked")
	}
	response := interaction.Response{Action: "confirm", Version: draft.Version, ContentHash: draft.ContentHash}
	f.exec("UPDATE hosts SET resource_version=2 WHERE id=$1", host)
	if _, err = s.Respond(t.Context(), p, draft.ID, response, uuid.NewString(), "zh-CN"); err == nil {
		t.Fatal("approved changed target")
	}
	if f.count("SELECT count(*) FROM runs WHERE approved_plan_id=$1", draft.ID) != 0 || f.count("SELECT count(*) FROM conversation_interactions WHERE id=$1 AND status='pending'", draft.ID) != 1 {
		t.Fatal("failed confirmation partially committed")
	}
	f.exec("UPDATE hosts SET resource_version=1 WHERE id=$1", host)
	var wg sync.WaitGroup
	errors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.Respond(t.Context(), p, draft.ID, response, uuid.NewString(), "zh-CN")
			errors <- e
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	confirmed, err := s.Get(t.Context(), p, draft.ID)
	if err != nil || confirmed.ImplementationRunID == nil {
		t.Fatalf("missing implementation: %v", err)
	}
	grants, err := f.store.Queries.ListPlanCommandAuthorizations(t.Context(), db.ListPlanCommandAuthorizationsParams{PlanID: draft.ID, EnterpriseID: f.e})
	if err != nil || len(grants) != 1 || grants[0].ImplementationRunID.UUID != *confirmed.ImplementationRunID || grants[0].UsedRequestID.Valid {
		t.Fatalf("incorrect activation: %v %#v", err, grants)
	}
	if f.count("SELECT count(*) FROM runs WHERE approved_plan_id=$1", draft.ID) != 1 || f.count("SELECT count(*) FROM conversation_events WHERE conversation_id=$1 AND event_type='user_message' AND payload->>'content'='是的，执行此计划'", f.c) != 1 {
		t.Fatal("confirmation was duplicated")
	}
}
