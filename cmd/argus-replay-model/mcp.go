//go:build m4e2e

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

var mcpWrites atomic.Int64

func remoteMCP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("auth") == "bearer" && r.Header.Get("Authorization") != "Bearer p5-token" {
		w.WriteHeader(401)
		return
	}
	if r.URL.Query().Get("auth") == "basic" {
		user, password, ok := r.BasicAuth()
		if !ok || user != "p5" || password != "p5-pass" {
			w.WriteHeader(401)
			return
		}
	}
	if r.Method == http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  map[string]any  `json:"params"`
	}
	if json.NewDecoder(r.Body).Decode(&request) != nil || request.JSONRPC != "2.0" {
		http.Error(w, "invalid RPC", 400)
		return
	}
	if len(request.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	result := map[string]any{}
	switch request.Method {
	case "initialize":
		if request.Params["protocolVersion"] != "2025-11-25" {
			http.Error(w, "unsupported protocol", 400)
			return
		}
		result = map[string]any{"protocolVersion": "2025-11-25", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "Argus P5 Remote MCP fixture", "version": "1"}}
		w.Header().Set("Mcp-Session-Id", "p5-session")
	case "tools/list":
		name := "read_counter"
		if request.Params["cursor"] == "write" {
			name = "write_counter"
		} else {
			result["nextCursor"] = "write"
		}
		description := "P5 fixture " + name
		if r.URL.Query().Get("large") == "1" {
			description += strings.Repeat(" schema context", 10000)
		}
		if r.URL.Query().Get("reflect") == "catalog" {
			description += " " + r.Header.Get("Authorization")
		}
		result["tools"] = []any{map[string]any{"name": name, "description": description, "inputSchema": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"drop_response": map[string]any{"type": "boolean"}}}}}
	case "tools/call":
		if request.Params["name"] == "write_counter" {
			mcpWrites.Add(1)
		}
		args, _ := request.Params["arguments"].(map[string]any)
		if args["drop_response"] == true {
			if hijacker, ok := w.(http.Hijacker); ok {
				connection, _, err := hijacker.Hijack()
				if err == nil {
					connection.Close()
				}
			}
			return
		}
		result = map[string]any{"content": []any{map[string]any{"type": "text", "text": fmt.Sprint(mcpWrites.Load())}}, "structuredContent": map[string]any{"writes": mcpWrites.Load()}, "_meta": map[string]any{"ui": map[string]any{"html": "<script>throw new Error('must not render MCP metadata')</script>"}}}
		if r.URL.Query().Get("reflect") == "result" {
			result["content"] = []any{map[string]any{"type": "text", "text": r.Header.Get("Authorization")}}
			result["structuredContent"] = map[string]any{"diagnostic": r.Header.Get("Authorization")}
		}

	default:
		writeJSON(w, 200, map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32601, "message": "unknown method"}})
		return
	}
	message := map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result}
	if r.URL.Query().Get("mode") == "sse" {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(w, message)
		return
	}
	writeJSON(w, 200, message)
}
