package remoteexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/action"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/conversation"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/runtime"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type fixture struct {
	t                                    *testing.T
	store                                *postgres.Store
	service                              *Service
	transport                            *controlledTransport
	e, d, u, m, c, r, h, connector, role uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("ARGUS_P7_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable ARGUS_P7_TEST_DATABASE_URL")
	}
	store, err := postgres.Open(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	f := &fixture{t: t, store: store, e: uuid.New(), d: uuid.New(), u: uuid.New(), m: uuid.New(), c: uuid.New(), r: uuid.New(), h: uuid.New(), connector: uuid.New(), role: uuid.New()}
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Plan7 fixture',$2,'UTC')", f.e, "p7-"+f.e.String())
	if err = audit.InitializeChain(t.Context(), store.Queries, "enterprise", uuid.NullUUID{UUID: f.e, Valid: true}); err != nil {
		t.Fatal(err)
	}
	f.exec("INSERT INTO departments(id,enterprise_id,name,is_default) VALUES($1,$2,'Default',true)", f.d, f.e)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,$4,'Plan7 user')", f.u, f.e, f.d, "p7-"+f.u.String())
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Plan7 operator')", f.role, f.e)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) SELECT $1,id FROM permissions", f.role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), f.e, f.u, f.role)
	f.exec("INSERT INTO ai_models(id,enterprise_id,name,base_url,model_id,api_protocol,context_window_tokens,max_output_tokens,input_price_per_million,output_price_per_million,health_status) VALUES($1,$2,'Fixture','https://model.example.test','fixture','chat_completions',32768,1024,0,0,'healthy')", f.m, f.e)
	f.exec("INSERT INTO conversations(id,enterprise_id,owner_user_id,title,selected_model_id) VALUES($1,$2,$3,'Plan7 fixture',$4)", f.c, f.e, f.u, f.m)
	f.exec("INSERT INTO runs(id,conversation_id,enterprise_id,actor_user_id,model_id,model_revision,locale,authorization_version,status) VALUES($1,$2,$3,$4,$5,1,'zh-CN',1,'running')", f.r, f.c, f.e, f.u, f.m)
	f.exec("INSERT INTO hosts(id,enterprise_id,name,platform,environment,labels_hash,role,control_path) VALUES($1,$2,$3,'linux','development',$4,'managed_host','direct')", f.h, f.e, "p7-"+f.h.String(), bytes.Repeat([]byte{1}, 32))
	f.exec("INSERT INTO connectors(id,enterprise_id,role,host_id,name,instance_id,device_fingerprint_hash,public_key_hash,certificate_expires_at,status,connection_epoch) VALUES($1,$2,'host',$3,'Plan7 connector',$4,$5,$5,now()+interval '1 day','online',1)", f.connector, f.e, f.h, uuid.NewString(), bytes.Repeat([]byte{2}, 32))
	f.exec("UPDATE hosts SET connector_id=$2 WHERE id=$1", f.h, f.connector)
	f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'host',$4)", uuid.New(), f.e, f.u, f.h)
	f.exec("INSERT INTO remote_execution_identities(id,enterprise_id,resource_type,resource_id,username,platform,connector_id,binding_source,resource_version) VALUES($1,$2,'host',$3,'fixture-account','linux',$4,'verified_local_account',1)", uuid.New(), f.e, f.h, f.connector)
	actions := resource.PendingActionService{Store: store, Idempotency: postgres.Idempotency{Key: bytes.Repeat([]byte{3}, 32)}, Key: bytes.Repeat([]byte{4}, 32)}
	transport := &controlledTransport{store: store}
	f.transport = transport
	f.service = &Service{Store: store, Actions: actions, Transport: transport}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := store.Pool.Exec(ctx, "UPDATE runtime_tasks SET status='succeeded',lease_owner=NULL,lease_until=NULL WHERE enterprise_id=$1", f.e); err != nil {
			t.Error(err)
		}
	})
	return f
}
func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.store.Pool.Exec(f.t.Context(), sql, args...); err != nil {
		f.t.Fatal(err)
	}
}
func (f *fixture) call() toolruntime.Invocation {
	p, err := (conversation.Service{Store: f.store}).ToolPrincipal(f.t.Context(), f.e, f.u, f.c)
	if err != nil {
		f.t.Fatal(err)
	}
	return toolruntime.Invocation{Principal: p, ID: uuid.New(), RunID: f.r}
}
func (f *fixture) input(scripts ...string) Input {
	steps := []Step{}
	for _, script := range scripts {
		steps = append(steps, Step{Description: "Fixture step", Target: Target{ResourceType: "host", ResourceID: f.h}, Interpreter: "bash", Script: script})
	}
	return Input{Title: "Fixture plan", Description: "Explicitly approved test commands", Steps: steps}
}
func (f *fixture) approve(v View) (*db.Execution, action.Executor) {
	f.t.Helper()
	resources := resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}
	workflow := action.Service{Store: f.store, Idempotency: f.service.Actions.Idempotency, Resources: resources}
	confirmed, err := workflow.Confirm(f.t.Context(), f.u.String(), uuid.NewString(), f.e, 1, false, v.ActionRef, uuid.NewString())
	if err != nil {
		f.t.Fatal(err)
	}
	executor := action.Executor{Store: f.store, Resources: resources}
	encoded, _ := json.Marshal(action.ExecutionTask{ExecutionID: confirmed.Execution.ID, EnterpriseID: f.e})
	if err = executor.Handle(f.t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: encoded}}); err != nil {
		f.t.Fatal(err)
	}
	return confirmed.Execution, executor
}
func (f *fixture) run(v View) error {
	payload, _ := json.Marshal(Task{RequestID: v.ID, EnterpriseID: f.e})
	return f.service.Handle(f.t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}})
}

