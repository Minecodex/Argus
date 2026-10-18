package workspacefs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// List stays beneath the open Root even if a concurrent process renames a
// directory. Links are rejected rather than followed during automatic export.
func (f *Files) List(ctx context.Context, directory string, maxFiles int, maxBytes int64) ([]Info, error) {
	name, err := Normalize(directory)
	if err != nil {
		return nil, err
	}
	info, err := f.root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return []Info{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrPath
	}
	if maxFiles < 1 || maxFiles > 256 || maxBytes < 0 || maxBytes > 64<<20 {
		return nil, ErrSize
	}
	files := []Info{}
	var bytes int64
	visited := 0
	err = fs.WalkDir(f.root.FS(), name, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > 4096 || strings.Count(p, "/") > 32 {
			return ErrSize
		}
		info, e := f.root.Lstat(p)
		if e != nil {
			return e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrPath
		}
		if d.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return ErrFile
		}
		bytes += info.Size()
		if len(files) >= maxFiles || bytes > maxBytes {
			return ErrSize
		}
		files = append(files, Info{Path: p, Size: info.Size(), Modified: info.ModTime()})
		return nil
	})
	return files, err
}
