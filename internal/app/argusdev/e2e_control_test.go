package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestE2EControlRequiresExplicitMatchingBoundedRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.json")
	for _, input := range []string{`{"run_id":"other","action":"cancel"}`, `{"run_id":"owned","action":"continue"}`, `{"run_id":"owned","action":"cancel","unexpected":true}`, `{"run_id":"owned","action":"cancel"} {}`, strings.Repeat("x", 4097)} {
		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatal(err)
		}
		if request, err := readE2EControl(path, "owned"); err == nil || request != nil {
			t.Fatal("foreign, malformed or oversized control was accepted")
		}
	}
	if err := os.WriteFile(path, []byte(`{"run_id":"owned","action":"cancel"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if request, err := readE2EControl(path, "owned"); err != nil || request == nil {
		t.Fatal("matching explicit cancellation rejected", err)
	}
	if request, err := readE2EControl(path, ""); err == nil || request != nil {
		t.Fatal("empty run identity was accepted")
	}
	withBOM := append([]byte{0xef, 0xbb, 0xbf}, []byte(`{"run_id":"owned","action":"cancel"}`)...)
	if err := os.WriteFile(path, withBOM, 0o600); err != nil {
		t.Fatal(err)
	}
	if request, err := readE2EControl(path, "owned"); err != nil || request == nil {
		t.Fatal("valid Windows UTF-8 cancellation rejected", err)
	}
}

func TestE2EControlCancelsOnlyOwnedChildAndAcknowledgesWithoutRawReason(t *testing.T) {
	dir := t.TempDir()
	parent, cancelParent := context.WithCancel(t.Context())
	defer cancelParent()
	ctx, stop := controlledE2EContext(parent, dir, "owned")
	defer stop()
	raw, _ := json.Marshal(e2eControlRequest{RunID: "owned", Action: "cancel", Reason: "private operator reason"})
	if err := os.WriteFile(filepath.Join(dir, "cancel-request.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("operator cancellation was not observed")
	}
	if !errors.Is(context.Cause(ctx), errE2EOperatorCancelled) || parent.Err() != nil {
		t.Fatal("cancellation escaped the owned context")
	}
	ack, err := os.ReadFile(filepath.Join(dir, "cancel-ack.json"))
	if err != nil || strings.Contains(string(ack), "private operator reason") {
		t.Fatal("acknowledgement missing or raw reason leaked", err)
	}
	var value map[string]any
	if json.Unmarshal(ack, &value) != nil || value["run_id"] != "owned" || value["state"] != "cancellation_requested_cleanup_pending" {
		t.Fatal("acknowledgement falsely claimed cleanup complete")
	}
}

func TestE2EControlIgnoresForeignRunAndStopsWithParent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cancel-request.json"), []byte(`{"run_id":"foreign","action":"cancel"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	parent, cancel := context.WithCancel(t.Context())
	ctx, stop := controlledE2EContext(parent, dir, "owned")
	defer stop()
	select {
	case <-ctx.Done():
		t.Fatal("foreign run cancelled the active test")
	case <-time.After(250 * time.Millisecond):
	}
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("parent cancellation was ignored")
	}
	if _, err := os.Stat(filepath.Join(dir, "cancel-ack.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("foreign request produced a cancellation acknowledgement")
	}
}