type controlledTransport struct {
	store        *postgres.Store
	batches      atomic.Int32
	active       atomic.Int32
	maximum      atomic.Int32
	failure      int32
	unknown      int32
	started      chan struct{}
	release      <-chan struct{}
	fileFailures atomic.Int32
	stopUnknown  bool
	stopSeen     chan struct{}
}

func (t *controlledTransport) Call(ctx context.Context, identity db.RemoteExecutionIdentity, kind string, payload proto.Message, key string, _ time.Duration, queued func(context.Context, *db.Queries, string) error) (json.RawMessage, error) {
	if queued != nil {
		if err := retryStateTx(ctx, t.store, func(q *db.Queries) error { return queued(ctx, q, key) }); err != nil {
			return nil, err
		}
	}
	var result proto.Message
	switch p := payload.(type) {
	case *connectorv1.RemoteExecutionControl:
		if p.Operation == "stop" {
			if t.stopSeen != nil {
				close(t.stopSeen)
			}
			if t.stopUnknown {
				return nil, Error("REMOTE_RESULT_UNKNOWN")
			}
			return protojson.Marshal(&connectorv1.RemoteExecutionControlResult{SessionId: p.SessionId, SessionGeneration: p.SessionGeneration, Status: "stopped"})
		}
		result = &connectorv1.RemoteExecutionControlResult{SessionId: p.SessionId, SessionGeneration: p.SessionGeneration, ContextRevision: p.Binding.ContextRevision, Username: identity.Username, IdentitySid: identity.IdentitySid, Status: "active", RuntimeAccount: "fixture-account", InterpreterVersion: "fixture-bash"}
	case *connectorv1.RemoteExecutionFile:
		if p.Operation == "list_output" && t.fileFailures.Add(-1) >= 0 {
			return nil, Error("REMOTE_TRANSFER_FAILED")
		}
		result = &connectorv1.RemoteExecutionFileResult{Complete: true}
	case *connectorv1.RemoteExecutionBatch:
		count := t.batches.Add(1)
		active := t.active.Add(1)
		defer t.active.Add(-1)
		for old := t.maximum.Load(); active > old; old = t.maximum.Load() {
			if t.maximum.CompareAndSwap(old, active) {
				break
			}
		}
		if t.started != nil {
			t.started <- struct{}{}
		}
		if t.release != nil {
			select {
			case <-ctx.Done():
				return nil, Error("REMOTE_RESULT_UNKNOWN")
			case <-t.release:
			}
		}
		if count == t.unknown {
			return nil, Error("REMOTE_RESULT_UNKNOWN")
		}
		exit := int32(0)
		if count == t.failure {
			exit = 1
		}
		result = &connectorv1.RemoteExecutionBatchResult{RequestId: p.RequestId, SessionId: p.SessionId, SessionGeneration: p.SessionGeneration, ContextRevision: p.ContextRevision + 1, Sequence: p.Sequence, ExitCode: exit, Stdout: []byte("controlled test receipt"), WorkingDirectory: "/fixture", Terminated: true}
	default:
		return nil, Error("REMOTE_INPUT_INVALID")
	}
	return protojson.Marshal(result)
}

