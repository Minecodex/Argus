//go:build !windows

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	connectorapp "github.com/kakj-go/Argus/internal/app/connector"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
)

func main() {
	if kuberneteshelm.RunRenderCommand(os.Args[1:], os.Stdin, os.Stdout) {
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := connectorapp.Run(ctx, logger); err != nil {
		logger.Error("argus-connector stopped with an error", "error", err)
		os.Exit(1)
	}
}
