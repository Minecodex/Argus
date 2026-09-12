package argusdev

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

func (a *App) checkGuacd(ctx context.Context) (returnErr error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("%w: docker is required for the guacd integration check", errCapability)
	}
	name := kubernetesNameForDev("argus-guacd-check-" + fmt.Sprint(time.Now().UnixNano()))
	if err := a.runner.Run(ctx, nil, "docker", "run", "--detach", "--rm", "--name", name,
		"--publish", "127.0.0.1::4822", "guacamole/guacd:1.6.0", "/opt/guacamole/sbin/guacd",
		"-b", "0.0.0.0", "-l", "4822", "-L", "debug", "-f"); err != nil {
		return err
	}
	defer func() {
		returnErr = errorsJoin(returnErr, a.runner.Run(context.Background(), nil, "docker", "rm", "--force", name))
	}()
	var address string
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		value, err := a.runner.Output(ctx, nil, "docker", "port", name, "4822/tcp")
		if err == nil {
			address = strings.TrimSpace(value)
			connection, dialErr := net.DialTimeout("tcp", address, time.Second)
			if dialErr == nil {
				_ = connection.Close()
				break
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	if address == "" {
		return fmt.Errorf("guacd did not publish its integration port")
	}
	if err := a.runner.Run(ctx, map[string]string{"ARGUS_GUACD_INTEGRATION_ADDRESS": address},
		"go", "test", "./internal/remoteaccess", "-run", "TestRealGuacdHandshakeCompatibility", "-count=1"); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(a.stdout, "real guacd 1.6 accepted the Argus RDP handshake and rejected only the intentionally unavailable RDP target")
	return nil
}

func errorsJoin(left, right error) error {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return fmt.Errorf("%v; cleanup: %w", left, right)
}