func TestConfirmationSerialExecutionAndNextBatchApproval(t *testing.T) {
	f := newFixture(t)
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "plan", f.input("first", "second"))
	if err != nil {
		t.Fatal(err)
	}
	if f.transport.batches.Load() != 0 || v.Status != "awaiting_confirmation" {
		t.Fatal("preview executed a command")
	}
	repeated, err := f.service.Prepare(t.Context(), call, "plan", f.input("first", "second"))
	if err != nil || repeated.ID != v.ID {
		t.Fatal("preview replay changed identity")
	}
	execution, executor := f.approve(v)
	if f.transport.batches.Load() != 0 {
		t.Fatal("confirmation executed outside background worker")
	}
	if err = f.run(v); err != nil {
		t.Fatal(err)
	}
	after, err := f.service.Get(t.Context(), call.Principal, v.ID)
	if err != nil || after.Status != "succeeded" || f.transport.batches.Load() != 2 || f.transport.maximum.Load() != 1 {
		t.Fatalf("serial plan: %#v %v", after, err)
	}
	if err = f.run(v); err != nil || f.transport.batches.Load() != 2 {
		t.Fatal("worker replay duplicated a batch")
	}
	payload, _ := json.Marshal(action.ExecutionTask{ExecutionID: execution.ID, EnterpriseID: f.e})
	if err = executor.Handle(t.Context(), runtime.Task{RuntimeTask: db.RuntimeTask{Payload: payload}}); err != nil {
		t.Fatal(err)
	}
	next, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("third"))
	if err != nil {
		t.Fatal(err)
	}
	if next.Status != "awaiting_confirmation" || next.Spec.Steps[0].SessionID != v.Spec.Steps[0].SessionID || next.Spec.Steps[0].ContextRevision != 2 || f.transport.batches.Load() != 2 {
		t.Fatal("session reuse reused consent or lost context")
	}
}
func TestFailureAndUnknownNeverDispatchRemainingSteps(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "unknown"}[unknown], func(t *testing.T) {
			f := newFixture(t)
			if unknown {
				f.transport.unknown = 1
			} else {
				f.transport.failure = 1
			}
			call := f.call()
			v, err := f.service.Prepare(t.Context(), call, "plan", f.input("first", "must-not-run"))
			if err != nil {
				t.Fatal(err)
			}
			f.approve(v)
			if err = f.run(v); err != nil {
				t.Fatal(err)
			}
			view, err := f.service.Get(t.Context(), call.Principal, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			if f.transport.batches.Load() != 1 || view.Status == "succeeded" || view.Results[1].Status != "skipped" {
				t.Fatalf("unapproved continuation: %#v", view)
			}
		})
	}
}
func TestChangedContextAndRevocationInvalidateApproval(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "context", true: "grant"}[revoke], func(t *testing.T) {
			f := newFixture(t)
			v, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("inspect"))
			if err != nil {
				t.Fatal(err)
			}
			if revoke {
				f.exec("DELETE FROM data_authorization_grants WHERE enterprise_id=$1", f.e)
			} else {
				f.exec("UPDATE remote_execution_sessions SET context_revision=1 WHERE id=$1", v.Spec.Steps[0].SessionID)
			}
			resources := resource.Service{Store: f.store, Actions: f.service.Actions, Extension: f.service}
			workflow := action.Service{Store: f.store, Idempotency: f.service.Actions.Idempotency, Resources: resources}
			if _, err = workflow.Confirm(t.Context(), f.u.String(), uuid.NewString(), f.e, 1, false, v.ActionRef, uuid.NewString()); err == nil {
				t.Fatal("stale approval accepted")
			}
			if f.transport.batches.Load() != 0 {
				t.Fatal("invalidated command executed")
			}
		})
	}
}
func TestCrossConversationAndTenantIsolation(t *testing.T) {
	f := newFixture(t)
	call := f.call()
	v, err := f.service.Prepare(t.Context(), call, "connect", f.input("inspect"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []toolruntime.Principal{{EnterpriseID: uuid.New(), UserID: f.u, ConversationID: f.c}, {EnterpriseID: f.e, UserID: uuid.New(), ConversationID: f.c}, {EnterpriseID: f.e, UserID: f.u, ConversationID: uuid.New()}} {
		if _, err = f.service.Get(t.Context(), p, v.ID); err == nil {
			t.Fatal("cross-scope view accepted")
		}
	}
}
func TestManualHostHasNoImplicitExecutionAccount(t *testing.T) {
	f := newFixture(t)
	f.exec("DELETE FROM remote_execution_identities WHERE enterprise_id=$1", f.e)
	_, err := f.service.Prepare(t.Context(), f.call(), "connect", f.input("id"))
	if err == nil {
		t.Fatal("manual host inherited a service/root account")
	}
	if !errorsIsCode(err, "REMOTE_IDENTITY_UNAVAILABLE") {
		t.Fatal(err)
	}
}
func errorsIsCode(err error, code string) bool { return err != nil && err.Error() == code }

var _ = pgx.ErrNoRows
