package configbundle

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestFixedCollectionsReplaceLegacyProfilesAndKeepIndependentState(t *testing.T) {
	host, collector := uuid.NewString(), uuid.NewString()
	input := RenderInput{CollectorID: collector, ResourceID: host, ResourceType: "host", Role: "direct", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test", ProfileKeys: []string{"host-basic", "collector-self"}, Collections: []Collection{{ID: uuid.NewString(), Integration: "host_metrics", TargetType: "host", TargetID: host, Config: json.RawMessage(`{"interval":"10s"}`), Revision: 1, AssignmentEpoch: 1, Enabled: false}, {ID: uuid.NewString(), Integration: "collector_metrics", TargetType: "host", TargetID: host, Config: json.RawMessage(`{}`), Revision: 1, AssignmentEpoch: 1, Enabled: true}}}
	raw, err := Render(input)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := Sources(raw)
	if err != nil || len(sources) != 1 || sources[0].Type != "collector_self" || sources[0].CollectionID != input.Collections[1].ID {
		t.Fatalf("independent builtin sources: %+v %v", sources, err)
	}
	config := decodeTarget(t, raw, "host")
	receivers := nestedMap(t, config, "receivers")
	if receivers["hostmetrics"] != nil || receivers["prometheus/collector_self"] != nil {
		t.Fatal("legacy profiles continued alongside fixed instances")
	}
	input.Collections[1].Enabled = false
	raw, err = Render(input)
	if err != nil {
		t.Fatal(err)
	}
	sources, _ = Sources(raw)
	if len(sources) != 0 {
		t.Fatal("disabled own metrics introduced an implicit source")
	}
	input.Collections[0].Enabled = true
	input.Collections[0].TargetID = uuid.NewString()
	if _, err = Render(input); err == nil {
		t.Fatal("fixed metrics collected another resource")
	}
}

func TestClusterFixedCollectionsRunAtTheirRequiredPlacement(t *testing.T) {
	cluster := uuid.NewString()
	input := RenderInput{CollectorID: uuid.NewString(), ResourceID: cluster, ResourceType: "kubernetes_cluster", Role: "direct", Platform: "linux_amd64", SourceGeneration: uuid.NewString(), ConfigRevision: 1, RouteKind: "direct_argus", Transport: "direct", EnrollmentEndpoint: "https://control.test/enroll", IngestGRPCEndpoint: "https://ingest.test:4317", IngestHTTPEndpoint: "https://ingest.test"}
	for _, key := range []string{"collector_metrics", "kubernetes_cluster_metrics", "kubernetes_node_metrics"} {
		input.Collections = append(input.Collections, Collection{ID: uuid.NewString(), Integration: key, TargetType: "kubernetes_cluster", TargetID: cluster, Config: json.RawMessage(`{}`), Revision: 1, AssignmentEpoch: 1, Enabled: true})
	}
	raw, err := Render(input)
	if err != nil {
		t.Fatal(err)
	}
	sources, err := Sources(raw)
	if err != nil || len(sources) != 4 {
		t.Fatalf("cluster instance placements: %d %v", len(sources), err)
	}
	for _, source := range sources {
		if source.Type == "kubeletstats" && source.Key[:16] != "kubernetes_agent" {
			t.Fatal("node collection ran on the gateway")
		}
		if source.Type == "k8s_cluster" && source.Key[:18] != "kubernetes_gateway" {
			t.Fatal("cluster collection ran on each node")
		}
	}
}
