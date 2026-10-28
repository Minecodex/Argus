package httpapi

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	auditapi "github.com/kakj-go/Argus/internal/gen/openapi/audit"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresAuditWorkflowFinalResultCorrelationAndReplay(t *testing.T) {
	f := newAuditWorkflowFixture(t)
	ref, id := f.prepare("audit-renamed")
	before := f.call("GET", "/enterprise/audit-events?operation_ref="+ref, nil, 200, "")
	var initial auditapi.AuditEventPage
	if err := json.Unmarshal(before, &initial); err != nil {
		t.Fatal(err)
	}
	if len(initial.Items) != 2 {
		t.Fatalf("expected preview and confirmation, got %d", len(initial.Items))
	}
	for _, event := range initial.Items {
		if event.Operation == nil || event.Operation.ExecutionStatus == nil || *event.Operation.ExecutionStatus != "pending" {
			t.Fatal("confirmation treated as execution success")
		}
	}
	if err := f.run(id); err != nil {
		t.Fatal(err)
	}
	if err := f.run(id); err != nil {
		t.Fatal("replayed worker", err)
	}
	f.session()
	var completed auditapi.AuditEventPage
	raw := f.call("GET", "/enterprise/audit-events?operation_ref="+ref, nil, 200, "")
	if err := json.Unmarshal(raw, &completed); err != nil {
		t.Fatal(err)
	}
	terminals := 0
	for _, event := range completed.Items {
		if event.Operation == nil || event.Operation.ActionRef != ref || event.Operation.ExecutionStatus == nil || *event.Operation.ExecutionStatus != "succeeded" {
			t.Fatal("operation projection missing final status")
		}
		if event.Action == "execution.succeeded" {
			terminals++
		}
		for _, old := range initial.Items {
			if old.Id == event.Id && (old.EventHash != event.EventHash || old.PreviousHash != event.PreviousHash || old.Result != event.Result) {
				t.Fatal("immutable event changed")
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal event count %d", terminals)
	}
	host, err := f.store.Queries.GetHost(t.Context(), db.GetHostParams{ID: f.host, EnterpriseID: f.enterprise})
	if err != nil || host.Name != "audit-renamed" {
		t.Fatal("real host mutation missing", err)
	}
	f.call("GET", "/enterprise/executions?action_ref="+ref, nil, 200, "")
	resourceEvents := f.call("GET", "/enterprise/audit-events?resource_id="+f.host.String(), nil, 200, "")
	if !bytes.Contains(resourceEvents, []byte("execution.succeeded")) || !bytes.Contains(resourceEvents, []byte("pending_action.confirm")) {
		t.Fatal("resource history lost operation stages")
	}
	failedRef, failedID := f.prepare("must-not-apply")
	// Authorization is revoked after confirmation; the real executor must fail closed.
	f.exec("UPDATE enterprise_users SET authorization_version=authorization_version+1 WHERE id=$1", f.user)
	if err = f.run(failedID); err != nil {
		t.Fatal(err)
	}
	f.session()
	raw = f.call("GET", "/enterprise/audit-events?operation_ref="+failedRef, nil, 200, "")
	if !bytes.Contains(raw, []byte(`"execution_status":"failed"`)) || !bytes.Contains(raw, []byte("ACTION_INVALIDATED")) || !bytes.Contains(raw, []byte("execution.failed")) {
		t.Fatal("failed terminal outcome missing", string(raw))
	}
	raw = f.call("GET", "/enterprise/executions?resource_id="+uuid.NewString(), nil, 200, "")
	if !bytes.Contains(raw, []byte(`"items":[]`)) {
		t.Fatal("resource execution filter leaked unrelated items")
	}
	// A signed cursor cannot be reused for another operation.
	var page auditapi.AuditEventPage
	_ = json.Unmarshal(f.call("GET", "/enterprise/audit-events?operation_ref="+ref+"&limit=1", nil, 200, ""), &page)
	if page.Page.NextCursor == nil {
		t.Fatal("expected bounded operation cursor")
	}
	f.call("GET", "/enterprise/audit-events?operation_ref="+failedRef+"&limit=1&cursor="+*page.Page.NextCursor, nil, 400, "")
	// Same authenticated identity switched to a different tenant has no access.
	other := uuid.New()
	f.exec("INSERT INTO enterprises(id,name,code,timezone) VALUES($1,'Other audit tenant',$2,'UTC')", other, other.String())
	department, user, role := uuid.New(), uuid.New(), uuid.New()
	f.exec("INSERT INTO departments(id,enterprise_id,name) VALUES($1,$2,'Other')", department, other)
	f.exec("INSERT INTO enterprise_users(id,enterprise_id,department_id,username,display_name) VALUES($1,$2,$3,'other-operator','Other operator')", user, other, department)
	f.exec("INSERT INTO roles(id,enterprise_id,name) VALUES($1,$2,'Other reader')", role, other)
	f.exec("INSERT INTO role_permissions(role_id,permission_id) VALUES($1,'audit.read'),($1,'execution.read')", role)
	f.exec("INSERT INTO role_bindings(id,enterprise_id,subject_type,subject_id,role_id) VALUES($1,$2,'user',$3,$4)", uuid.New(), other, user, role)
	f.enterprise, f.department, f.user = other, department, user
	f.session()
	f.call("GET", "/enterprise/executions/"+id.String(), nil, 404, "")
	raw = f.call("GET", "/enterprise/audit-events?operation_ref="+ref, nil, 200, "")
	if !bytes.Contains(raw, []byte(`"items":[]`)) {
		t.Fatal("cross-tenant audit association leaked")
	}
}
