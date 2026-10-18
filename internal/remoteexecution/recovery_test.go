package remoteexecution

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestOutputRetryNeverRepeatsKnownCommand(t *testing.T) {
	f := newFixture(t)
	f.transport.fileFailures.Store(1)
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "plan", f.input("first", "second"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(v)
	if err = f.run(v); err == nil {
		t.Fatal("injected transfer failure ignored")
	}
	view, err := f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Results[0].Status != "collecting" || view.Results[1].Status != "pending" || f.transport.batches.Load() != 1 {
		t.Fatalf("known result not preserved: %#v", view)
	}
	if err = f.run(v); err != nil {
		t.Fatal(err)
	}
	view, err = f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil || view.Status != "succeeded" || f.transport.batches.Load() != 2 {
		t.Fatalf("retry replayed command: %#v %v", view, err)
	}
	session, err := f.store.Queries.GetRemoteSession(t.Context(), db.GetRemoteSessionParams{ID: v.Spec.Steps[0].SessionID, EnterpriseID: f.e})
	if err != nil || session.ContextRevision != 2 {
		t.Fatalf("context advanced twice: %#v %v", session, err)
	}
}

func TestExhaustedOutputTransferPreservesSuccessfulEffect(t *testing.T) {
	f := newFixture(t)
	f.transport.fileFailures.Store(10)
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "plan", f.input("effect", "must-not-run"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(v)
	if err = f.run(v); err == nil {
		t.Fatal("expected transfer failure")
	}
	payload, _ := json.Marshal(Task{RequestID: v.ID, EnterpriseID: f.e})
	if err = f.service.HandleExhausted(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}, err); err != nil {
		t.Fatal(err)
	}
	view, err := f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != "partial_failed" || view.Results[0].Status != "succeeded" || view.Results[0].OutputsCollected || view.Results[1].Status != "skipped" || f.transport.batches.Load() != 1 {
		t.Fatalf("lost known success: %#v", view)
	}
	f.exec("UPDATE remote_execution_sessions SET last_active_at=now()-interval '1 hour' WHERE id=$1", v.Spec.Steps[0].SessionID)
	f.exec("UPDATE remote_execution_generations SET last_active_at=now()-interval '1 hour' WHERE session_id=$1", v.Spec.Steps[0].SessionID)
	if err = f.service.ReclaimIdle(t.Context()); err != nil {
		t.Fatal(err)
	}
	session, err := f.store.Queries.GetRemoteSession(t.Context(), db.GetRemoteSessionParams{ID: v.Spec.Steps[0].SessionID, EnterpriseID: f.e})
	if err != nil || session.Status == "reclaimed" {
		t.Fatal("reclaimed sole uncollected output", err)
	}
}

func TestStopBeforeDispatchAndUnknownStop(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_dispatch", true: "unknown_receipt"}[during], func(t *testing.T) {
			f := newFixture(t)
			call := f.call()
			v, err := f.service.Prepare(t.Context(), call, "plan", f.input("effect", "must-not-run"))
			if err != nil {
				t.Fatal(err)
			}
			f.approve(v)
			var completed chan error
			var release chan struct{}
			if during {
				f.transport.started = make(chan struct{}, 1)
				release = make(chan struct{})
				f.transport.release = release
				f.transport.stopSeen = make(chan struct{})
				f.transport.stopUnknown = true
				f.transport.unknown = 1
				completed = make(chan error, 1)
				go func() { completed <- f.run(v) }()
				select {
				case <-f.transport.started:
				case <-time.After(5 * time.Second):
					t.Fatal("batch did not start")
				}
			}
			if _, err = f.service.Stop(t.Context(), call.Principal, v.ID); err != nil {
				t.Fatal(err)
			}
			if during {
				select {
				case <-f.transport.stopSeen:
				case <-time.After(5 * time.Second):
					t.Fatal("stop not sent")
				}
				close(release)
				select {
				case err = <-completed:
				case <-time.After(5 * time.Second):
					t.Fatal("worker did not reconcile unknown")
				}
			} else {
				err = f.run(v)
			}
			if err != nil {
				t.Fatal(err)
			}
			view, err := f.service.Get(t.Context(), call.Principal, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			if during {
				if view.Status != "result_unknown" || view.StopStatus == "confirmed" || f.transport.batches.Load() != 1 {
					t.Fatalf("false stop confirmation: %#v", view)
				}
			} else if view.Status != "cancelled" || f.transport.batches.Load() != 0 {
				t.Fatalf("executed after stop: %#v", view)
			}
			if view.Results[1].Status != "skipped" {
				t.Fatal("later step dispatched")
			}
		})
	}
}

