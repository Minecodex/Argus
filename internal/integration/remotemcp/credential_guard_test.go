package remotemcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCredentialBearingEnvelopesNeverReachCatalogOrResults(t *testing.T) {
	for _, auth := range []string{"Bearer fixture-token-23456789", "Basic " + base64.StdEncoding.EncodeToString([]byte("fixture-user:fixture-password-23456789"))} {
		for _, wire := range []string{"json", "sse", "sse_eof"} {
			for _, location := range []string{"description", "schema", "text", "structured", "key", "error"} {
				t.Run(strings.Fields(auth)[0]+"/"+wire+"/"+location, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						var request struct {
							ID uint64 `json:"id"`
						}
						if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
							t.Error(err)
							return
						}
						secret := strings.TrimPrefix(auth, "Bearer ")
						if strings.HasPrefix(auth, "Basic ") {
							secret = "fixture-password-23456789"
						}
						result := map[string]any{"content": []any{map[string]any{"type": "text", "text": "safe"}}}
						switch location {
						case "description":
							result = map[string]any{"tools": []any{map[string]any{"name": "test", "description": auth, "inputSchema": map[string]any{"type": "object"}}}}
						case "schema":
							result = map[string]any{"tools": []any{map[string]any{"name": "test", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"value": map[string]any{"type": "string", "default": secret}}}}}}
						case "text":
							result["content"] = []any{map[string]any{"type": "text", "text": "header: " + auth}}
						case "structured":
							result["structuredContent"] = map[string]any{"nested": []any{map[string]any{"value": secret}}}
						case "key":
							result["structuredContent"] = map[string]any{secret: "present"}
						}
						envelope := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}
						if location == "error" {
							delete(envelope, "result")
							envelope["error"] = map[string]any{"code": -32000, "message": secret}
						}
						data, _ := json.Marshal(envelope)
						if wire == "json" {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write(data)
						} else {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprintf(w, "data: %s\n", data)
							if wire == "sse" {
								fmt.Fprint(w, "\n")
							}
						}
					}))
					defer server.Close()
					client := Client{Endpoint: server.URL, HTTP: server.Client(), Authorization: auth}
					var err error
					catalog := location == "description" || location == "schema"
					if catalog {
						tools, _, failure := client.ListTools(context.Background())
						err = failure
						if len(tools) != 0 {
							t.Fatal("credential-bearing catalog escaped")
						}
					} else {
						data, failure := client.Call(t.Context(), "test", nil)
						err = failure
						if data != nil {
							t.Fatal("credential-bearing result escaped")
						}
					}
					var failure Error
					if !errors.As(err, &failure) || failure.Kind != credentialResponseError || failure.Unknown == catalog {
						t.Fatalf("wrong error or dispatch certainty: %v", err)
					}
					if strings.Contains(err.Error(), "23456789") {
						t.Fatal("credential leaked through diagnostic")
					}
				})
			}
		}
	}
}

func TestCredentialGuardPreservesSafeSchemaFields(t *testing.T) {
	client := Client{Authorization: "Basic " + base64.StdEncoding.EncodeToString([]byte("user:strong-fixture-password"))}
	if err := client.checkCredentialResponse([]byte(`{"properties":{"password":{"type":"string"}},"text":"ordinary data"}`)); err != nil {
		t.Fatal(err)
	}
	if err := (&Client{}).checkCredentialResponse([]byte(`{"text":"ordinary data"}`)); err != nil {
		t.Fatal(err)
	}
}
