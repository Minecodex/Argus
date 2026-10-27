package kubernetesworkbench

import (
	"encoding/json"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

type DiffLine struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// PublicMutationDiff uses the existing PendingAction public contract. It never
// truncates an approval diff: oversized changes fail before an action is saved.
func PublicMutationDiff(result kubernetesinventory.MutationResult) ([]DiffLine, error) {
	lines := []DiffLine{}
	appendValue := func(kind, path string, value any) error {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		lines = append(lines, DiffLine{Kind: "note", Text: path})
		for _, line := range strings.Split(string(raw), "\n") {
			if utf8.RuneCountInString(line) > 2048 {
				return kubernetesinventory.ErrBudget
			}
			lines = append(lines, DiffLine{Kind: kind, Text: line})
			if len(lines) > 256 {
				return kubernetesinventory.ErrBudget
			}
		}
		return nil
	}
	var walk func(string, any, any) error
	walk = func(path string, before, after any) error {
		if reflect.DeepEqual(before, after) {
			return nil
		}
		left, lok := before.(map[string]any)
		right, rok := after.(map[string]any)
		if lok && rok {
			keys := map[string]bool{}
			for key := range left {
				keys[key] = true
			}
			for key := range right {
				keys[key] = true
			}
			ordered := make([]string, 0, len(keys))
			for key := range keys {
				ordered = append(ordered, key)
			}
			sort.Strings(ordered)
			for _, key := range ordered {
				escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
				if err := walk(path+"/"+escaped, left[key], right[key]); err != nil {
					return err
				}
			}
			return nil
		}
		if before != nil {
			if err := appendValue("remove", path, before); err != nil {
				return err
			}
		}
		if after != nil {
			if err := appendValue("add", path, after); err != nil {
				return err
			}
		}
		return nil
	}
	err := walk("", previewManifest(result.Before), previewManifest(result.After))
	if err == nil && result.Drain != nil {
		lines = append(lines, DiffLine{Kind: "note", Text: "drain: PDB-respecting eviction; local data and unmanaged pods block execution; node stays cordoned"})
		for _, pod := range result.Drain.Pods {
			raw, e := json.Marshal(pod)
			if e != nil || utf8.RuneCount(raw) > 2048 || len(lines) >= 256 {
				return nil, kubernetesinventory.ErrBudget
			}
			lines = append(lines, DiffLine{Kind: "note", Text: string(raw)})
		}
	}
	return lines, err
}
