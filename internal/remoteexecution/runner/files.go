package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func safeRelative(p string) bool {
	return p != "" && len(p) < 4096 && !strings.ContainsAny(p, "\\:\x00") && !path.IsAbs(p) && path.Clean(p) == p && p != "." && p != ".." && !strings.HasPrefix(p, "../")
}

// All I/O is relative to pinned roots. Checks also reject links inside the root;
// a concurrent replacement can at worst affect this same exchange directory.
func rootDirectory(root *os.Root, relative string) error {
	if relative == "." {
		return nil
	}
	if !safeRelative(filepath.ToSlash(relative)) {
		return errors.New("REMOTE_PATH_INVALID")
	}
	current := ""
	for _, segment := range strings.Split(filepath.ToSlash(relative), "/") {
		current = path.Join(current, segment)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err = root.Mkdir(current, 0700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || isLinkOrReparse(filepath.Join(root.Name(), current), info) {
			return errors.New("REMOTE_PATH_INVALID")
		}
	}
	return nil
}
func rootFile(root *os.Root, relative string) error {
	if !safeRelative(relative) {
		return errors.New("REMOTE_PATH_INVALID")
	}
	current := ""
	parts := strings.Split(relative, "/")
	for i, segment := range parts {
		current = path.Join(current, segment)
		info, err := root.Lstat(current)
		if i == len(parts)-1 && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if isLinkOrReparse(filepath.Join(root.Name(), current), info) {
			return errors.New("REMOTE_PATH_INVALID")
		}
		if i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
			return errors.New("REMOTE_PATH_INVALID")
		}
	}
	return nil
}
func (m *Manager) File(ctx context.Context, r *connectorv1.RemoteExecutionFile) (*connectorv1.RemoteExecutionFileResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s, err := m.lookup(r.SessionId, r.SessionGeneration)
	if err != nil {
		return nil, err
	}
	if !validID(r.RequestId) {
		return nil, errors.New("REMOTE_INPUT_INVALID")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil, errors.New("REMOTE_SESSION_BUSY")
	}
	if s.lost && r.Operation != "list_output" && r.Operation != "read_output" {
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	exchange := s.exchanges[r.RequestId]
	if exchange == nil {
		return nil, errors.New("REMOTE_BINDING_INVALID")
	}
	if r.Operation == "list_output" {
		return listOutput(ctx, exchange.output)
	}
	root := exchange.input
	if r.Operation == "read_output" {
		root = exchange.output
	}
	if len(r.Data) > 64<<10 || r.ByteSize > 64<<20 || r.Offset > r.ByteSize {
		return nil, errors.New("REMOTE_FILE_LIMIT")
	}
	if !safeRelative(r.Path) {
		return nil, errors.New("REMOTE_PATH_INVALID")
	}
	if r.Operation == "write_input" || r.Operation == "finish_input" {
		if err = rootDirectory(root, path.Dir(r.Path)); err != nil {
			return nil, err
		}
	}
	if err = rootFile(root, r.Path); err != nil {
		return nil, err
	}
	switch r.Operation {
	case "write_input":
		partial := r.Path + ".argus-partial"
		if err = rootFile(root, partial); err != nil {
			return nil, err
		}
		f, err := openRegular(root, partial, os.O_CREATE|os.O_WRONLY)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if uint64(info.Size()) < r.Offset {
			return nil, errors.New("REMOTE_TRANSFER_OFFSET")
		}
		if r.Offset+uint64(len(r.Data)) > r.ByteSize {
			return nil, errors.New("REMOTE_FILE_LIMIT")
		}
		if _, err = f.WriteAt(r.Data, int64(r.Offset)); err != nil {
			return nil, err
		}
		return &connectorv1.RemoteExecutionFileResult{Offset: r.Offset + uint64(len(r.Data))}, f.Sync()
	case "finish_input":
		partial := r.Path + ".argus-partial"
		digest, size, err := fileDigest(root, partial)
		hasPartial := !errors.Is(err, os.ErrNotExist)
		if !hasPartial {
			digest, size, err = fileDigest(root, r.Path)
		}
		if err != nil || uint64(size) != r.ByteSize || digest != r.Sha256 {
			return nil, errors.New("REMOTE_FILE_INTEGRITY")
		}
		if hasPartial {
			if _, err = root.Lstat(r.Path); err == nil {
				existingDigest, existingSize, e := fileDigest(root, r.Path)
				if e != nil || existingDigest != r.Sha256 || uint64(existingSize) != r.ByteSize {
					return nil, errors.New("REMOTE_FILE_CONFLICT")
				}
				if e = root.Remove(partial); e != nil {
					return nil, e
				}
				return &connectorv1.RemoteExecutionFileResult{Complete: true, Offset: r.ByteSize}, nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
			if err = root.Rename(partial, r.Path); err != nil {
				return nil, err
			}
		}
		return &connectorv1.RemoteExecutionFileResult{Complete: true, Offset: uint64(size)}, nil
	case "read_output":
		f, err := openRegular(root, r.Path, os.O_RDONLY)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || uint64(info.Size()) != r.ByteSize {
			return nil, errors.New("REMOTE_FILE_CHANGED")
		}
		data := make([]byte, min(uint64(64<<10), r.ByteSize-r.Offset))
		n, err := f.ReadAt(data, int64(r.Offset))
		if err != nil && err != io.EOF {
			return nil, err
		}
		return &connectorv1.RemoteExecutionFileResult{Data: data[:n], Offset: r.Offset + uint64(n), Complete: r.Offset+uint64(n) == r.ByteSize}, nil
	default:
		return nil, errors.New("REMOTE_INPUT_INVALID")
	}
}
func fileDigest(root *os.Root, relative string) (string, int64, error) {
	if err := rootFile(root, relative); err != nil {
		return "", 0, err
	}
	f, err := openRegular(root, relative, os.O_RDONLY)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, 64<<20+1))
	if err != nil {
		return "", 0, err
	}
	after, err := f.Stat()
	if err != nil || n > 64<<20 || n != before.Size() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", 0, errors.New("REMOTE_FILE_CHANGED")
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
func listOutput(ctx context.Context, root *os.Root) (*connectorv1.RemoteExecutionFileResult, error) {
	result := &connectorv1.RemoteExecutionFileResult{Complete: true}
	var total int64
	err := fs.WalkDir(root.FS(), ".", func(relative string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if isLinkOrReparse(filepath.Join(root.Name(), relative), info) {
			return errors.New("REMOTE_PATH_INVALID")
		}
		if d.IsDir() {
			return nil
		}
		digest, size, e := fileDigest(root, relative)
		if e != nil {
			return e
		}
		total += size
		if len(result.Files) >= 256 || total > 64<<20 {
			return errors.New("REMOTE_FILE_LIMIT")
		}
		result.Files = append(result.Files, &connectorv1.RemoteExecutionFileInfo{Path: relative, ByteSize: uint64(size), Sha256: digest})
		return nil
	})
	return result, err
}
