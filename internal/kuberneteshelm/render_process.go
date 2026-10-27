package kuberneteshelm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"helm.sh/helm/v4/pkg/chart/common"
)

const renderCommand = "--argus-internal-helm-render-v1"
const rendererSchema = "argus.helm_renderer/v1"

var renderSlots = make(chan struct{}, 1)

type rendererInput struct {
	SchemaVersion string               `json:"schema_version"`
	Request       Request              `json:"request"`
	Material      Material             `json:"material"`
	Capabilities  *common.Capabilities `json:"capabilities"`
}
type rendererOutput struct {
	SchemaVersion string `json:"schema_version"`
	Result        frozen `json:"result"`
	ErrorCode     string `json:"error_code,omitempty"`
}
type renderLimiter interface {
	Attach(*os.Process) error
	Close() error
}
type renderBuffer struct{ bytes.Buffer }

func (b *renderBuffer) Write(value []byte) (int, error) {
	if b.Len()+len(value) > MaxCommandBytes {
		return 0, ErrBudget
	}
	return b.Buffer.Write(value)
}

func renderIsolated(ctx context.Context, r Request, m Material, caps *common.Capabilities) (frozen, error) {
	// Origin authentication belongs to the parent fetch, never the renderer.
	m.SourceCredentials = nil
	var empty frozen
	select {
	case renderSlots <- struct{}{}:
	case <-ctx.Done():
		return empty, ErrUnavailable
	}
	defer func() { <-renderSlots }()
	input := rendererInput{SchemaVersion: rendererSchema, Request: Request{Name: r.Name, Namespace: Namespace{Name: r.Namespace.Name}, Operation: r.Operation, ExpectedRevision: r.ExpectedRevision}, Material: Material{Chart: m.Chart, Values: m.Values}, Capabilities: caps}
	raw, e := json.Marshal(input)
	if e != nil || len(raw) > MaxCommandBytes {
		return empty, ErrBudget
	}
	defer clear(raw)
	executable, e := os.Executable()
	if e != nil {
		return empty, ErrUnavailable
	}
	call, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(call, executable, renderCommand)
	command.Env = []string{"GOMAXPROCS=1", "GOMEMLIMIT=96MiB"}
	if root := os.Getenv("SystemRoot"); root != "" {
		command.Env = append(command.Env, "SystemRoot="+root)
	}
	configureRenderCommand(command)
	var output renderBuffer
	command.Stdout = &output
	command.Stderr = io.Discard
	command.WaitDelay = time.Second
	stdin, e := command.StdinPipe()
	if e != nil {
		return empty, ErrUnavailable
	}
	limiter, e := newRenderLimiter()
	if e != nil {
		return empty, ErrUnavailable
	}
	defer limiter.Close()
	if e = command.Start(); e != nil {
		stdin.Close()
		return empty, ErrUnavailable
	}
	if e = limiter.Attach(command.Process); e != nil {
		stdin.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
		return empty, ErrUnavailable
	}
	_, writeErr := stdin.Write(raw)
	stdin.Close()
	waitErr := command.Wait()
	defer clear(output.Bytes())
	if writeErr != nil || waitErr != nil || call.Err() != nil {
		return empty, ErrBudget
	}
	var response rendererOutput
	if json.Unmarshal(output.Bytes(), &response) != nil || response.SchemaVersion != rendererSchema {
		return empty, ErrInvalid
	}
	if e = ErrorFromCode(response.ErrorCode); e != nil {
		return empty, e
	}
	return response.Result, nil
}

// RunRenderCommand is called before normal flag/logging/service initialization
// in server, worker and Connector mains. It is a private stdin/stdout protocol;
// it never loads platform configuration, credentials or ambient kubeconfig.
func RunRenderCommand(args []string, in io.Reader, out io.Writer) bool {
	if len(args) != 1 || args[0] != renderCommand {
		return false
	}
	log.SetOutput(io.Discard)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := rendererOutput{SchemaVersion: rendererSchema}
	if e := limitRenderSelf(); e != nil {
		response.ErrorCode = ErrBudget.Error()
		_ = json.NewEncoder(out).Encode(response)
		return true
	}
	raw, e := io.ReadAll(io.LimitReader(in, MaxCommandBytes+1))
	defer clear(raw)
	var input rendererInput
	if e != nil || len(raw) > MaxCommandBytes || json.Unmarshal(raw, &input) != nil || input.SchemaVersion != rendererSchema || input.Capabilities == nil || len(input.Material.Key) != 0 {
		response.ErrorCode = ErrInvalid.Error()
	} else {
		defer func() { clear(input.Material.Chart); clear(input.Material.Values) }()
		response.Result, e = renderLocal(input.Request, input.Material, input.Capabilities)
		response.ErrorCode = ErrorCode(e)
		if e != nil {
			response.Result = frozen{}
		}
	}
	_ = json.NewEncoder(out).Encode(response)
	return true
}
