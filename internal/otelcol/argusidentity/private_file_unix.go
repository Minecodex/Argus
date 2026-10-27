//go:build !windows

package argusidentity

import (
	"fmt"
	"os"
)

func protectPrivateFile(path string) error { return os.Chmod(path, 0600) }
func verifyPrivateFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0600 {
		return fmt.Errorf("identity file permissions are not private")
	}
	return nil
}
