package collectormanager

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

type savedRuntimeFile struct {
	path   string
	value  []byte
	mode   os.FileMode
	exists bool
}

func saveRuntimeFiles(paths ...string) ([]savedRuntimeFile, error) {
	result := []savedRuntimeFile{}
	for _, path := range paths {
		item := savedRuntimeFile{path: path}
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			result = append(result, item)
			continue
		}
		if err != nil {
			clearRuntimeFiles(result)
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 4<<20 {
			clearRuntimeFiles(result)
			return nil, ErrInvalidCommand
		}
		item.value, err = os.ReadFile(path)
		if err != nil {
			clearRuntimeFiles(result)
			return nil, err
		}
		item.mode, item.exists = info.Mode().Perm(), true
		result = append(result, item)
	}
	return result, nil
}

func restoreRuntimeFiles(files []savedRuntimeFile) error {
	var failures []error
	for _, file := range files {
		var err error
		if file.exists {
			err = writeAtomic(file.path, file.value, file.mode)
		} else {
			err = os.Remove(file.path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func clearRuntimeFiles(files []savedRuntimeFile) {
	for _, file := range files {
		clear(file.value)
	}
}

// Files can reside on different bind mounts of the hardened helper. A streamed
// copy followed by a same-directory rename also works across those boundaries.
func copyRuntimeFile(source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".argus-collector-")
	if err != nil {
		return err
	}
	temporary := output.Name()
	defer os.Remove(temporary)
	if err = output.Chmod(mode); err == nil {
		var size int64
		size, err = io.Copy(output, io.LimitReader(input, MaxArtifactBytes+1))
		if size > MaxArtifactBytes {
			err = ErrArtifactInvalid
		}
	}
	if err == nil {
		err = output.Sync()
	}
	closeErr := output.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary, destination)
}
