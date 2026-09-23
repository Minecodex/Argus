// Package workspacesupervisor runs only in an offline compute Pod. Commands
// use a different non-root UID; the manager's TLS keys and capabilities are
// never inherited by user code. The worker-host and file-only role do not run it.
package workspacesupervisor

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/workspacelease"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const ManagerUID = 10001

type Server struct {
	workspacev1.UnimplementedWorkspaceSupervisorServiceServer
	WorkspaceID  string
	Generation   int64
	UserUID      uint32
	ProcessLimit int
	lease        *workspacelease.Lease
	execute      sync.Mutex
}

func (s *Server) IdleFor(duration time.Duration) bool { return s.lease.IdleFor(duration) }

func New(id string, generation int64, uid uint32, limit int) (*Server, error) {
	if id == "" || generation <= 0 || uid < 100000 || limit < 16 || limit > 4096 {
		return nil, errors.New("invalid supervisor identity")
	}
	if err := platformReady(); err != nil {
		return nil, err
	}
	return &Server{WorkspaceID: id, Generation: generation, UserUID: uid, ProcessLimit: limit, lease: workspacelease.New(id, generation)}, nil
}

func (s *Server) cleanup(ctx context.Context) error { return sweepUserProcesses(ctx, s.UserUID) }
func (s *Server) BindLease(ctx context.Context, r *workspacev1.WorkspaceSupervisorServiceBindLeaseRequest) (*workspacev1.WorkspaceSupervisorServiceBindLeaseResponse, error) {
	if err := s.lease.Bind(ctx, r.WorkspaceId, r.Generation, r.FenceToken, s.cleanup); err != nil {
		return nil, status.Error(codes.FailedPrecondition, "supervisor lease could not be fenced")
	}
	return &workspacev1.WorkspaceSupervisorServiceBindLeaseResponse{}, nil
}
func (s *Server) ReleaseLease(ctx context.Context, r *workspacev1.WorkspaceSupervisorServiceReleaseLeaseRequest) (*workspacev1.WorkspaceSupervisorServiceReleaseLeaseResponse, error) {
	if r.Authority == nil {
		return nil, status.Error(codes.PermissionDenied, "missing authority")
	}
	if err := s.lease.Release(ctx, r.Authority.WorkspaceId, r.Authority.FenceToken, s.cleanup); err != nil {
		return nil, status.Error(codes.FailedPrecondition, "user processes were not revoked")
	}
	return &workspacev1.WorkspaceSupervisorServiceReleaseLeaseResponse{}, nil
}

func (s *Server) Execute(ctx context.Context, r *workspacev1.ExecuteRequest) (*workspacev1.ExecuteResponse, error) {
	if r.Authority == nil || r.Command == "" || len(r.Command) > 64<<10 || r.TimeoutSeconds < 1 || r.TimeoutSeconds > 300 {
		return nil, status.Error(codes.InvalidArgument, "invalid command")
	}
	ctx, done, err := s.lease.Begin(ctx, r.Authority.WorkspaceId, r.Authority.FenceToken)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "stale command authority")
	}
	defer done()
	s.execute.Lock()
	defer s.execute.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, status.Error(codes.Canceled, "command cancelled")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(r.TimeoutSeconds)*time.Second)
	defer cancel()
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, self, "--child", strconv.Itoa(s.ProcessLimit), r.Command)
	configureChild(cmd, s.UserUID)
	cmd.Dir = "/workspace"
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/home/argus", "LANG=C.UTF-8", "PYTHONUNBUFFERED=1", "PYTHONDONTWRITEBYTECODE=1", "MPLCONFIGDIR=/home/argus/.matplotlib"}
	stdout, stderr := &boundedOutput{}, &boundedOutput{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
	cleanupErr := s.cleanup(cleanupCtx)
	cleanupCancel()
	if cleanupErr != nil {
		return nil, status.Error(codes.Unavailable, "user process cleanup failed")
	}
	if ctx.Err() != nil {
		return nil, status.Error(codes.Canceled, "command cancelled or timed out")
	}
	code := int32(0)
	if cmd.ProcessState != nil {
		code = int32(cmd.ProcessState.ExitCode())
	}
	if code < 0 {
		code = 137
	}
	if err != nil && cmd.ProcessState == nil {
		slog.Error("offline command process could not start", "error", err)
		return nil, status.Error(codes.Unavailable, "command could not start")
	}
	return &workspacev1.ExecuteResponse{Stdout: strings.ToValidUTF8(stdout.text.String(), "�"), Stderr: strings.ToValidUTF8(stderr.text.String(), "�"), ExitCode: code, Partial: stdout.partial || stderr.partial || errors.Is(err, exec.ErrWaitDelay)}, nil
}

type boundedOutput struct {
	text    strings.Builder
	partial bool
}

func (w *boundedOutput) Write(value []byte) (int, error) {
	n := len(value)
	remaining := (64 << 10) - w.text.Len()
	if len(value) > remaining {
		value = value[:remaining]
		w.partial = true
	}
	_, _ = w.text.Write(value)
	return n, nil
}
