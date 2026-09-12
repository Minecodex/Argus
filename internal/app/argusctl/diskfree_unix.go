//go:build !windows

package argusctl

import (
	"golang.org/x/sys/unix"
)

func hostDiskFreeBytes(path string) (uint64, error) {
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return 0, err
	}
	return uint64(fs.Bavail) * uint64(fs.Bsize), nil
}
