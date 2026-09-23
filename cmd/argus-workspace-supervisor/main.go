package main

import (
	"context"
	"errors"
	"fmt"
	workspacev1 "github.com/kakj-go/Argus/internal/gen/proto/argus/workspace/v1"
	"github.com/kakj-go/Argus/internal/workspaceio"
	"github.com/kakj-go/Argus/internal/workspacesupervisor"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) == 4 && os.Args[1] == "--child" {
		limit, err := strconv.Atoi(os.Args[2])
		if err != nil {
			return err
		}
		return workspacesupervisor.Child(limit, os.Args[3])
	}
	uid, err := strconv.ParseUint(os.Getenv("ARGUS_WORKSPACE_USER_UID"), 10, 32)
	if err != nil {
		return err
	}
	fence, err := strconv.ParseInt(os.Getenv("ARGUS_WORKSPACE_FENCE"), 10, 64)
	if err != nil {
		return err
	}
	limit, err := strconv.Atoi(os.Getenv("ARGUS_WORKSPACE_PROCESS_LIMIT"))
	if err != nil {
		return err
	}
	service, err := workspacesupervisor.New(os.Getenv("ARGUS_WORKSPACE_ID"), fence, uint32(uid), limit)
	if err != nil {
		return err
	}
	config, err := workspaceio.TLS("/var/run/argus/manager/tls/tls.crt", "/var/run/argus/manager/tls/tls.key", "/var/run/argus/manager/tls/ca.crt", "", true, os.Getenv("ARGUS_WORKSPACE_CLIENT_NAME"))
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", ":8448")
	if err != nil {
		return err
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(config)), grpc.MaxRecvMsgSize(1<<20), grpc.MaxSendMsgSize(1<<20))
	workspacev1.RegisterWorkspaceSupervisorServiceServer(server, service)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
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
					stop()
					return
				}
			}
		}
	}()
	health := &http.Server{Addr: ":44772", ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && (r.URL.Path == "/ping" || r.URL.Path == "/health") {
			w.WriteHeader(200)
			_, _ = w.Write([]byte("pong"))
			return
		}
		http.NotFound(w, r)
	})}
	healthListener, err := net.Listen("tcp", health.Addr)
	if err != nil {
		_ = listener.Close()
		return err
	}
	healthError := make(chan error, 1)
	go func() {
		if err := health.Serve(healthListener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			healthError <- err
			stop()
		}
	}()
	go func() { <-ctx.Done(); server.Stop(); _ = health.Close() }()
	err = server.Serve(listener)
	select {
	case err := <-healthError:
		return err
	default:
	}
	if ctx.Err() != nil {
		return nil
	}
	return err
}
