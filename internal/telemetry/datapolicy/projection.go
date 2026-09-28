// Package datapolicy applies the same credential masking to every telemetry
// reader. It has no role, signal-permission or caller-controlled bypass.
package datapolicy

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

const Version = "argus.telemetry_data_policy/v1"
const Marker = "[REDACTED]"

var assignments = regexp.MustCompile(`(?i)((?:password|passwd|secret|(?:access[_-]?)?token|api[_-]?key|authorization|cookie|set-cookie)["']?\s*[:=]\s*)(?:"[^"]*"|'[^']*'|(?:bearer|basic)\s+[^\s,;]+|[^\s,;]+)`)
var headerLine = regexp.MustCompile(`(?im)((?:authorization|proxy-authorization|cookie|set-cookie)\s*:\s*)[^\r\n]+`)
var privateKey = regexp.MustCompile(`(?s)-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----.*?-----END (?:[A-Z ]+ )?PRIVATE KEY-----`)

func CredentialKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, part := range strings.FieldsFunc(normalized, func(r rune) bool { return r == '.' || r == '/' || r == '[' || r == ']' }) {
		plain := strings.NewReplacer("_", "", "-", "").Replace(part)
		switch plain {
		case "password", "passwd", "secret", "clientsecret", "token", "accesstoken", "refreshtoken", "idtoken", "apikey", "authorization", "proxyauthorization", "cookie", "setcookie", "privatekey", "secretaccesskey":
			return true
		}
	}
	return false
}

// JSON normalization preserves wire data (including large integer values) while
// covering typed slices/maps and attributes encoded as JSON text consistently.
func Project(data any) (any, bool, error) {
	if data == nil {
		return nil, false, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err = decoder.Decode(&value); err != nil {
		return nil, false, err
	}
	output, changed := walk(value, 0)
	if !changed {
		return data, false, nil
	}
	return output, changed, nil
}
func walk(value any, depth int) (any, bool) {
	switch v := value.(type) {
	case map[string]any:
		changed := false
		credentialPair := false
		if key, ok := v["key"].(string); ok {
			credentialPair = CredentialKey(key)
		}
		for key, item := range v {
			if CredentialKey(key) || credentialPair && (key == "value" || key == "values") {
				if item == nil {
					continue
				}
				if item != nil && item != Marker {
					changed = true
				}
				v[key] = Marker
				continue
			}
			next, redacted := walk(item, depth+1)
			v[key] = next
			changed = changed || redacted
		}
		return v, changed
	case []any:
		changed := false
		for i, item := range v {
			next, redacted := walk(item, depth+1)
			v[i] = next
			changed = changed || redacted
		}
		return v, changed
	case string:
		trimmed := strings.TrimSpace(v)
		if depth < 32 && len(trimmed) > 1 && (trimmed[0] == '{' || trimmed[0] == '[') {
			var nested any
			decoder := json.NewDecoder(strings.NewReader(v))
			decoder.UseNumber()
			if decoder.Decode(&nested) == nil && json.Valid([]byte(v)) {
				next, changed := walk(nested, depth+1)
				if changed {
					raw, err := json.Marshal(next)
					if err == nil {
						return string(raw), true
					}
				}
			}
		}
		next := privateKey.ReplaceAllString(v, Marker)
		next = headerLine.ReplaceAllString(next, "${1}"+Marker)
		next = assignments.ReplaceAllStringFunc(next, func(match string) string {
			parts := assignments.FindStringSubmatch(match)
			prefix := parts[1]
			value := match[len(prefix):]
			if strings.HasPrefix(value, "\"") {
				return prefix + "\"" + Marker + "\""
			}
			if strings.HasPrefix(value, "'") {
				return prefix + "'" + Marker + "'"
			}
			return prefix + Marker
		})
		return next, next != v
	default:
		return value, false
	}
}
