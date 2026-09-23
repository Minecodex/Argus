// Package remotemcp implements the versioned Remote Streamable HTTP transport.
// It never starts a program or renders customer UI content.
package remotemcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const ProtocolVersion = "2025-11-25"
const MaxMessageBytes = 4 << 20

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

type Client struct {
	BeforeCall    func(context.Context) error
	Endpoint      string
	HTTP          *http.Client
	Authorization string
	SessionID     string
	SchemaChanged bool
	lastEventID   string
	next          atomic.Uint64
}

type Error struct {
	Kind    string
	Unknown bool
	Status  int
}

func (e Error) Error() string { return e.Kind }
func (e Error) Code() string  { return e.Kind }

func (client *Client) Initialize(ctx context.Context) error {
	var initialized struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	err := client.request(ctx, "initialize", map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "argus", "version": "1"}}, &initialized, false)
	if err != nil {
		return err
	}
	if initialized.ProtocolVersion != ProtocolVersion {
		return Error{Kind: "MCP_PROTOCOL_VERSION_UNSUPPORTED"}
	}
	return client.notify(ctx, "notifications/initialized", map[string]any{})
}

func (client *Client) ListTools(ctx context.Context) ([]Tool, string, error) {
	all := []Tool{}
	cursor := ""
	seenCursors := map[string]bool{}
	names := map[string]bool{}
	for page := 0; page < 100; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var result struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := client.request(ctx, "tools/list", params, &result, false); err != nil {
			return nil, "", err
		}
		for _, tool := range result.Tools {
			if tool.Name == "" || len(tool.Name) > 256 || names[tool.Name] || tool.InputSchema == nil {
				return nil, "", Error{Kind: "MCP_PROTOCOL_ERROR"}
			}
			names[tool.Name] = true
			all = append(all, tool)
			if len(all) > 10000 {
				return nil, "", Error{Kind: "MCP_CATALOG_TOO_LARGE"}
			}
		}
		if result.NextCursor == "" {
			sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
			encoded, _ := json.Marshal(all)
			if len(encoded) > MaxMessageBytes {
				return nil, "", Error{Kind: "MCP_CATALOG_TOO_LARGE"}
			}
			hash := sha256.Sum256(encoded)
			return all, hex.EncodeToString(hash[:]), nil
		}
		if seenCursors[result.NextCursor] {
			return nil, "", Error{Kind: "MCP_PROTOCOL_ERROR"}
		}
		seenCursors[result.NextCursor] = true
		cursor = result.NextCursor
	}
	return nil, "", Error{Kind: "MCP_CATALOG_TOO_LARGE"}
}

func (client *Client) Call(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StructuredContent map[string]any `json:"structuredContent"`
		IsError           bool           `json:"isError"`
	}
	if err := client.request(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result, true); err != nil {
		return nil, err
	}
	texts := []string{}
	for _, part := range result.Content {
		if part.Type == "text" {
			texts = append(texts, part.Text)
		}
	}
	// Presentation metadata, embedded resources, and executable HTML are never
	// interpreted. Text remains text, including strings containing HTML markup.
	return map[string]any{"text": strings.Join(texts, "\n"), "data": result.StructuredContent, "is_error": result.IsError}, nil
}

func (client *Client) Close(ctx context.Context) {
	if client.SessionID == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, client.Endpoint, nil)
	if err != nil {
		return
	}
	client.headers(req)
	res, err := client.http().Do(req)
	if err == nil {
		res.Body.Close()
	}
}

