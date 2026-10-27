package arguslogidentity

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestMetadataLagRetriesWholeBatchAndOwnerUIDFilters(t *testing.T) {
	config := Config{Namespace: "orders", WorkloadType: "deployment", WorkloadUID: uuid.NewString(), NodeName: "node-a", Containers: []string{"main"}}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	logs := plog.NewLogs()
	appendResource := func(owner string, metadata bool) {
		r := logs.ResourceLogs().AppendEmpty()
		a := r.Resource().Attributes()
		a.PutStr("k8s.namespace.name", "orders")
		a.PutStr("k8s.container.name", "main")
		a.PutStr("k8s.pod.uid", uuid.NewString())
		a.PutStr("argus.application.id", "spoof")
		if metadata {
			a.PutStr("k8s.pod.start_time", "2026-10-19T00:00:00Z")
			a.PutStr("k8s.node.name", "node-a")
			a.PutStr("k8s.deployment.uid", owner)
		}
		r.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr("log line")
	}
	appendResource(config.WorkloadUID, true)
	appendResource(config.WorkloadUID, false)
	marshal := plog.JSONMarshaler{}
	before, _ := marshal.MarshalLogs(logs)
	if _, err := config.process(logs); err == nil || consumererror.IsPermanent(err) {
		t.Fatal("missing metadata acknowledged or made permanent")
	}
	after, _ := marshal.MarshalLogs(logs)
	if !bytes.Equal(before, after) {
		t.Fatal("retry mutated a partial batch")
	}
	a := logs.ResourceLogs().At(1).Resource().Attributes()
	a.PutStr("k8s.pod.start_time", "2026-10-19T00:00:00Z")
	a.PutStr("k8s.node.name", "node-a")
	a.PutStr("k8s.deployment.uid", config.WorkloadUID)
	appendResource(uuid.NewString(), true)
	appendResource("", true)
	result, err := config.process(logs)
	if err != nil || result.ResourceLogs().Len() != 2 {
		t.Fatalf("owner binding: %v", err)
	}
	if _, ok := result.ResourceLogs().At(0).Resource().Attributes().Get("argus.application.id"); ok {
		t.Fatal("untrusted assignment claim survived")
	}
}
