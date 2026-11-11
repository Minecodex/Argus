package kubernetesinventory

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
)

func referenceManifest(version, kind string, projected bool) map[string]any {
	ref := map[string]any{"secretName": "database-auth", "items": []any{map[string]any{"key": "password", "path": "password"}}, "optional": false}
	volume := map[string]any{"name": "protected", "secret": ref}
	if projected {
		volume = map[string]any{"name": "protected", "projected": map[string]any{"sources": []any{map[string]any{"secret": map[string]any{"name": "database-auth", "items": ref["items"]}}}}}
	}
	pod := map[string]any{"containers": []any{map[string]any{"name": "app", "image": "owned:test"}}, "volumes": []any{volume}, "automountServiceAccountToken": false}
	object := map[string]any{"apiVersion": version, "kind": kind, "metadata": map[string]any{"name": "owned"}}
	switch kind {
	case "Pod":
		object["spec"] = pod
	case "CronJob":
		object["spec"] = map[string]any{"jobTemplate": map[string]any{"spec": map[string]any{"template": map[string]any{"spec": pod}}}}
	default:
		object["spec"] = map[string]any{"template": map[string]any{"spec": pod}}
	}
	return object
}

func TestManifestPreservesOnlyTypedSecretReferencesWithoutMutatingInput(t *testing.T) {
	for _, tc := range []struct{ version, kind string }{{"v1", "Pod"}, {"apps/v1", "Deployment"}, {"apps/v1", "StatefulSet"}, {"apps/v1", "DaemonSet"}, {"apps/v1", "ReplicaSet"}, {"batch/v1", "Job"}, {"batch/v1", "CronJob"}} {
		for _, projected := range []bool{false, true} {
			input := referenceManifest(tc.version, tc.kind, projected)
			before, _ := json.Marshal(input)
			output, changed, err := ProjectManifest(input)
			after, _ := json.Marshal(output)
			original, _ := json.Marshal(input)
			if err != nil || changed || string(before) != string(after) || string(before) != string(original) {
				t.Fatalf("%s reference was redacted or mutated: %s %v", tc.kind, after, err)
			}
		}
	}
}

func TestManifestReferenceRecognitionNeverBypassesCredentialMasking(t *testing.T) {
	const canary = "private-canary-value"
	for _, scenario := range []string{"unknown-kind", "unknown-version", "unknown-secret-field", "secret-value-assignment", "environment"} {
		t.Run(scenario, func(t *testing.T) {
			input := referenceManifest("apps/v1", "Deployment", false)
			pod := manifestAt(input, manifestPodPath(input)).(map[string]any)
			volume := pod["volumes"].([]any)[0].(map[string]any)
			switch scenario {
			case "unknown-kind":
				input["kind"] = "CustomDeployment"
				volume["secret"].(map[string]any)["password"] = canary
			case "unknown-version":
				input["apiVersion"] = "customer.example/v1"
				volume["secret"].(map[string]any)["password"] = canary
			case "unknown-secret-field":
				volume["secret"].(map[string]any)["password"] = canary
			case "secret-value-assignment":
				volume["secret"].(map[string]any)["secretName"] = "password=" + canary
			case "environment":
				pod["containers"].([]any)[0].(map[string]any)["env"] = []any{map[string]any{"name": "DATABASE_PASSWORD", "value": canary}}
			}
			output, changed, err := ProjectManifest(input)
			raw, _ := json.Marshal(output)
			if err != nil || !changed || strings.Contains(string(raw), canary) {
				t.Fatal("credential escaped manifest projection", scenario, err)
			}
		})
	}
	// The shared telemetry policy still treats an arbitrary secret object as
	// private; only Kubernetes' typed wrapper recognizes the reference shape.
	if _, changed, err := datapolicy.Project(map[string]any{"secret": map[string]any{"secretName": "database-auth"}}); err != nil || !changed {
		t.Fatal("generic telemetry masking was weakened")
	}
}

func TestManifestAllowsAlreadyMaskedEnvironmentPlaceholder(t *testing.T) {
	input := referenceManifest("apps/v1", "Deployment", false)
	pod := manifestAt(input, manifestPodPath(input)).(map[string]any)
	pod["containers"].([]any)[0].(map[string]any)["env"] = []any{map[string]any{"name": "DATABASE_PASSWORD", "value": datapolicy.Marker}}
	if _, changed, err := ProjectManifest(input); err != nil || changed {
		t.Fatal("masked edit placeholder rejected", err)
	}
}

func TestManifestProjectionPreservesLargeIntegerWireValues(t *testing.T) {
	input := referenceManifest("apps/v1", "Deployment", false)
	input["test_integer"] = json.Number("9007199254740993")
	output, changed, err := ProjectManifest(input)
	raw, _ := json.Marshal(output)
	if err != nil || changed || !strings.Contains(string(raw), "9007199254740993") {
		t.Fatal("manifest integer changed during masking", string(raw), err)
	}
}
