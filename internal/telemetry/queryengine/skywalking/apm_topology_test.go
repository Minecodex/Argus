package skywalking

import (
	"context"
	"testing"
	"time"
)

func TestTopologyUsesExplicitParentIdentityAndReportsAmbiguity(t *testing.T) {
	parent := topologySpan{source: "source-a", resource: "host-a", sourceType: "otlp", trace: "trace", id: "parent", service: "same-name", kind: 3}
	child := topologySpan{source: "source-b", resource: "host-b", sourceType: "otlp", trace: "trace", id: "child", parent: "parent", service: "same-name", kind: 2, duration: 100000000, status: "error"}
	from := time.Unix(0, 0)
	graph, err := assembleTopology(context.Background(), []topologySpan{parent, child}, apmArgs{}, from, from.Add(10*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.nodes) != 2 || len(graph.edges) != 1 || graph.edges[0].stats.samples != 1 || graph.edges[0].stats.errors != 1 || *graph.edges[0].SamplesPerSecond() != 0.1 {
		t.Fatalf("source identities or entry samples lost: %+v", graph)
	}
	duplicate := parent
	duplicate.source, duplicate.resource = "source-c", "host-c"
	graph, err = assembleTopology(context.Background(), []topologySpan{parent, duplicate, child}, apmArgs{}, from, from.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.edges) != 0 || graph.coverage.ambiguous != 1 || graph.status != "incomplete_observations" {
		t.Fatal("ambiguous source histories were joined")
	}
	parent.sourceType = "jaeger"
	graph, err = assembleTopology(context.Background(), []topologySpan{parent, child}, apmArgs{}, from, from.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.edges) != 0 || graph.coverage.missing != 1 {
		t.Fatal("different receiver types were merged")
	}
	if !topologyCycles(map[string]string{"a": "b", "b": "a", "descendant": "a"})["descendant"] {
		t.Fatal("malformed span cycle not marked")
	}
}