func TestFeedbackInvalidatesOldApprovalAndCreatesNewRevision(t *testing.T) {
	f := newFixture(t)
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "plan", f.input("old"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec("UPDATE runs SET status='waiting_input' WHERE id=$1", f.r)
	key := uuid.NewString()
	for range 2 {
		if _, err = f.service.Feedback(t.Context(), call.Principal, v.ID, "use the revised command", key); err != nil {
			t.Fatal(err)
		}
	}
	old, err := f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil || old.Status != "superseded" {
		t.Fatal("old approval still valid", err)
	}
	in := f.input("new")
	next, err := f.service.Prepare(t.Context(), f.call(), "plan", in)
	if err != nil || next.Revision != 2 || next.Status != "awaiting_confirmation" || f.transport.batches.Load() != 0 {
		t.Fatalf("revision reused approval: %#v %v", next, err)
	}
}

func TestPreviewIdempotencyChecksOwnerAndContent(t *testing.T) {
	f := newFixture(t)
	call := f.call()
	if _, err := f.service.Prepare(t.Context(), call, "connect", f.input("original")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Prepare(t.Context(), call, "connect", f.input("replacement")); !errorsIsCode(err, "REMOTE_COMMAND_CONFLICT") {
		t.Fatal("tool call ID reused with changed script", err)
	}
}

func TestUnknownExecutionCanBeStoppedWithoutReplayingCommand(t *testing.T) {
	f := newFixture(t)
	f.transport.unknown = 1
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "connect", f.input("effect"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(v)
	if err = f.run(v); err != nil {
		t.Fatal(err)
	}
	stopping, err := f.service.Stop(t.Context(), call.Principal, v.ID)
	if err != nil || stopping.Status != "stopping" {
		t.Fatalf("unknown cannot be stopped: %#v %v", stopping, err)
	}
	var controls int
	if err = f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM runtime_tasks WHERE enterprise_id=$1 AND queue=$2 AND payload->>'operation'='stop'", f.e, ControlQueue).Scan(&controls); err != nil || controls != 1 {
		t.Fatalf("stop did not reserve its independent control queue: %d %v", controls, err)
	}
	payload, _ := json.Marshal(Task{Operation: "stop", RequestID: v.ID, EnterpriseID: f.e})
	if err = f.service.Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Queue: ControlQueue, Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	after, err := f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "cancelled" || after.StopStatus != "confirmed" || after.Results[0].Status != "result_unknown" || !after.Results[0].Terminated || f.transport.batches.Load() != 1 {
		t.Fatalf("stop rewrote effects or replayed: %#v", after)
	}
}

func TestRemoteControlQueueCannotDispatchAnExecution(t *testing.T) {
	f := newFixture(t)
	payload, _ := json.Marshal(Task{RequestID: uuid.New(), EnterpriseID: f.e})
	if err := f.service.Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Queue: ControlQueue, Payload: payload}}); err == nil || f.transport.batches.Load() != 0 {
		t.Fatalf("control capacity accepted an execution: %v", err)
	}
}

