// Package workspacefs is used exclusively by the isolated workspace-io role.
// A Root descriptor is the authority; callers cannot pass host filesystem paths.
package workspacefs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

const MaxFileBytes int64 = 100 << 20

var (
	ErrPath     = errors.New("invalid workspace path")
	ErrFile     = errors.New("workspace target is not a regular file")
	ErrSize     = errors.New("workspace file size limit exceeded")
	ErrConflict = errors.New("workspace file already exists or edit is ambiguous")
)

type Files struct {
	root  *os.Root
	Limit int64
}
type Info struct {
	Path     string
	Size     int64
	Hash     string
	Modified time.Time
}

func Open(directory string, limit int64) (*Files, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = MaxFileBytes
	}
	return &Files{root: root, Limit: limit}, nil
}
func (files *Files) Close() error { return files.root.Close() }

func Normalize(name string) (string, error) {
	if strings.HasPrefix(name, "/workspace/") {
		name = strings.TrimPrefix(name, "/workspace/")
	}
	if name == "" || len(name) > 4096 || strings.ContainsAny(name, "\x00\\:") || strings.HasPrefix(name, "/") {
		return "", ErrPath
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", ErrPath
		}
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", ErrPath
	}
	return clean, nil
}

func (files *Files) Read(name string) (*os.File, Info, error) {
	name, err := Normalize(name)
	if err != nil {
		return nil, Info{}, err
	}
	file, err := files.root.OpenFile(name, readFlags, 0)
	if err != nil {
		return nil, Info{}, err
	}
	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		file.Close()
		if err == nil {
			err = ErrFile
		}
		return nil, Info{}, err
	}
	if stat.Size() > files.Limit {
		file.Close()
		return nil, Info{}, ErrSize
	}
	return file, Info{Path: name, Size: stat.Size(), Modified: stat.ModTime()}, nil
}

func (files *Files) AtomicWrite(ctx context.Context, name string, input io.Reader, size int64, replace bool) (Info, error) {
	name, err := Normalize(name)
	if err != nil {
		return Info{}, err
	}
	if size < 0 || size > files.Limit {
		return Info{}, ErrSize
	}
	parentName, base := path.Dir(name), path.Base(name)
	if err := files.root.MkdirAll(parentName, 0700); err != nil {
		return Info{}, err
	}
	parent, err := files.root.OpenRoot(parentName)
	if err != nil {
		return Info{}, err
	}
	defer parent.Close()
	if stat, err := parent.Lstat(base); err == nil {
		if !stat.Mode().IsRegular() {
			return Info{}, ErrFile
		}
		if !replace {
			return Info{}, ErrConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Info{}, err
	}
	temp := ".argus-upload-" + uuid.NewString()
	file, err := parent.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Info{}, err
	}
	defer func() { file.Close(); _ = parent.Remove(temp) }()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(contextReader{ctx: ctx, reader: input}, size+1))
	if err != nil {
		return Info{}, err
	}
	if written != size {
		return Info{}, ErrSize
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	if err := file.Sync(); err != nil {
		return Info{}, err
	}
	if err := file.Close(); err != nil {
		return Info{}, err
	}
	if replace {
		if stat, err := parent.Lstat(base); err == nil && !stat.Mode().IsRegular() {
			return Info{}, ErrFile
		}
		if err := parent.Rename(temp, base); err != nil {
			return Info{}, err
		}
	} else {
		// Link is atomic and does not replace an existing destination, including
		// one created after the earlier existence check.
		if err := parent.Link(temp, base); err != nil {
			return Info{}, err
		}
		if err := parent.Remove(temp); err != nil {
			return Info{}, err
		}
	}
	if err := syncDirectory(parent); err != nil {
		return Info{}, err
	}
	return Info{Path: name, Size: size, Hash: hex.EncodeToString(hash.Sum(nil)), Modified: time.Now().UTC()}, nil
}

func (files *Files) Edit(ctx context.Context, name, old, new string) (Info, error) {
	file, info, err := files.Read(name)
	if err != nil {
		return Info{}, err
	}
	defer file.Close()
	if info.Size > files.Limit || info.Size < 0 {
		return Info{}, ErrSize
	}
	// One bounded input buffer; the replacement streams from slices without
	// duplicating the original file or allocating the final file in memory.
	data := make([]byte, info.Size)
	if _, err := io.ReadFull(contextReader{ctx: ctx, reader: file}, data); err != nil {
		return Info{}, err
	}
	needle := []byte(old)
	if old == "" || bytes.Count(data, needle) != 1 {
		return Info{}, ErrConflict
	}
	index := bytes.Index(data, needle)
	value := io.MultiReader(bytes.NewReader(data[:index]), strings.NewReader(new), bytes.NewReader(data[index+len(old):]))
	return files.AtomicWrite(ctx, info.Path, value, info.Size-int64(len(old))+int64(len(new)), true)
}

func (files *Files) Delete(name string) error {
	name, err := Normalize(name)
	if err != nil {
		return err
	}
	stat, err := files.root.Lstat(name)
	if err != nil {
		return err
	}
	if !stat.Mode().IsRegular() {
		return ErrFile
	}
	return files.root.Remove(name)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader contextReader) Read(p []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(p)
}
