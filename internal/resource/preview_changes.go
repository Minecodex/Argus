package resource

import (
	"encoding/json"
	"reflect"
	"sort"
)

// PublicChange describes presentation facts only; it is never an executable
// plan. Callers pass the public fields that the frozen update will apply.
type PublicChange struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

func PublicChanges(before, after map[string]any) []PublicChange {
	keys := make([]string, 0, len(after))
	for key := range after {
		switch key {
		case "name", "hostname", "environment", "address", "port", "api_server", "default_namespace", "connection_mode", "bastion_scope_id", "authentication_changed", "labels":
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	changes := []PublicChange{}
	for _, key := range keys {
		if !reflect.DeepEqual(before[key], after[key]) {
			left, right := before[key], after[key]
			if key == "labels" {
				// Label keys may contain punctuation and system namespaces. Keep
				// them as data, rather than properties of the public JSON contract.
				oldJSON, _ := json.Marshal(left)
				newJSON, _ := json.Marshal(right)
				left, right = string(oldJSON), string(newJSON)
			}
			changes = append(changes, PublicChange{Path: key, Before: left, After: right})
		}
	}
	return changes
}

func hostUpdateChanges(currentName, hostname, environment, address string, port int32, storedLabels []byte, input HostInput) []PublicChange {
	beforeLabels, _ := DecodeLabels(storedLabels)
	before := map[string]any{"name": currentName, "hostname": hostname, "environment": environment, "address": address, "port": port, "labels": beforeLabels}
	after := map[string]any{}
	for key, value := range map[string]string{"name": input.Name, "hostname": input.Hostname, "environment": input.Environment, "address": input.Address} {
		if value != "" {
			after[key] = value
		}
	}
	if input.Port > 0 {
		after["port"] = input.Port
	}
	if input.Labels != nil {
		after["labels"] = MergeSystemLabels(input.Labels, beforeLabels)
	}
	return PublicChanges(before, after)
}
