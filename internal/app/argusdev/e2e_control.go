package argusdev

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

var errE2EOperatorCancelled = errors.New("E2E cancelled by a run-scoped operator request")

type e2eControlRequest struct {
	RunID  string `json:"run_id"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

func readE2EControl(path, runID string) (*e2eControlRequest, error) {
	if runID == "" {
		return nil, errors.New("E2E control requires a nonempty run identity")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 4096 {
		return nil, errors.New("invalid E2E control file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(raw) > 4096 {
		return nil, errors.New("invalid E2E control file size")
	}
	// Windows PowerShell's UTF-8 output can include a BOM.
	raw = bytes.TrimPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request e2eControlRequest
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.RunID != runID || request.Action != "cancel" || len(request.Reason) > 512 {
		return nil, errors.New("E2E control request does not match this run")
	}
	return &request, nil
}

// File control avoids killing the Windows go-run process group. Cancellation
// flows through the existing Run stop, accounting drain and fenced cleanup.
func controlledE2EContext(parent context.Context, artifacts, runID string) (context.Context, func()) {
	ctx, cancel := context.WithCancelCause(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			request, err := readE2EControl(filepath.Join(artifacts, "cancel-request.json"), runID)
			if err == nil && request != nil {
				digest := sha256.Sum256([]byte(request.Reason))
				ack, _ := json.MarshalIndent(map[string]any{"run_id": runID, "action": "cancel", "observed_at": time.Now().UTC(), "reason_sha256": hex.EncodeToString(digest[:]), "state": "cancellation_requested_cleanup_pending"}, "", "  ")
				file, openErr := os.OpenFile(filepath.Join(artifacts, "cancel-ack.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
				if openErr == nil {
					_, _ = file.Write(ack)
					_ = file.Close()
				}
				cancel(errE2EOperatorCancelled)
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return ctx, func() { cancel(nil); <-done }
}