func TestLateReceiptPreservesNewGenerationAndDoesNotResumeOldPlan(t *testing.T) {
	f := newFixture(t)
	f.transport.unknown = 1
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "plan", f.input("effect", "must-not-run"))
	if err != nil {
		t.Fatal(err)
	}
	f.approve(v)
	if err = f.run(v); err != nil {
		t.Fatal(err)
	}
	in := f.input("inspect existing effect")
	in.RecoveryOfID = &v.ID
	next, err := f.service.Prepare(t.Context(), f.call(), "connect", in)
	if err != nil {
		t.Fatal(err)
	}
	if next.Spec.Steps[0].Generation == v.Spec.Steps[0].Generation {
		t.Fatal("recovery reused lost generation")
	}
	req, err := f.store.Queries.GetRemoteRequest(t.Context(), db.GetRemoteRequestParams{ID: v.ID, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	step := v.Spec.Steps[0]
	binding := batchBinding(req, step, 1, 0)
	payload, _ := protojson.Marshal(binding)
	commandID := v.ID.String() + "/batch/" + step.ID.String()
	_, err = f.store.Queries.CreateConnectorCommand(t.Context(), db.CreateConnectorCommandParams{ID: uuid.New(), CommandID: commandID, EnterpriseID: f.e, ConnectorID: f.connector, ConnectionEpoch: 1, CommandType: "remote_execution_batch", OperationRef: commandID, PayloadSchemaVersion: SchemaVersion, Payload: payload, PayloadHash: make([]byte, 32), IdempotencyKey: commandID, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	result, _ := protojson.Marshal(&connectorv1.RemoteExecutionBatchResult{RequestId: v.ID.String(), SessionId: step.SessionID.String(), SessionGeneration: step.Generation.String(), ContextRevision: 1, Sequence: 1, ExitCode: 0, Terminated: true, Stdout: []byte("effect already happened")})
	f.exec("UPDATE connector_commands SET status='succeeded',result=$2 WHERE command_id=$1", commandID, result)
	if err = f.service.ReconcileLateReceipts(t.Context()); err != nil {
		t.Fatal(err)
	}
	collect, _ := json.Marshal(Task{Operation: "collect", RequestID: v.ID, EnterpriseID: f.e, StepID: step.ID})
	if err = f.service.Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: collect}}); err != nil {
		t.Fatal(err)
	}
	after, err := f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "paused_for_recovery" || after.Results[0].Status != "succeeded" || after.Results[1].Status != "skipped" || f.transport.batches.Load() != 1 {
		t.Fatalf("late receipt resumed plan: %#v", after)
	}
	current, err := f.store.Queries.GetRemoteSession(t.Context(), db.GetRemoteSessionParams{ID: step.SessionID, EnterpriseID: f.e})
	if err != nil || current.Generation != next.Spec.Steps[0].Generation || current.ContextRevision != 0 {
		t.Fatal("late receipt overwrote new interpreter", err)
	}
	var generations int
	if err = f.store.Pool.QueryRow(t.Context(), "SELECT count(*) FROM remote_execution_generations WHERE session_id=$1", step.SessionID).Scan(&generations); err != nil || generations != 2 {
		t.Fatal("old interpreter history was lost", err)
	}
}

func secondChat(f *fixture) *fixture {
	f.t.Helper()
	other := *f
	other.c = uuid.New()
	other.r = uuid.New()
	f.exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Second chat',$4)", other.c, f.e, f.u, f.m)
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'running')", other.r, other.c, f.e, f.u, f.m)
	return &other
}

func TestDifferentChatsExecuteConcurrentlyWithPrivateActivity(t *testing.T) {
	f := newFixture(t)
	other := secondChat(f)
	release := make(chan struct{})
	f.transport.release = release
	f.transport.started = make(chan struct{}, 2)
	first, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("systemctl status secret-service --password=hidden"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.service.Prepare(t.Context(), other.call(), "connect", other.input("ps secret-marker"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Spec.Steps[0].SessionID == second.Spec.Steps[0].SessionID {
		t.Fatal("cross-chat session shared")
	}
	f.approve(first)
	other.approve(second)
	completed := make(chan error, 2)
	go func() { completed <- f.run(first) }()
	go func() { completed <- other.run(second) }()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	for range 2 {
		select {
		case <-f.transport.started:
		case <-time.After(5 * time.Second):
			t.Fatal("resource execution was queued")
		}
	}
	run, err := f.store.Queries.GetRun(t.Context(), db.GetRunParams{ID: other.r, EnterpriseID: f.e})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := f.service.ActivityFacts(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(facts)
	if !strings.Contains(string(raw), "systemctl") || strings.Contains(string(raw), "secret-service") || strings.Contains(string(raw), "password") || strings.Contains(string(raw), "hidden") {
		t.Fatalf("unsafe or absent activity: %s", raw)
	}
	f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.e)
	facts, err = f.service.ActivityFacts(t.Context(), run)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(facts)
	if strings.Contains(string(raw), "systemctl") {
		t.Fatal("revoked activity leaked")
	}
	close(release)
	for range 2 {
		select {
		case err := <-completed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not finish")
		}
	}
	if f.transport.maximum.Load() != 2 {
		t.Fatal("same-resource concurrency absent")
	}
}
