// Package presentation owns immutable UI projections. Template code never
// enters model results, prompts or compaction inputs.
package presentation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/kakj-go/Argus/internal/toolruntime"
)

func Asset(source, version string) *toolruntime.TemplateAsset {
	hash := sha256.Sum256([]byte(source))
	return &toolruntime.TemplateAsset{Source: source, Hash: hex.EncodeToString(hash[:]), Version: version, Runtime: "argus-template/v1"}
}
func Validate(asset *toolruntime.TemplateAsset) error {
	if asset == nil {
		return nil
	}
	hash := sha256.Sum256([]byte(asset.Source))
	if asset.Runtime != "argus-template/v1" || asset.Version == "" || len(asset.Source) == 0 || len(asset.Source) > 256<<10 || !utf8.ValidString(asset.Source) || asset.Hash != hex.EncodeToString(hash[:]) {
		return errors.New("invalid immutable Tool template")
	}
	lower := strings.ToLower(asset.Source)
	for _, disallowed := range []string{"<iframe", "<object", "<embed", "<form", "<base", "<meta", "<link", "<script src", "eval(", "new function", "import(", "javascript:", "https://", "http://"} {
		if strings.Contains(lower, disallowed) {
			return errors.New("unsupported Tool template capability")
		}
	}
	return nil
}

// Details copies only the public result and removes control references. Private
// action plans are never passed here, and templates cannot acquire authority.
func Details(value map[string]any) map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var copy map[string]any
	_ = json.Unmarshal(encoded, &copy)
	var clean func(any) any
	clean = func(value any) any {
		switch value := value.(type) {
		case map[string]any:
			for key, item := range value {
				normalized := strings.ToLower(key)
				if normalized == "action_ref" || normalized == "commit_token" || normalized == "argus__token" || normalized == "private" || normalized == "frozen_plan" || normalized == "approval_internal" || normalized == "one_time_result" {
					delete(value, key)
				} else {
					value[key] = clean(item)
				}
			}
			return value
		case []any:
			for i, item := range value {
				value[i] = clean(item)
			}
			return value
		default:
			return value
		}
	}
	data := clean(copy).(map[string]any)
	trimmed := false
	var bound func(any, int) any
	bound = func(value any, depth int) any {
		if depth > 8 {
			trimmed = true
			return "[detail omitted]"
		}
		switch value := value.(type) {
		case string:
			if len(value) > 8192 {
				trimmed = true
				return strings.ToValidUTF8(value[:8192], "")
			}
			return value
		case []any:
			if len(value) > 200 {
				trimmed = true
				value = value[:200]
			}
			for i, item := range value {
				value[i] = bound(item, depth+1)
			}
			return value
		case map[string]any:
			for key, item := range value {
				value[key] = bound(item, depth+1)
			}
			return value
		default:
			return value
		}
	}
	data = bound(data, 0).(map[string]any)
	encoded, _ = json.Marshal(data)
	if len(encoded) > 512<<10 {
		return map[string]any{"_presentation_truncated": true, "sample": strings.ToValidUTF8(string(encoded[:64<<10]), "")}
	}
	if trimmed {
		data["_presentation_truncated"] = true
	}
	return data
}
