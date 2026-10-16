//go:build m4e2e

package main

import (
	"bytes"
	"testing"
	"time"
)

func TestTemplateREDHasCompleteSeriesAndTwoObservedServiceEdges(t *testing.T) {
	start := time.Unix(1792020000, 0).UTC()
	request := templateREDRequest(start, "owned-resource")
	if len(request.ResourceSpans) != 2 {
		t.Fatal("two RED service identities are required")
	}
	edges := map[string]int{}
	for index := 0; index < 600; index++ {
		a, b := request.ResourceSpans[0].ScopeSpans[0].Spans[index], request.ResourceSpans[1].ScopeSpans[0].Spans[index]
		if !bytes.Equal(a.TraceId, b.TraceId) || bytes.Equal(a.SpanId, b.SpanId) || a.StartTimeUnixNano != uint64(start.Add(time.Duration(index)*time.Second).UnixNano()) {
			t.Fatal("trace identity or full-window observation changed")
		}
		parent, child, edge := a, b, "a->b"
		if index%2 == 1 {
			parent, child, edge = b, a, "b->a"
		}
		if len(parent.ParentSpanId) != 0 || !bytes.Equal(child.ParentSpanId, parent.SpanId) {
			t.Fatal("every trace must contain a real acyclic parent-child pair")
		}
		edges[edge]++
	}
	if edges["a->b"] != 300 || edges["b->a"] != 300 || len(request.ResourceSpans[0].ScopeSpans[0].Spans) != 600 || len(request.ResourceSpans[1].ScopeSpans[0].Spans) != 600 {
		t.Fatal("fixture must preserve 1200 RED observations and two topology edges")
	}
}
