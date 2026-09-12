//go:build windows

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/signal"

	connectorapp "github.com/kakj-go/Argus/internal/app/connector"
	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "ArgusConnector"

type serviceHandler struct{}

func (serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes <- svc.Status{State: svc.StartPending}
	done := make(chan error, 1)
	go func() {
		logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
		done <- connectorapp.Run(ctx, logger)
	}()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			changes <- svc.Status{State: svc.StopPending}
			if err != nil {
				return false, 1
			}
			return false, 0
		case request := <-requests:
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
			}
		}
	}
}

func main() {
	isService, err := svc.IsWindowsService()
	if err == nil && isService {
		serviceName := windowsServiceName
		if len(os.Args) > 1 && os.Args[1] == "collector-service" {
			serviceName = "ArgusCollector"
		}
		if err = svc.Run(serviceName, serviceHandler{}); err != nil {
			os.Exit(1)
		}
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err = connectorapp.Run(ctx, logger); err != nil {
		logger.Error("argus-connector stopped with an error", "error", err)
		os.Exit(1)
	}
}
