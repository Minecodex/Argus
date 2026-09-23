package worker

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"strconv"
	"time"

	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/workspacefs"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func runWorkspaceIO(ctx context.Context, logger *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	id := os.Getenv("ARGUS_WORKSPACE_ID")
	fence, _ := strconv.ParseInt(os.Getenv("ARGUS_WORKSPACE_FENCE"), 10, 64)
	if id == "" || fence <= 0 {
		return errors.New("workspace IO identity is required")
	}
	root := os.Getenv("ARGUS_WORKSPACE_ROOT")
	if root == "" {
		root = "/workspace"
	}
	limit, _ := strconv.ParseInt(os.Getenv("ARGUS_WORKSPACE_MAX_FILE_BYTES"), 10, 64)
	files, err := workspacefs.Open(root, limit)
	if err != nil {
		return err
	}
	defer files.Close()
	config, err := workspaceio.TLS(os.Getenv("ARGUS_WORKSPACE_TLS_CERT"), os.Getenv("ARGUS_WORKSPACE_TLS_KEY"), os.Getenv("ARGUS_WORKSPACE_TLS_CA"), "", true, os.Getenv("ARGUS_WORKSPACE_CLIENT_NAME"))
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", ":8447")
	if err != nil {
		return err
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(config)), grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(1<<20))
	service := &workspaceio.Server{Files: files, WorkspaceID: id, Fence: fence}
	workspacev1.RegisterWorkspaceIOServiceServer(server, service)
	idleSeconds, _ := strconv.Atoi(os.Getenv("ARGUS_WORKSPACE_IDLE_SECONDS"))
	if idleSeconds <= 0 {
		idleSeconds = 900
	}
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if service.IdleFor(time.Duration(idleSeconds) * time.Second) {
					cancel()
					server.Stop()
					return
				}
			}
		}
	}()
	go func() { <-ctx.Done(); server.Stop() }()
	logger.Info("workspace file RPC ready", "workspace_id", id, "fence", fence)
	err = server.Serve(listener)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
