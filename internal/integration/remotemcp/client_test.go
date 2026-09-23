package remotemcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestBusinessDispatchMustBeDurableBeforeHTTPRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{}}})
	}))
	defer server.Close()
	leaseLost := errors.New("dispatch lease lost")
	client := &Client{Endpoint: server.URL, HTTP: server.Client(), BeforeCall: func(context.Context) error { return leaseLost }}
	if _, err := client.Call(t.Context(), "write", map[string]any{}); !errors.Is(err, leaseLost) {
		t.Fatalf("guard error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatal("business request sent before dispatch was committed")
	}
	client.BeforeCall = func(context.Context) error { return nil }
	if _, err := client.Call(t.Context(), "write", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("business request count = %d", requests.Load())
	}
}

func TestStreamableHTTPDiscoveryCallAndPresentationIsolation(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprint(streaming), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("MCP-Protocol-Version") != ProtocolVersion || r.Header.Get("Authorization") != "Bearer fixture-value" {
					t.Error("protocol or authorization header missing")
				}
				var request struct {
					ID     uint64         `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					return
				}
				var result any
				switch request.Method {
				case "initialize":
					w.Header().Set("Mcp-Session-Id", "session-1")
					result = map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{}}}
				case "notifications/initialized":
					w.WriteHeader(http.StatusAccepted)
					return
				case "tools/list":
					if r.Header.Get("Mcp-Session-Id") != "session-1" {
						t.Error("missing session")
					}
					if request.Params["cursor"] == nil {
						result = map[string]any{"tools": []any{map[string]any{"name": "z_write", "inputSchema": map[string]any{"type": "object"}}}, "nextCursor": "next"}
					} else {
						result = map[string]any{"tools": []any{map[string]any{"name": "a_read", "inputSchema": map[string]any{"type": "object"}}}}
					}
				case "tools/call":
					calls.Add(1)
					result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "done"}, map[string]any{"type": "resource", "resource": map[string]any{"text": "<script>unsafe()</script>"}}}, "structuredContent": map[string]any{"count": 1}, "_meta": map[string]any{"presentation": "<script>unsafe()</script>"}}
				default:
					t.Errorf("unexpected method %s", request.Method)
				}
				body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
				if streaming {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: %s\n\n", body)
				} else {
					w.Header().Set("Content-Type", "application/json")
					w.Write(body)
				}
			}))
			defer server.Close()
			client := Client{Endpoint: server.URL, HTTP: server.Client(), Authorization: "Bearer fixture-value"}
			if err := client.Initialize(context.Background()); err != nil {
				t.Fatal(err)
			}
			tools, hash, err := client.ListTools(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(tools) != 2 || tools[0].Name != "a_read" || len(hash) != 64 {
				t.Fatalf("bad catalog: %#v", tools)
			}
			data, err := client.Call(context.Background(), "z_write", map[string]any{})
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(data)
			if string(encoded) != `{"data":{"count":1},"is_error":false,"text":"done"}` || calls.Load() != 1 {
				t.Fatalf("unexpected data/call count: %s / %d", encoded, calls.Load())
			}
		})
	}
}

func TestSSERecoveryNeverRepostsInvocation(t *testing.T) {
	var posts, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if r.Method == http.MethodPost {
			posts.Add(1)
			fmt.Fprint(w, "id: event-1\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n")
			return
		}
		gets.Add(1)
		if r.Header.Get("Last-Event-ID") != "event-1" {
			t.Error("missing resume cursor")
		}
		fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"recovered\"}]}}\n\n")
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, HTTP: server.Client(), SessionID: "session"}
	data, err := client.Call(context.Background(), "write", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if data["text"] != "recovered" || posts.Load() != 1 || gets.Load() != 1 {
		t.Fatalf("unexpected recovery: %#v post=%d get=%d", data, posts.Load(), gets.Load())
	}
}

func TestLostWriteResultIsUnknownAndNotRetried(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, ": disconnected\n\n")
	}))
	defer server.Close()
	client := Client{Endpoint: server.URL, HTTP: server.Client()}
	_, err := client.Call(context.Background(), "write", map[string]any{})
	var unknown Error
	if !errors.As(err, &unknown) || !unknown.Unknown || calls.Load() != 1 {
		t.Fatalf("write was retried or uncertainty lost: %v count=%d", err, calls.Load())
	}
}
