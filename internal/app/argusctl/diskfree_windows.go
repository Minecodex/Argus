//go:build windows

package argusctl

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func hostDiskFreeBytes(path string) (uint64, error) {
	// GetDiskFreeSpaceEx 根路径不能带尾部分隔符（如 "D:\"），否则返回 ERROR_INVALID_NAME。
	volume := filepath.VolumeName(path)
	if volume != "" && (volume[len(volume)-1] == '\\' || volume[len(volume)-1] == '/') {
		path = volume[:len(volume)-1]
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}
