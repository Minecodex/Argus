package dashboard

import (
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
)

type Change struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

func publicationDiff(before, after any) []Change {
	normalize := func(value any) any {
		encoded, _ := json.Marshal(value)
		var result any
		_ = json.Unmarshal(encoded, &result)
		return result
	}
	changes := []Change{}
	var walk func(string, any, any)
	walk = func(path string, left, right any) {
		if reflect.DeepEqual(left, right) || len(changes) >= 256 {
			return
		}
		if len(changes) == 255 {
			changes = append(changes, Change{Kind: "note", Text: "Additional changes are included in the complete frozen before/after configuration."})
			return
		}
		lm, lok := left.(map[string]any)
		rm, rok := right.(map[string]any)
		if lok && rok {
			keys := map[string]bool{}
			for key := range lm {
				keys[key] = true
			}
			for key := range rm {
				keys[key] = true
			}
			names := make([]string, 0, len(keys))
			for key := range keys {
				names = append(names, key)
			}
			sort.Strings(names)
			for _, name := range names {
				walk(path+"."+name, lm[name], rm[name])
			}
			return
		}
		la, lok := left.([]any)
		ra, rok := right.([]any)
		if lok && rok {
			if indexedLeft, ok := indexItems(la); ok {
				if indexedRight, ok := indexItems(ra); ok {
					if len(la) == len(ra) {
						leftOrder, rightOrder := []string{}, []string{}
						for i := range la {
							leftOrder = append(leftOrder, la[i].(map[string]any)["id"].(string))
							rightOrder = append(rightOrder, ra[i].(map[string]any)["id"].(string))
						}
						if !reflect.DeepEqual(leftOrder, rightOrder) {
							l, _ := json.Marshal(leftOrder)
							r, _ := json.Marshal(rightOrder)
							changes = append(changes, Change{Kind: "change", Text: clip(path, 300) + ".order: " + clip(string(l), 650) + " → " + clip(string(r), 650)})
						}
					}
					walk(path, indexedLeft, indexedRight)
					return
				}
			}
			if len(la) == len(ra) {
				for i := range la {
					walk(path+"["+strconv.Itoa(i)+"]", la[i], ra[i])
				}
				return
			}
		}
		kind := "change"
		if left == nil {
			kind = "add"
		} else if right == nil {
			kind = "remove"
		}
		l, _ := json.Marshal(left)
		r, _ := json.Marshal(right)
		changes = append(changes, Change{Kind: kind, Text: clip(path, 300) + ": " + clip(string(l), 650) + " → " + clip(string(r), 650)})
	}
	walk("dashboard", normalize(before), normalize(after))
	return changes
}

func indexItems(items []any) (map[string]any, bool) {
	result := map[string]any{}
	for _, item := range items {
		value, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		id, ok := value["id"].(string)
		if !ok || id == "" {
			return nil, false
		}
		if _, exists := result[id]; exists {
			return nil, false
		}
		result[id] = item
	}
	return result, true
}

func clip(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}
