package modelprovider

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RequestBytes measures the complete protocol payload before any dispatch.
func (provider Provider) RequestBytes(request Request) (int, error) {
	_, data, err := provider.buildRequest(request)
	return len(data), err
}

// RequestHash hashes the exact provider JSON, including wire aliases and
// protocol-specific tool messages, without including endpoint credentials.
func (provider Provider) RequestHash(request Request) (string, error) {
	_, data, err := provider.buildRequest(request)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

type Protocol string

const (
	ProtocolChatCompletions Protocol = "chat_completions"
	ProtocolResponses       Protocol = "responses"
)

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"parameters"`
}

type Request struct {
	Model          string
	Messages       []Message
	Tools          []Tool
	MaxTokens      int
	Temperature    float64
	ResponseSchema map[string]any
	// The compatibility probe fixes this per model revision. Empty uses the
	// standard Chat Completions completion budget parameter.
	OutputTokenParameter string
	ReasoningEffort      string
}

type Event struct {
	Type             string
	Text             string
	ToolCallID       string
	ToolName         string
	Arguments        string
	Input            int64
	Output           int64
	InputKnown       bool
	CachedInput      int64
	CachedInputKnown bool
	OutputKnown      bool
	StopReason       string
	Index            int
}

type Provider struct {
	Protocol Protocol
	BaseURL  string
	APIKey   string
	Client   *http.Client
	// A lower per-stream bound may be supplied; zero uses DefaultStreamTimeout.
	StreamTimeout time.Duration
}

const DefaultStreamTimeout = 10 * time.Minute

func (provider Provider) Stream(ctx context.Context, request Request, sink func(Event) error) (failure error) {
	if provider.Protocol != ProtocolChatCompletions && provider.Protocol != ProtocolResponses {
		return errors.New("unsupported model protocol")
	}
	client := provider.Client
	if client == nil {
		client = http.DefaultClient
	}
	deadline := DefaultStreamTimeout
	if provider.StreamTimeout > 0 && provider.StreamTimeout < deadline {
		deadline = provider.StreamTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	// Client.Timeout includes the entire SSE body. Keep the endpoint policy's
	// connection/header limits, but give streaming its own bounded deadline.
	// Copy the client so concurrent callers never mutate shared configuration.
	streamClient := *client
	streamClient.Timeout = 0
	endpoint, payload, err := provider.buildRequest(request)
	if err != nil {
		return err
	}
	started := time.Now()
	phase, status := "response_headers", 0
	digest := sha256.Sum256(payload)
	defer func() {
		if failure == nil || errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		var network net.Error
		timedOut := errors.Is(failure, context.DeadlineExceeded) || errors.As(failure, &network) && network.Timeout()
		// Never log endpoints, credentials, response bodies or raw errors.
		slog.Warn("model stream failed", "request_hash", hex.EncodeToString(digest[:]), "phase", phase, "http_status", status, "failure_kind", streamFailureKind(failure), "failure_origin", streamFailureOrigin(failure), "failure_type", streamFailureType(failure), "timeout", timedOut, "elapsed_ms", time.Since(started).Milliseconds())
	}()
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("Authorization", "Bearer "+provider.APIKey)
	response, err := streamClient.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	status = response.StatusCode
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		phase = "provider_status"
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 8192))
		return &HTTPStatusError{StatusCode: response.StatusCode}
	}
	aliases, err := toolAliases(request.Tools)
	if err != nil {
		return err
	}
	phase = "response_body"
	var reported TokenUsage
	err = provider.consumeSSE(response.Body, func(event Event) error {
		reported.Observe(event)
		if logical, exists := aliases[event.ToolName]; exists {
			event.ToolName = logical
		}
		return sink(event)
	})
	if err == nil && request.MaxTokens > 0 && reported.OutputSource() == "provider" && reported.Output > int64(request.MaxTokens) {
		return ErrBudgetNotEnforced
	}
	return err
}

func (provider Provider) buildRequest(request Request) (string, []byte, error) {
	effort, err := NormalizeReasoningEffort(provider.Protocol, request.ReasoningEffort)
	if err != nil {
		return "", nil, err
	}
	if _, err := toolAliases(request.Tools); err != nil {
		return "", nil, err
	}
	messages, err := encodeMessages(provider.Protocol, request.Messages)
	if err != nil {
		return "", nil, err
	}
	base, err := url.Parse(strings.TrimRight(provider.BaseURL, "/"))
	if err != nil {
		return "", nil, err
	}
	path := "/responses"
	body := map[string]any{"model": request.Model, "stream": true, "max_output_tokens": request.MaxTokens}
	if provider.Protocol == ProtocolChatCompletions {
		path = "/chat/completions"
		budgetParameter := request.OutputTokenParameter
		if budgetParameter == "" {
			budgetParameter = "max_completion_tokens"
		}
		if budgetParameter != "max_tokens" && budgetParameter != "max_completion_tokens" {
			return "", nil, errors.New("unsupported model output token parameter")
		}
		body = map[string]any{"model": request.Model, "messages": messages, "stream": true, budgetParameter: request.MaxTokens}
		body["stream_options"] = map[string]any{"include_usage": true}
		if len(request.Tools) > 0 {
			tools := make([]map[string]any, 0, len(request.Tools))
			for _, tool := range request.Tools {
				tool.Name = WireToolName(tool.Name)
				tools = append(tools, map[string]any{"type": "function", "function": tool})
			}
			body["tools"] = tools
		}
		if request.ResponseSchema != nil {
			body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": "argus_compatibility", "strict": true, "schema": request.ResponseSchema}}
		}
	} else {
		body["input"] = messages
		if len(request.Tools) > 0 {
			tools := make([]map[string]any, 0, len(request.Tools))
			for _, tool := range request.Tools {
				tools = append(tools, map[string]any{"type": "function", "name": WireToolName(tool.Name),
					"description": tool.Description, "parameters": tool.Schema, "strict": false})
			}
			body["tools"] = tools
		}
		if request.ResponseSchema != nil {
			body["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": "argus_compatibility", "strict": true, "schema": request.ResponseSchema}}
		}
	}
	if effort != "" {
		body["reasoning_effort"] = effort
	}
	base.Path = strings.TrimRight(base.Path, "/") + path
	payload, err := json.Marshal(body)
	return base.String(), payload, err
}

func (provider Provider) consumeSSE(reader io.Reader, sink func(Event) error) (err error) {
	var usage TokenUsage
	defer func() {
		if usage.Invalid() {
			err = ErrInvalidUsage
		}
	}()
	receive := func(event Event) error {
		usage.Observe(event)
		if err := sink(event); err != nil {
			return &streamStageError{stage: "event_sink", cause: err}
		}
		return nil
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	var data strings.Builder
	state := newStreamState()
	terminal := false
	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		raw := strings.TrimSuffix(data.String(), "\n")
		data.Reset()
		if raw == "[DONE]" {
			terminal = true
			return nil
		}
		if err := state.decode(provider.Protocol, []byte(raw), receive); err != nil {
			return err
		}
		// Chat Completions sends usage after finish_reason, before [DONE].
		// Responses includes its final usage in the terminal response event.
		terminal = provider.Protocol == ProtocolResponses && state.completed
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			if terminal {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return &streamStageError{stage: "response_read", cause: err}
	}
	if err := flush(); err != nil {
		return err
	}
	if !state.completed {
		return ErrIncompleteStream
	}
	return usage.Validate()
}
