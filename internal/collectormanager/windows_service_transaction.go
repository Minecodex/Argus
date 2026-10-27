//go:build windows

package collectormanager

import (
	"context"
	"errors"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

func stopWindowsRuntime(ctx context.Context, service *mgr.Service) error {
	if service == nil {
		return nil
	}
	state, err := service.Query()
	if err != nil {
		return err
	}
	if state.State == svc.Stopped {
		return nil
	}
	if _, err = service.Control(svc.Stop); err != nil && !errors.Is(err, windows.ERROR_SERVICE_NOT_ACTIVE) {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err = service.Query()
		if err != nil {
			return err
		}
		if state.State == svc.Stopped {
			return nil
		}
		select {
		case <-deadline.Done():
			return deadline.Err()
		case <-ticker.C:
		}
	}
}

// Only change the binary command. Preserve the account, credentials, dependencies
// and recovery policy of an existing managed service, including during rollback.
func changeWindowsRuntimeBinary(service *mgr.Service, command string) error {
	value, err := windows.UTF16PtrFromString(command)
	if err != nil {
		return err
	}
	return windows.ChangeServiceConfig(service.Handle, windows.SERVICE_NO_CHANGE, windows.SERVICE_NO_CHANGE, windows.SERVICE_NO_CHANGE, value, nil, nil, nil, nil, nil, nil)
}
