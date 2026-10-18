package runner

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func openRegular(root *os.Root, name string, flags int) (*os.File, error) {
	f, err := root.OpenFile(name, flags, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	var handle windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &handle)
	if err != nil || !info.Mode().IsRegular() || handle.NumberOfLinks != 1 || handle.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		f.Close()
		return nil, errors.New("REMOTE_PATH_INVALID")
	}
	return f, nil
}
