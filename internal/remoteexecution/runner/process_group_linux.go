//go:build linux

package runner

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func processGroupHasChildren(group int) (bool, error) { return processGroupActive(group, group) }
func processGroupActive(group, ignore int) (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		pid, e := strconv.Atoi(entry.Name())
		if e != nil || pid == ignore {
			continue
		}
		raw, e := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return false, errors.New("REMOTE_STOP_UNKNOWN")
		}
		end := strings.LastIndexByte(string(raw), ')')
		if end < 0 {
			return false, errors.New("REMOTE_STOP_UNKNOWN")
		}
		fields := strings.Fields(string(raw[end+1:]))
		if len(fields) < 3 {
			return false, errors.New("REMOTE_STOP_UNKNOWN")
		}
		if fields[2] == strconv.Itoa(group) && fields[0] != "Z" && fields[0] != "X" {
			return true, nil
		}
	}
	return false, nil
}

func waitProcessGroupStopped(group int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		alive, err := processGroupActive(group, 0)
		if err != nil {
			return err
		}
		if !alive {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("REMOTE_STOP_UNKNOWN")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
