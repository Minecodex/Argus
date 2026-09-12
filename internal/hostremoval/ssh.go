package hostremoval

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/kakj-go/Argus/internal/sshtarget"
	"golang.org/x/crypto/ssh"
)

// ExecuteSSH uses a separate SSH session, which survives stopping the target
// Connector. Both the Direct Executor and Bastion Connector use this boundary.
func ExecuteSSH(ctx context.Context, client *ssh.Client, plan Plan) (CleanupEvidence, []byte, []byte, error) {
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	session, err := client.NewSession()
	if err != nil {
		return CleanupEvidence{}, nil, nil, err
	}
	defer session.Close()
	script, windows := BuildSSHScript(plan)
	command := "sh -s"
	if windows {
		command = sshtarget.PowerShellCommand(script)
	} else {
		session.Stdin = strings.NewReader(script)
	}
	var stdout, stderr bytes.Buffer
	session.Stdout, session.Stderr = &stdout, &stderr
	if err = session.Run(command); err != nil {
		return CleanupEvidence{}, nil, nil, fmt.Errorf("SSH local removal failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseSSHCleanupOutput(stdout.String(), plan.OperationID.UUID, plan.ConnectorID, plan.RemovalGeneration)
}
