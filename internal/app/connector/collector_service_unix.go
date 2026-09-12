//go:build !windows

package connector

import (
	"context"
	"errors"
)

func runCollectorService(context.Context, []string) error {
	return errors.New("collector-service is only available on Windows")
}
