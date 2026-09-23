package opensandbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Endpoint struct {
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers"`
}
type CommandResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Partial  bool   `json:"partial"`
}

func (client *Client) Endpoint(ctx context.Context, id string, port int) (Endpoint, error) {
	var result Endpoint
	err := client.request(ctx, http.MethodGet, "/sandboxes/"+url.PathEscape(id)+"/endpoints/"+strconv.Itoa(port)+"?use_server_proxy=true", nil, &result)
	return result, err
}

func (client *Client) RunCommand(ctx context.Context, id, command string, timeoutSeconds int) (CommandResult, error) {
	endpoint, err := client.Endpoint(ctx, id, 44772)
	if err != nil {
		return CommandResult{}, err
	}
	address := endpoint.Endpoint
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return CommandResult{}, err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/command"
	data, _ := json.Marshal(map[string]any{"command": command, "cwd": "/workspace", "background": false, "timeout": timeoutSeconds * 1000})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, parsed.String(), bytes.NewReader(data))
	if err != nil {
		return CommandResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}
	commandClient := *client.http
	commandClient.Timeout = time.Duration(timeoutSeconds+10) * time.Second
	res, err := commandClient.Do(req)
	if err != nil {
		return CommandResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return CommandResult{}, errors.New("sandbox command request failed")
	}
	result := CommandResult{}
	scanner := bufio.NewScanner(io.LimitReader(res.Body, 4<<20))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	appendOutput := func(target *string, text string) {
		const limit = 64 << 10
		if len(*target)+len(text) > limit {
			remaining := limit - len(*target)
			if remaining > 0 {
				*target += text[:remaining]
			}
			result.Partial = true
		} else {
			*target += text
		}
	}
	for scanner.Scan() {
		// The pinned Execd emits JSON records separated by blank lines under
		// text/event-stream, without the SSE "data:" prefix.
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			ExitCode *int            `json:"exit_code"`
			Error    json.RawMessage `json:"error"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			return result, errors.New("invalid sandbox command event")
		}
		text := event.Text
		switch event.Type {
		case "stdout":
			appendOutput(&result.Stdout, text)
		case "stderr":
			appendOutput(&result.Stderr, text)
		case "execution_complete":
			if event.ExitCode != nil {
				result.ExitCode = *event.ExitCode
			}
			return result, nil
		case "error":
			var detail struct {
				EName  string `json:"ename"`
				EValue string `json:"evalue"`
			}
			if json.Unmarshal(event.Error, &detail) != nil || detail.EName == "" || detail.EValue == "" {
				return result, errors.New("invalid sandbox error event")
			}
			result.ExitCode = 1
			if code, err := strconv.Atoi(detail.EValue); err == nil && code != 0 {
				result.ExitCode = code
			}
			appendOutput(&result.Stderr, detail.EName+": "+detail.EValue)
			// Execd v1.0.22 emits error instead of execution_complete when a
			// foreground process exits unsuccessfully. It is a terminal result.
			return result, nil
		case "init", "ping", "status", "execution_count":
		default:
			return result, errors.New("unsupported sandbox command event")
		}
	}
	if scanner.Err() != nil {
		return result, scanner.Err()
	}
	return result, errors.New("sandbox command stream incomplete")
}