func (client *Client) request(ctx context.Context, method string, params any, result any, sideEffect bool) error {
	id := client.next.Add(1)
	dispatched, terminal := false, false
	// Cancellation is independent of the transport phase. In particular, a
	// closed SSE/JSON response body does not tell the server to cancel its job.
	defer func() {
		if dispatched && !terminal && method != "initialize" && ctx.Err() != nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = client.notify(cancelCtx, "notifications/cancelled", map[string]any{"requestId": id, "reason": "caller cancelled"})
		}
	}()
	encoded, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.Endpoint, bytes.NewReader(encoded))
	if err != nil {
		return Error{Kind: "MCP_CONNECTION_INVALID"}
	}
	client.headers(req)
	if sideEffect && client.BeforeCall != nil {
		if err := client.BeforeCall(ctx); err != nil {
			return err
		}
	}
	dispatched = true
	response, err := client.http().Do(req)
	if err != nil {
		return Error{Kind: "MCP_UPSTREAM_UNAVAILABLE", Unknown: sideEffect}
	}
	defer response.Body.Close()
	terminal = response.StatusCode < 200 || response.StatusCode >= 300
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return Error{Kind: "MCP_AUTHENTICATION_FAILED", Status: response.StatusCode}
	}
	if response.StatusCode == http.StatusNotFound {
		return Error{Kind: "MCP_SESSION_UNAVAILABLE", Status: response.StatusCode}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Error{Kind: "MCP_UPSTREAM_ERROR", Unknown: sideEffect, Status: response.StatusCode}
	}
	if method == "initialize" {
		client.SessionID = response.Header.Get("Mcp-Session-Id")
		if len(client.SessionID) > 1024 || strings.ContainsAny(client.SessionID, "\r\n") {
			return Error{Kind: "MCP_PROTOCOL_ERROR"}
		}
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaType == "text/event-stream" {
		client.lastEventID = ""
		err := client.readSSE(ctx, response.Body, id, result, sideEffect)
		_ = response.Body.Close()
		for attempt := 0; attempt < 2 && err != nil && client.lastEventID != "" && client.SessionID != "" && ctx.Err() == nil; attempt++ {
			var interrupted Error
			if !errors.As(err, &interrupted) || interrupted.Kind != "MCP_STREAM_INTERRUPTED" {
				break
			}
			resume, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, client.Endpoint, nil)
			if requestErr != nil {
				break
			}
			client.headers(resume)
			resume.Header.Set("Last-Event-ID", client.lastEventID)
			res, requestErr := client.http().Do(resume)
			if requestErr != nil {
				break
			}
			if res.StatusCode != http.StatusOK {
				res.Body.Close()
				break
			}
			err = client.readSSE(ctx, res.Body, id, result, sideEffect)
			res.Body.Close()
		}
		var failure Error
		terminal = err == nil || errors.As(err, &failure) && failure.Kind == "MCP_TOOL_ERROR"
		if errors.As(err, &failure) && (failure.Kind == "MCP_PROTOCOL_ERROR" || failure.Kind == credentialResponseError) {
			failure.Unknown = sideEffect
			return failure
		}
		return err
	}
	if mediaType != "application/json" {
		return Error{Kind: "MCP_PROTOCOL_ERROR", Unknown: sideEffect}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, MaxMessageBytes+1))
	if err != nil || len(body) > MaxMessageBytes {
		return Error{Kind: "MCP_MESSAGE_TOO_LARGE", Unknown: sideEffect}
	}
	matched, err := client.decode(body, id, result)
	var remoteError Error
	terminal = matched || errors.As(err, &remoteError) && remoteError.Kind == "MCP_TOOL_ERROR"
	if err != nil {
		var e Error
		if errors.As(err, &e) {
			e.Unknown = sideEffect && (e.Kind == "MCP_PROTOCOL_ERROR" || e.Kind == credentialResponseError)
			return e
		}
		return err
	}
	if !matched {
		return Error{Kind: "MCP_PROTOCOL_ERROR", Unknown: sideEffect}
	}
	return nil
}

func (client *Client) readSSE(ctx context.Context, reader io.Reader, id uint64, result any, sideEffect bool) error {
	scanner := bufio.NewScanner(io.LimitReader(reader, MaxMessageBytes+1))
	scanner.Buffer(make([]byte, 4096), MaxMessageBytes)
	var data strings.Builder
	eventID := ""
	flush := func() (bool, error) {
		if data.Len() == 0 {
			return false, nil
		}
		value := data.String()
		data.Reset()
		matched, err := client.decode([]byte(value), id, result)
		if err == nil && eventID != "" {
			client.lastEventID = eventID
		}
		eventID = ""
		return matched, err
	}
	for scanner.Scan() {
		if ctx.Err() != nil {
			return Error{Kind: "MCP_UPSTREAM_TIMEOUT", Unknown: sideEffect}
		}
		line := scanner.Text()
		if line == "" {
			matched, err := flush()
			if err != nil {
				return err
			}
			if matched {
				return nil
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			data.WriteByte('\n')
		}
		if strings.HasPrefix(line, "id:") {
			eventID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			if len(eventID) > 1024 || strings.ContainsAny(eventID, "\x00\r\n") {
				return Error{Kind: "MCP_PROTOCOL_ERROR", Unknown: sideEffect}
			}
		}
	}
	matched, err := flush()
	if err != nil {
		return err
	}
	if err == nil && matched {
		return nil
	}
	return Error{Kind: "MCP_STREAM_INTERRUPTED", Unknown: sideEffect}
}

func (client *Client) decode(raw []byte, id uint64, result any) (bool, error) {
	if err := client.checkCredentialResponse(raw); err != nil {
		return false, err
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      *uint64         `json:"id"`
		Method  string          `json:"method"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.JSONRPC != "2.0" {
		return false, Error{Kind: "MCP_PROTOCOL_ERROR"}
	}
	if envelope.ID == nil {
		if envelope.Method == "notifications/tools/list_changed" {
			client.SchemaChanged = true
		}
		return false, nil
	}
	if *envelope.ID != id || envelope.Method != "" {
		return false, Error{Kind: "MCP_PROTOCOL_ERROR"}
	}
	if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return false, Error{Kind: "MCP_TOOL_ERROR"}
	}
	if len(envelope.Result) == 0 || json.Unmarshal(envelope.Result, result) != nil {
		return false, Error{Kind: "MCP_PROTOCOL_ERROR"}
	}
	return true, nil
}

func (client *Client) notify(ctx context.Context, method string, params any) error {
	encoded, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.Endpoint, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	client.headers(req)
	res, err := client.http().Do(req)
	if err != nil {
		return Error{Kind: "MCP_UPSTREAM_UNAVAILABLE"}
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Error{Kind: "MCP_PROTOCOL_ERROR"}
	}
	return nil
}

func (client *Client) headers(request *http.Request) {
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	if client.SessionID != "" {
		request.Header.Set("Mcp-Session-Id", client.SessionID)
	}
	if client.Authorization != "" {
		request.Header.Set("Authorization", client.Authorization)
	}
}

func (client *Client) http() *http.Client {
	if client.HTTP != nil {
		return client.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}
