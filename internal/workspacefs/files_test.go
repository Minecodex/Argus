package workspacefs

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAtomicWriteKeepsOldFileOnFailure(t *testing.T) {
	directory := t.TempDir()
	files, err := Open(directory, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	if _, err := files.AtomicWrite(context.Background(), "dir/data.txt", strings.NewReader("original"), 8, false); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		data string
		size int64
	}{{"too much", 3}, {"short", 15}} {
		if _, err := files.AtomicWrite(context.Background(), "dir/data.txt", strings.NewReader(input.data), input.size, true); err == nil {
			t.Fatal("size mismatch accepted")
		}
		data, _ := os.ReadFile(filepath.Join(directory, "dir", "data.txt"))
		if string(data) != "original" {
			t.Fatal("failed upload destroyed old file")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := files.AtomicWrite(ctx, "dir/data.txt", strings.NewReader("new"), 3, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled write: %v", err)
	}
	if _, err := files.Edit(context.Background(), "dir/data.txt", "original", "replacement"); err != nil {
		t.Fatal(err)
	}
	file, info, err := files.Read("/workspace/dir/data.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, _ := io.ReadAll(file)
	if string(data) != "replacement" || info.Size != 11 {
		t.Fatalf("bad edit: %q %#v", data, info)
	}
}

func TestPathEscapeAndAmbiguousEditRejected(t *testing.T) {
	directory := t.TempDir()
	files, err := Open(directory, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	for _, name := range []string{"../outside", "/etc/passwd", "C:/Windows/file", "a/../../b", "a\\b", "."} {
		if _, err := files.AtomicWrite(context.Background(), name, strings.NewReader("x"), 1, false); err == nil {
			t.Fatalf("path accepted: %s", name)
		}
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(directory, "escape")); err == nil {
		if _, err := files.AtomicWrite(context.Background(), "escape/file", strings.NewReader("x"), 1, false); err == nil {
			t.Fatal("symlink escaped root")
		}
	}
	if _, err := files.AtomicWrite(context.Background(), "same", strings.NewReader("aa"), 2, false); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Edit(context.Background(), "same", "a", "b"); !errors.Is(err, ErrConflict) {
		t.Fatalf("ambiguous edit accepted: %v", err)
	}
}
