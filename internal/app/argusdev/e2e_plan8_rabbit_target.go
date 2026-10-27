package argusdev

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// The target stays on the owned internal network. Authentication and request
// bodies travel only through stdin to its loopback management endpoint.
func (a *App) plan8TargetHTTP(ctx context.Context, target, port, method, path, username, password string, body []byte) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, "http://127.0.0.1:"+port+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	request.Close = true
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(username, password)
	var input, output bytes.Buffer
	if err = request.Write(&input); err != nil {
		return 0, nil, err
	}
	callErr := a.runner.RunIO(ctx, nil, &input, &output, io.Discard, "docker", "exec", "-i", target, "sh", "-c", "{ cat; sleep 1; } | nc -w 3 127.0.0.1 "+port)
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(output.Bytes())), request)
	if err != nil {
		return 0, nil, fmt.Errorf("owned target HTTP response unavailable: bytes=%d call_failed=%t", output.Len(), callErr != nil)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return 0, nil, errors.New("owned target HTTP body incomplete")
	}
	if callErr != nil && response.StatusCode == 0 {
		return 0, nil, errors.New("owned target HTTP request failed")
	}
	return response.StatusCode, data, nil
}

func (a *App) preparePlan8RabbitTarget(ctx context.Context, env *E2EEnvironment, host *plan6HostLoop) (row plan8ExtraTarget, failure error) {
	if host == nil || host.Docker == nil || !filepath.IsAbs(env.WorkDir) {
		return row, errors.New("owned RabbitMQ environment missing")
	}
	network := ""
	for _, n := range host.Docker.Networks {
		if n.Internal {
			network = n.Name
		}
	}
	if network == "" {
		return row, errors.New("owned customer network missing")
	}
	row.plan8DatabaseFixture = plan8DatabaseFixture{owner: "plan8-rabbit-" + env.Options.RunID, name: kubernetesNameForDev("p8-rabbit-" + env.Options.RunID), engine: "rabbitmq", password: "Monitor-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")}
	row.endpoint, row.metric = "http://"+row.name+":15672", "rabbitmq_node_mem_used"
	rootPassword := "Root-P8-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	private := filepath.Join(env.WorkDir, "plan8-rabbitmq.env")
	if err := os.WriteFile(private, []byte("RABBITMQ_DEFAULT_USER=bootstrap\nRABBITMQ_DEFAULT_PASS="+rootPassword+"\nRABBITMQ_SERVER_ADDITIONAL_ERL_ARGS=+S 2:2 +A 4\n"), 0600); err != nil {
		return row, err
	}
	save := func() error {
		body, _ := json.MarshalIndent(map[string]any{"owner": row.owner, "network": network, "name": row.name, "id": row.id, "engine": row.engine, "image_id": row.imageID, "runtime_version": row.version}, "", "  ")
		return writePrivate(filepath.Join(env.Options.Artifacts, "plan8-rabbitmq-ownership.json"), body)
	}
	if err := save(); err != nil {
		return row, err
	}
	id, err := a.runner.Output(ctx, nil, "docker", "run", "--detach", "--name", row.name, "--label", "io.argus.e2e.run="+row.owner, "--network", network, "--memory=512m", "--cpus=1", "--env-file", private, "rabbitmq:4-management-alpine")
	if err != nil {
		return row, errors.New("owned RabbitMQ start failed")
	}
	row.id = strings.TrimSpace(id)
	if err = save(); err != nil {
		return row, err
	}
	call := func(user, password, method, path string, body any) (int, []byte, error) {
		var data []byte
		if body != nil {
			data, _ = json.Marshal(body)
		}
		return a.plan8TargetHTTP(ctx, row.id, "15672", method, path, user, password, data)
	}
	lastStatus, lastFailure := 0, ""
	deadline := time.Now().Add(90 * time.Second)
	for {
		status, _, e := call("bootstrap", rootPassword, http.MethodGet, "/api/overview", nil)
		lastStatus = status
		if e != nil {
			lastFailure = e.Error()
		}
		if e == nil && status == 200 {
			break
		}
		if time.Now().After(deadline) {
			state, _ := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.State.Status}}|{{.State.ExitCode}}|{{.State.OOMKilled}}", row.id)
			logs, _ := a.runner.Output(ctx, nil, "docker", "logs", "--tail", "20", row.id)
			logs = strings.ReplaceAll(strings.ReplaceAll(logs, rootPassword, "[redacted]"), row.password, "[redacted]")
			body, _ := json.MarshalIndent(map[string]any{"status": lastStatus, "failure": lastFailure, "state": strings.TrimSpace(state), "owned_redacted_log": logs}, "", "  ")
			_ = writePrivate(filepath.Join(env.Root, "artifacts", "plan8-rabbit-fixture-readiness.json"), body)
			return row, fmt.Errorf("owned RabbitMQ management readiness timed out: status=%d cause=%s", lastStatus, lastFailure)
		}
		if err = waitContext(ctx, 300*time.Millisecond); err != nil {
			return row, err
		}
	}
	for _, operation := range []struct {
		path string
		body any
	}{
		{"/api/users/monitor", map[string]any{"password": row.password, "tags": "monitoring"}},
		{"/api/permissions/%2F/monitor", map[string]any{"configure": "^$", "write": "^$", "read": "^$"}},
		{"/api/queues/%2F/plan8-owned", map[string]any{"durable": true, "auto_delete": false, "arguments": map[string]any{}}},
	} {
		status, _, e := call("bootstrap", rootPassword, http.MethodPut, operation.path, operation.body)
		if e != nil || (status != 201 && status != 204) {
			return row, errors.New("RabbitMQ monitoring role or actual queue setup failed")
		}
	}
	status, data, err := call("monitor", row.password, http.MethodGet, "/api/overview", nil)
	if err != nil || status != 200 {
		return row, errors.New("RabbitMQ monitoring identity cannot inspect actual broker")
	}
	var overview struct {
		Version string `json:"rabbitmq_version"`
	}
	if json.Unmarshal(data, &overview) != nil || overview.Version == "" || len(overview.Version) > 128 {
		return row, errors.New("RabbitMQ runtime version missing")
	}
	row.version = overview.Version
	status, _, err = call("monitor", row.password, http.MethodGet, "/api/nodes", nil)
	if err != nil || status != 200 {
		return row, errors.New("RabbitMQ monitoring identity cannot inspect nodes")
	}
	status, _, err = call("monitor", row.password, http.MethodPut, "/api/queues/%2F/forbidden", map[string]any{"durable": true, "arguments": map[string]any{}})
	if err != nil || status != 401 && status != 403 {
		return row, errors.New("RabbitMQ monitoring identity did not explicitly reject a queue write")
	}
	status, _, err = call("monitor", "invalid-"+row.password, http.MethodGet, "/api/overview", nil)
	if err != nil || status != 401 {
		return row, errors.New("RabbitMQ incorrect authentication was not explicitly rejected")
	}
	image, err := a.runner.Output(ctx, nil, "docker", "inspect", "--format", "{{.Image}}", row.id)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(image), "sha256:") {
		return row, errors.New("RabbitMQ immutable image identity missing")
	}
	row.imageID = strings.TrimSpace(image)
	return row, save()
}
