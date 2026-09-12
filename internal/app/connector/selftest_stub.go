//go:build !windows

package connector

import (
	"context"
	"errors"
)

func platformSelfTest(context.Context) error {
	return errors.New("platform self-test is currently available only on Windows")
}
