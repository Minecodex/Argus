package kubernetesinventory

import (
	"encoding/json"
	"testing"
)

func TestSummaryPreservesObservedStateSemantics(t *testing.T) {
	tests := []struct{ kind, data, state string }{
		{"pod", `{"spec":{"containers":[{}]},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"False"}]}}`, "warning"},
		{"pod", `{"status":{"phase":"Running","containerStatuses":[{"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]}}`, "failed"},
		{"pod", `{"status":{"phase":"Succeeded"}}`, "completed"},
		{"node", `{"status":{"conditions":[{"type":"Ready","status":"Unknown"}]}}`, "unknown"},
		{"node", `{"status":{"conditions":[{"type":"Ready","status":"True"},{"type":"DiskPressure","status":"True"}]}}`, "warning"},
		{"deployment", `{"spec":{"replicas":0},"status":{"observedGeneration":1}}`, "scaled_down"},
		{"deployment", `{"spec":{"replicas":3},"status":{"readyReplicas":3,"observedGeneration":0}}`, "progressing"},
		{"deployment", `{"spec":{"replicas":3},"status":{"replicas":3,"readyReplicas":3,"availableReplicas":3,"updatedReplicas":3,"observedGeneration":1}}`, "healthy"},
		{"job", `{"spec":{"suspend":true},"status":{}}`, "paused"},
		{"job", `{"status":{"conditions":[{"type":"Failed","status":"True"}]}}`, "failed"},
		{"pvc", `{"status":{"phase":"Bound","conditions":[{"type":"FileSystemResizePending","status":"True"}]}}`, "progressing"},
		{"pvc", `{"status":{"phase":"Lost"}}`, "failed"},
		{"cronjob", `{"spec":{"suspend":true,"schedule":"*/2 * * * *"}}`, "paused"},
		{"pod", `{"metadata":{"deletionTimestamp":"2026-01-01T00:00:00Z"},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}]}}`, "terminating"},
	}
	for _, tc := range tests {
		t.Run(tc.kind+"/"+tc.state, func(t *testing.T) {
			d, _ := Lookup(tc.kind)
			got := Summarize(Object{Definition: d, Generation: 1, Data: json.RawMessage(tc.data)})
			if got.State != tc.state {
				t.Fatalf("got %+v want %s", got, tc.state)
			}
		})
	}
}
func TestSummaryDoesNotRevealSecretValuesOrInventObservation(t *testing.T) {
	d, _ := Lookup("secret")
	got := Summarize(Object{Definition: d, Data: json.RawMessage(`{"type":"Opaque","data":{"password":"canary-sensitive"}}`)})
	raw, _ := json.Marshal(got)
	if got.Fields["keys"] != "1" || string(raw) == "" {
		t.Fatal(got)
	}
	for _, v := range got.Fields {
		if v == "canary-sensitive" {
			t.Fatal("secret value leaked")
		}
	}
	d, _ = Lookup("pod")
	got = Summarize(Object{Definition: d, Data: json.RawMessage(`{"spec":{"containers":[{}]}}`)})
	if got.State != "unknown" || got.Fields["restarts"] != "" || got.Fields["ready"] != "" {
		t.Fatal("missing observation became healthy or zero", got)
	}
}

func TestDaemonSetDesiredCountAndPodDominantReason(t *testing.T) {
	d, _ := Lookup("daemonset")
	got := Summarize(Object{Definition: d, Generation: 2, Data: json.RawMessage(`{"status":{"desiredNumberScheduled":8,"numberReady":8,"numberAvailable":8,"updatedNumberScheduled":8,"observedGeneration":2}}`)})
	if got.Fields["desired_replicas"] != "8" || got.Fields["ready"] != "8 / 8" {
		t.Fatal(got)
	}
	d, _ = Lookup("pod")
	for _, statuses := range []string{`[{"state":{"waiting":{"reason":"CrashLoopBackOff"}}},{"state":{"waiting":{"reason":"ContainerCreating"}}}]`, `[{"state":{"waiting":{"reason":"ContainerCreating"}}},{"state":{"waiting":{"reason":"CrashLoopBackOff"}}}]`} {
		got = Summarize(Object{Definition: d, Data: json.RawMessage(`{"spec":{"containers":[{},{}]},"status":{"phase":"Running","containerStatuses":` + statuses + `}}`)})
		if got.State != "failed" || got.Reason != "CrashLoopBackOff" {
			t.Fatal(got)
		}
	}
}

func TestInitFailureAndRestartableSidecarSummary(t *testing.T) {
	definition, _ := Lookup("pod")
	failed := Summarize(Object{Definition: definition, Data: json.RawMessage(`{"spec":{"containers":[{"name":"app"}],"initContainers":[{"name":"init"}]},"status":{"phase":"Pending","initContainerStatuses":[{"name":"init","restartCount":3,"state":{"waiting":{"reason":"CrashLoopBackOff"}}}],"containerStatuses":[{"name":"app","state":{"waiting":{"reason":"PodInitializing"}}}]}}`)})
	if failed.State != "failed" || failed.Reason != "CrashLoopBackOff" || failed.Fields["restarts"] != "3" {
		t.Fatal(failed)
	}
	ready := Summarize(Object{Definition: definition, Data: json.RawMessage(`{"spec":{"containers":[{"name":"app"}],"initContainers":[{"name":"sidecar","restartPolicy":"Always"}]},"status":{"phase":"Running","conditions":[{"type":"Ready","status":"True"}],"initContainerStatuses":[{"name":"sidecar","ready":true}],"containerStatuses":[{"name":"app","ready":true}]}}`)})
	if ready.Fields["ready"] != "2 / 2" || ready.State != "healthy" {
		t.Fatal(ready)
	}
}

func TestTerminatedContainerFailureRemainsVisibleBetweenRestarts(t *testing.T) {
	definition, _ := Lookup("pod")
	summary := Summarize(Object{Definition: definition, Data: json.RawMessage(`{"spec":{"containers":[{},{}]},"status":{"phase":"Running","containerStatuses":[{"state":{"terminated":{"exitCode":1,"reason":"Error"}}},{"ready":true,"state":{"running":{}}}]}}`)})
	if summary.State != "failed" || summary.Reason != "Error" {
		t.Fatal(summary)
	}
}
