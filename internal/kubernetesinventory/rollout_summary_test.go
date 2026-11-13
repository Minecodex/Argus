package kubernetesinventory

import (
	"encoding/json"
	"testing"
)

func TestWorkloadRolloutDoesNotConfuseAvailabilityWithUpdatedRevision(t *testing.T) {
	cases := []struct{ name, kind, data, state string }{
		{"old pods ready", "deployment", `{"spec":{"replicas":3},"status":{"replicas":4,"readyReplicas":3,"availableReplicas":3,"updatedReplicas":1,"observedGeneration":2}}`, "progressing"},
		{"new version unavailable", "deployment", `{"spec":{"replicas":3},"status":{"replicas":3,"readyReplicas":3,"availableReplicas":2,"updatedReplicas":3,"observedGeneration":2}}`, "progressing"},
		{"timeout", "deployment", `{"status":{"conditions":[{"type":"Progressing","status":"False","reason":"ProgressDeadlineExceeded"}]}}`, "failed"},
		{"unobserved generation", "deployment", `{"spec":{"replicas":0},"status":{"observedGeneration":1}}`, "progressing"},
		{"stateful old revision", "statefulset", `{"spec":{"replicas":3},"status":{"replicas":3,"readyReplicas":3,"updatedReplicas":1,"currentRevision":"a","updateRevision":"b","observedGeneration":2}}`, "progressing"},
		{"stateful complete", "statefulset", `{"spec":{"replicas":3},"status":{"replicas":3,"readyReplicas":3,"updatedReplicas":3,"currentRevision":"b","updateRevision":"b","observedGeneration":2}}`, "healthy"},
		{"partition rollout complete", "statefulset", `{"spec":{"replicas":3,"updateStrategy":{"type":"RollingUpdate","rollingUpdate":{"partition":2}}},"status":{"replicas":3,"readyReplicas":3,"updatedReplicas":1,"currentRevision":"a","updateRevision":"b","observedGeneration":2}}`, "healthy"},
		{"on delete needs recreation", "statefulset", `{"spec":{"replicas":3,"updateStrategy":{"type":"OnDelete"}},"status":{"replicas":3,"readyReplicas":3,"updatedReplicas":1,"currentRevision":"a","updateRevision":"b","observedGeneration":2}}`, "warning"},
		{"daemon old pods ready", "daemonset", `{"status":{"desiredNumberScheduled":3,"numberReady":3,"numberAvailable":3,"updatedNumberScheduled":1,"observedGeneration":2}}`, "progressing"},
		{"daemon complete", "daemonset", `{"status":{"desiredNumberScheduled":3,"numberReady":3,"numberAvailable":3,"updatedNumberScheduled":3,"observedGeneration":2}}`, "healthy"},
		{"missing observation", "statefulset", `{"spec":{"replicas":3}}`, "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def, _ := Lookup(tc.kind)
			got := Summarize(Object{Definition: def, Generation: 2, Data: json.RawMessage(tc.data)})
			if got.State != tc.state {
				t.Fatalf("got %+v want %s", got, tc.state)
			}
		})
	}
}
