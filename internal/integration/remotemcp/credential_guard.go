package remotemcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
)

const credentialResponseError = "MCP_RESPONSE_CREDENTIAL_EXPOSED"

// Reject instead of rewriting: replacing a Schema default, enum, key or tool
// name can change its meaning. Nothing from a rejected envelope is published.
func (client *Client) checkCredentialResponse(raw []byte) error {
	if client.Authorization == "" {
		return nil
	}
	values := []string{client.Authorization}
	scheme, value, ok := strings.Cut(client.Authorization, " ")
	if ok && value != "" {
		values = append(values, value)
		if strings.EqualFold(scheme, "basic") {
			decoded, err := base64.StdEncoding.DecodeString(value)
			if err == nil {
				values = append(values, string(decoded))
				if _, password, ok := strings.Cut(string(decoded), ":"); ok && password != "" {
					values = append(values, password)
				}
				clear(decoded)
			}
		}
	}
	var data any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&data) != nil {
		return Error{Kind: "MCP_PROTOCOL_ERROR"}
	}
	if containsCredential(data, values) {
		return Error{Kind: credentialResponseError}
	}
	return nil
}

func containsCredential(data any, values []string) bool {
	switch data := data.(type) {
	case string:
		for _, value := range values {
			if strings.Contains(data, value) {
				return true
			}
		}
	case map[string]any:
		for key, value := range data {
			if containsCredential(key, values) || containsCredential(value, values) {
				return true
			}
		}
	case []any:
		for _, value := range data {
			if containsCredential(value, values) {
				return true
			}
		}
	case json.Number:
		return containsCredential(string(data), values)
	}
	return false
}
