//go:build !linux

package workspacefs

import (
	"errors"
	"os"
)

func IsQuotaError(err error) bool { return errors.Is(err, ErrSize) }

const readFlags = os.O_RDONLY

func syncDirectory(*os.Root) error { return nil }
