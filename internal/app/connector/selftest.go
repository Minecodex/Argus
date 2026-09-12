package connector

import (
	"context"
	"errors"
)

func runSelfTest(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return errors.New("argus-connector self-test takes no arguments")
	}
	return platformSelfTest(ctx)
}
