package skywalking

import "testing"

func TestSpanHierarchyKeepsMissingParentsAndCycles(t *testing.T) {
	edges, cycle := assembleEdges([]spanRecord{{spanID: "root", serviceName: "api"}, {spanID: "child", parentSpanID: "root"}, {spanID: "grandchild", parentSpanID: "child"}, {spanID: "orphan", parentSpanID: "absent"}})
	if cycle || len(edges) != 3 {
		t.Fatalf("invalid hierarchy: %+v", edges)
	}
	for _, edge := range edges {
		if edge.childSpanID == "grandchild" && edge.depth != 2 {
			t.Fatal("depth not assembled")
		}
		if edge.childSpanID == "orphan" && (!edge.missingParent || edge.parentService != "") {
			t.Fatal("missing parent fabricated")
		}
	}
	_, cycle = assembleEdges([]spanRecord{{spanID: "a", parentSpanID: "b"}, {spanID: "b", parentSpanID: "a"}})
	if !cycle {
		t.Fatal("cyclic spans not detected")
	}
}
