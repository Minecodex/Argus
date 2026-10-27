package collectormanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimeLockFencesIndependentManagers(t *testing.T) {
	root := t.TempDir()
	release, err := lockRuntime(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if unlock, err := lockRuntime(ctx, root); !errors.Is(err, context.DeadlineExceeded) {
		if unlock != nil {
			unlock()
		}
		release()
		t.Fatalf("concurrent runtime mutation admitted: %v", err)
	}
	if err = clearDirectory(root, 0700, runtimeLockFile); err != nil {
		release()
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, runtimeLockFile)); err != nil {
		release()
		t.Fatal("uninstall unlinked the held lock")
	}
	release()
	release, err = lockRuntime(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestRuntimeRollbackRestoresOnlySnapshotFiles(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "private.json")
	newFile := filepath.Join(root, "new.json")
	other := filepath.Join(root, "unrelated.txt")
	for path, value := range map[string]string{existing: "old private material", other: "preserve"} {
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := saveRuntimeFiles(existing, newFile)
	if err != nil {
		t.Fatal(err)
	}
	defer clearRuntimeFiles(snapshot)
	if err = writeAtomic(existing, []byte("candidate private material"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = writeAtomic(newFile, []byte("candidate"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = restoreRuntimeFiles(snapshot); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(existing)
	if err != nil || string(raw) != "old private material" {
		t.Fatalf("previous private configuration lost: %v", err)
	}
	if _, err = os.Stat(newFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed candidate file retained")
	}
	raw, err = os.ReadFile(other)
	if err != nil || string(raw) != "preserve" {
		t.Fatal("rollback changed unrelated file")
	}
	copy := filepath.Join(root, "copy")
	if err = copyRuntimeFile(existing, copy, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(copy)
	if err != nil || string(raw) != "old private material" {
		t.Fatal("artifact copy was not complete")
	}
}
