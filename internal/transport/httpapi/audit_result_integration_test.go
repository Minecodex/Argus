package httpapi

import (
	"bytes"
	"github.com/google/uuid"
	"testing"
)

func TestPostgresAuditResultClaimReplayExpiryAndSecretBoundary(t *testing.T) {
	f := newAuditWorkflowFixture(t)
	f.executor.Resources = auditCommandFixture{f.domain}
	ref, id := f.prepare("audit-command-target")
	if err := f.run(id); err != nil {
		t.Fatal(err)
	}
	f.session()
	path := "/enterprise/executions/" + id.String()
	metadata := f.call("GET", path, nil, 200, "")
	if bytes.Contains(metadata, []byte("claim-canary")) || !bytes.Contains(metadata, []byte(`"one_time_result_state":"available"`)) {
		t.Fatal("public execution exposed command or lost availability")
	}
	key := uuid.NewString()
	original := f.call("POST", path+"/one-time-result", nil, 200, key)
	replay := f.call("POST", path+"/one-time-result", nil, 200, key)
	if !bytes.Equal(original, replay) || !bytes.Contains(original, []byte("audit-acceptance-claim-canary")) {
		t.Fatal("one-time delivery replay changed result")
	}
	f.call("POST", path+"/one-time-result", nil, 409, uuid.NewString())
	events := f.call("GET", "/enterprise/audit-events?operation_ref="+ref, nil, 200, "")
	if bytes.Contains(events, []byte("claim-canary")) {
		t.Fatal("command entered public audit")
	}
	if bytes.Count(events, []byte(`"action":"execution.one_time_result.claim"`)) != 1 {
		t.Fatal("result claim audit duplicated or unlinked")
	}
	_, expired := f.prepare("audit-expired-target")
	if err := f.run(expired); err != nil {
		t.Fatal(err)
	}
	f.session()
	f.exec("UPDATE execution_one_time_results SET expires_at=now()-interval '1 second' WHERE execution_id=$1", expired)
	metadata = f.call("GET", "/enterprise/executions/"+expired.String(), nil, 200, "")
	if !bytes.Contains(metadata, []byte(`"one_time_result_state":"expired"`)) {
		t.Fatal("expired result remained available")
	}
	f.call("POST", "/enterprise/executions/"+expired.String()+"/one-time-result", nil, 410, uuid.NewString())
}
