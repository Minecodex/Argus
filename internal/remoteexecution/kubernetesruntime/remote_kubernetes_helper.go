package kubernetesruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func (b *kubernetesRuntimeBridge) installHelper(ctx context.Context, binding *connectorv1.RemoteExecutionBatch) (string, error) {
	var observed boundedRemoteBuffer
	if err := b.exec(ctx, []string{"sh", "-c", "command -v bash >/dev/null && command -v sha256sum >/dev/null && uname -m"}, nil, &observed, io.Discard); err != nil {
		return "", errors.New("REMOTE_POD_DEPENDENCY_UNAVAILABLE")
	}
	arch := strings.TrimSpace(observed.String())
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	default:
		return "", errors.New("REMOTE_PLATFORM_UNAVAILABLE")
	}
	directory := b.options.HelperDirectory
	if directory == "" {
		directory = "/usr/local/lib/argus/remote-runtime"
	}
	file, err := os.Open(filepath.Join(directory, "linux-"+arch))
	if err != nil {
		return "", errors.New("REMOTE_POD_DEPENDENCY_UNAVAILABLE")
	}
	defer file.Close()
	binary, err := io.ReadAll(io.LimitReader(file, 32<<20+1))
	if err != nil || len(binary) == 0 || len(binary) > 32<<20 {
		return "", errors.New("REMOTE_POD_DEPENDENCY_UNAVAILABLE")
	}
	hash := sha256.Sum256(binary)
	digest := hex.EncodeToString(hash[:])
	// Identifiers were UUID-validated before reaching this fixed bootstrap. A
	// pre-existing generation is evidence of context loss, never a restart cue.
	root := "/tmp/argus-runtime/" + binding.SessionId + "/" + binding.SessionGeneration
	if b.options.FilesOnly {
		command := "set -eu; test -d " + root + "; test ! -L /tmp/argus-runtime; test ! -L /tmp/argus-runtime/" + binding.SessionId + "; test ! -L " + root + "; test ! -L " + root + "/runtime; printf '%s  %s\\n' " + digest + " " + root + "/runtime | sha256sum -c - >/dev/null"
		if err = b.exec(ctx, []string{"sh", "-c", command}, nil, io.Discard, io.Discard); err != nil {
			return "", errors.New("REMOTE_DIRECTORY_OWNERSHIP_UNKNOWN")
		}
		b.helperDirectory = root
		return root + "/runtime", nil
	}
	command := "set -eu; umask 077; mkdir -p /tmp/argus-runtime/" + binding.SessionId + "; mkdir " + root + "; cat > " + root + "/runtime.partial; printf '%s  %s\\n' " + digest + " " + root + "/runtime.partial | sha256sum -c - >/dev/null; chmod 700 " + root + "/runtime.partial; mv " + root + "/runtime.partial " + root + "/runtime"
	if err = b.exec(ctx, []string{"sh", "-c", command}, bytes.NewReader(binary), io.Discard, io.Discard); err != nil {
		return "", errors.New("REMOTE_CONTEXT_LOST")
	}
	b.helperDirectory = root
	return root + "/runtime", nil
}

func (b *kubernetesRuntimeBridge) removeHelper(ctx context.Context) error {
	root := b.helperDirectory
	parent := root[:strings.LastIndex(root, "/")]
	// These UUID-only paths were created by installHelper. Remove only the known
	// executable and then empty directories; never recursively delete Pod data.
	command := "set -eu; test ! -L /tmp/argus-runtime; test ! -L " + parent + "; test ! -L " + root + "; test ! -L " + root + "/runtime; rm -f -- " + root + "/runtime; rmdir -- " + root + "; rmdir -- " + parent + " 2>/dev/null || true"
	if err := b.exec(ctx, []string{"sh", "-c", command}, nil, io.Discard, io.Discard); err != nil {
		return errors.New("REMOTE_DIRECTORY_OWNERSHIP_UNKNOWN")
	}
	return nil
}

type boundedRemoteBuffer struct{ bytes.Buffer }

func (b *boundedRemoteBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 64<<10 {
		return 0, errors.New("REMOTE_OUTPUT_LIMIT")
	}
	return b.Buffer.Write(p)
}
