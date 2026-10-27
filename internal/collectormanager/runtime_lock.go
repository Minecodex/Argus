package collectormanager

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

const runtimeLockFile = ".collector-control.lock"

func lockRuntime(ctx context.Context, root string) (func(), error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(root, runtimeLockFile), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		locked, err := tryRuntimeLock(file)
		if err != nil {
			file.Close()
			return nil, err
		}
		if locked {
			return func() { releaseRuntimeLock(file); file.Close() }, nil
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-deadline.Done():
			timer.Stop()
			file.Close()
			return nil, deadline.Err()
		case <-timer.C:
		}
	}
}
