package kubernetesinventory

import (
	"bytes"
	"encoding/json"

	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
)

type manifestReference struct {
	path  []any
	value any
}

// ProjectManifest distinguishes typed Kubernetes references from credential
// values. Telemetry's generic policy remains conservative; only the exact
// SecretVolumeSource/SecretProjection structures of built-in Pod specs qualify.
func ProjectManifest(input any) (any, bool, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, false, err
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err = decoder.Decode(&object); err != nil {
		return nil, false, err
	}
	environmentChanged := redactEnvironment(object)
	if environmentChanged {
		masked, _ := json.Marshal(object)
		environmentChanged = !bytes.Equal(raw, masked)
	}
	path := manifestPodPath(object)
	var references []manifestReference
	if len(path) > 0 {
		pod, _ := manifestAt(object, path).(map[string]any)
		volumes, _ := pod["volumes"].([]any)
		for i, item := range volumes {
			volume, _ := item.(map[string]any)
			base := append(append([]any{}, path...), "volumes", i)
			protectManifestReference(volume, "secret", append(base, "secret"), false, &references)
			projected, _ := volume["projected"].(map[string]any)
			sources, _ := projected["sources"].([]any)
			for j, source := range sources {
				mapping, _ := source.(map[string]any)
				protectManifestReference(mapping, "secret", append(append([]any{}, base...), "projected", "sources", j, "secret"), true, &references)
			}
		}
	}
	output, changed, err := datapolicy.Project(object)
	if err != nil {
		return nil, false, err
	}
	for _, reference := range references {
		parent, _ := manifestAt(output, reference.path[:len(reference.path)-1]).(map[string]any)
		parent[reference.path[len(reference.path)-1].(string)] = reference.value
	}
	return output, changed || environmentChanged, nil
}

func manifestPodPath(value map[string]any) []any {
	version, _ := value["apiVersion"].(string)
	kind, _ := value["kind"].(string)
	switch {
	case version == "v1" && kind == "Pod":
		return []any{"spec"}
	case version == "apps/v1" && (kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" || kind == "ReplicaSet"):
		return []any{"spec", "template", "spec"}
	case version == "batch/v1" && kind == "Job":
		return []any{"spec", "template", "spec"}
	case version == "batch/v1" && kind == "CronJob":
		return []any{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	return nil
}

func manifestAt(value any, path []any) any {
	for _, part := range path {
		switch key := part.(type) {
		case string:
			mapping, _ := value.(map[string]any)
			value = mapping[key]
		case int:
			items, _ := value.([]any)
			if key >= len(items) {
				return nil
			}
			value = items[key]
		}
	}
	return value
}

func protectManifestReference(parent map[string]any, key string, path []any, projected bool, references *[]manifestReference) {
	value, ok := parent[key].(map[string]any)
	if !ok || len(value) == 0 {
		return
	}
	nameKey := "secretName"
	if projected {
		nameKey = "name"
	}
	name, ok := value[nameKey].(string)
	if !ok || name == "" {
		return
	}
	for key := range value {
		if key != nameKey && key != "items" && key != "optional" && (projected || key != "defaultMode") {
			return
		}
	}
	if items, exists := value["items"]; exists {
		list, ok := items.([]any)
		if !ok {
			return
		}
		for _, item := range list {
			mapping, ok := item.(map[string]any)
			if !ok {
				return
			}
			for key := range mapping {
				if key != "key" && key != "path" && key != "mode" {
					return
				}
			}
		}
	}
	if _, changed, err := datapolicy.Project(value); err != nil || changed {
		return
	}
	*references = append(*references, manifestReference{path: append([]any{}, path...), value: value})
	delete(parent, key)
}
