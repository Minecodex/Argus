package workspacefs

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func IsQuotaError(err error) bool { return errors.Is(err, unix.ENOSPC) || errors.Is(err, unix.EDQUOT) }

const readFlags = os.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK

func syncDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
