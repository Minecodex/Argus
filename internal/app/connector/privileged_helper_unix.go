//go:build !windows

package connector

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kakj-go/Argus/internal/collectormanager"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

const privilegedSocketPath = "/run/argus-connector/privileged.sock"

type privilegedRequest struct {
	Operation string `json:"operation"`
	Payload   []byte `json:"payload,omitempty"`
}

type privilegedResponse struct {
	Payload    []byte `json:"payload,omitempty"`
	Error      string `json:"error,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	ErrorStage string `json:"error_stage,omitempty"`
}

type privilegedCollectorError struct {
	message, code, stage string
}

func (err privilegedCollectorError) Error() string { return err.message }
func (err privilegedCollectorError) failureCode() string {
	return err.code
}
func (err privilegedCollectorError) failureStage() string {
	return err.stage
}

func runPrivilegedHelper(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("argus-connector privileged-helper", flag.ContinueOnError)
	socket := flags.String("socket", privilegedSocketPath, "Privileged helper Unix socket")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if os.Geteuid() != 0 || *socket != privilegedSocketPath {
		return errors.New("privileged helper must run as root on its fixed socket")
	}
	if err := os.MkdirAll(filepath.Dir(*socket), 0o750); err != nil {
		return err
	}
	_ = os.Remove(*socket)
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	defer os.Remove(*socket)
	if err = os.Chmod(*socket, 0o660); err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if ctx.Err() != nil {
				return nil
			}
			return acceptErr
		}
		go handlePrivilegedConnection(ctx, connection)
	}
}

func handlePrivilegedConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(5 * time.Minute))
	var request privilegedRequest
	if err := json.NewDecoder(io.LimitReader(connection, 16<<20)).Decode(&request); err != nil {
		_ = json.NewEncoder(connection).Encode(privilegedResponse{Error: "invalid helper request"})
		return
	}
	response := privilegedResponse{}
	switch request.Operation {
	case "collector_management":
		var payload anypb.Any
		if proto.Unmarshal(request.Payload, &payload) != nil {
			response.Error = "invalid Collector command"
			break
		}
		result, err := executeCollectorManagement(ctx, &payload, nil)
		if err != nil {
			response.Error = err.Error()
			response.ErrorCode = collectormanager.FailureCode(err)
			response.ErrorStage = collectormanager.FailureStage(err)
			slog.Warn("privileged Collector command failed", "error_code", response.ErrorCode, "failure_stage", response.ErrorStage, "error", response.Error)
			break
		}
		typed, err := anypb.New(result)
		if err != nil {
			response.Error = err.Error()
			break
		}
		response.Payload, err = proto.Marshal(typed)
		if err != nil {
			response.Error = err.Error()
		}
	case "finalize_uninstall":
		if err := scheduleLinuxUninstall(); err != nil {
			response.Error = err.Error()
		}
	default:
		response.Error = "unsupported helper operation"
	}
	_ = json.NewEncoder(connection).Encode(response)
}

func usePrivilegedCollectorHelper() bool { return os.Geteuid() != 0 }

func executePrivilegedCollector(ctx context.Context, payload *anypb.Any) (*connectorv1.CollectorManagementResult, error) {
	encoded, err := proto.Marshal(payload)
	if err != nil {
		return nil, err
	}
	response, err := callPrivilegedHelper(ctx, privilegedRequest{Operation: "collector_management", Payload: encoded})
	if err != nil {
		return nil, err
	}
	var typed anypb.Any
	if proto.Unmarshal(response.Payload, &typed) != nil {
		return nil, errors.New("privileged helper returned an invalid Collector result")
	}
	var result connectorv1.CollectorManagementResult
	if typed.UnmarshalTo(&result) != nil {
		return nil, errors.New("privileged helper returned the wrong result type")
	}
	return &result, nil
}

func finalizeLocalUninstall(ctx context.Context, _ string) error {
	_, err := callPrivilegedHelper(ctx, privilegedRequest{Operation: "finalize_uninstall"})
	return err
}

func callPrivilegedHelper(ctx context.Context, request privilegedRequest) (privilegedResponse, error) {
	dialer := net.Dialer{Timeout: 3 * time.Second}
	connection, err := dialer.DialContext(ctx, "unix", privilegedSocketPath)
	if err != nil {
		return privilegedResponse{}, err
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	if err = json.NewEncoder(connection).Encode(request); err != nil {
		return privilegedResponse{}, err
	}
	var response privilegedResponse
	if err = json.NewDecoder(io.LimitReader(connection, 16<<20)).Decode(&response); err != nil {
		return privilegedResponse{}, err
	}
	if response.Error != "" {
		return privilegedResponse{}, privilegedCollectorError{message: response.Error, code: response.ErrorCode, stage: response.ErrorStage}
	}
	return response, nil
}

func scheduleLinuxUninstall() error {
	unit := fmt.Sprintf("argus-connector-cleanup-%d", os.Getpid())
	script := `set -eu
systemctl disable argus-connector.service argus-connector-privileged.service >/dev/null 2>&1 || true
systemctl stop argus-connector.service argus-connector-privileged.service >/dev/null 2>&1 || true
rm -f /etc/systemd/system/argus-connector.service /etc/systemd/system/argus-connector-privileged.service /usr/local/bin/argus-connector
rm -rf /etc/argus-connector /var/lib/argus-connector
systemctl daemon-reload`
	command := exec.Command("systemd-run", "--unit", unit, "--collect", "--on-active=1s", "/bin/sh", "-c", script)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("schedule Connector cleanup: %w: %s", err, output)
	}
	return nil
}
