package directexecutor

import (
	"context"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/sshtarget"
	"github.com/kakj-go/Argus/internal/tunnelruntime"
)

// These listeners belong only to this ConnectionTest. No Host, Connector,
// installation token or persistent control-tunnel lease is created.
func (executor *Executor) probeOnboardingCallbacks(ctx context.Context, client *ssh.Client, plan installation.CallbackProbePlan) error {
	if plan.ControlPath != "executor_tunnel" {
		return sshtarget.ProbeOnboarding(ctx, client, plan)
	}
	for _, target := range []string{executor.ConnectorEnrollForwardTarget, executor.ConnectorGatewayForwardTarget} {
		if host, port, err := net.SplitHostPort(target); err != nil || host == "" || port == "" {
			return &sshtarget.CallbackError{Code: "CONFIG_INVALID"}
		}
	}
	probeCtx, cancel := context.WithCancel(ctx)
	var listeners []*callbackPreflightListener
	var serving sync.WaitGroup
	defer func() {
		cancel()
		for _, listener := range listeners {
			_ = listener.Close()
		}
		serving.Wait()
		for _, listener := range listeners {
			listener.connections.Wait()
		}
	}()
	// An SSH global request has no context parameter. Closing this probe's SSH
	// client also releases an in-flight Listen request when the test is canceled.
	stopOnCancel := context.AfterFunc(probeCtx, func() { _ = client.Close() })
	defer stopOnCancel()
	for index, target := range []string{executor.ConnectorEnrollForwardTarget, executor.ConnectorGatewayForwardTarget} {
		remote, err := client.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			if ctx.Err() != nil {
				return &sshtarget.CallbackError{Code: "TIMEOUT"}
			}
			return &sshtarget.CallbackError{Code: "CONNECT_FAILED"}
		}
		listener := &callbackPreflightListener{Listener: remote}
		listeners = append(listeners, listener)
		if index == 0 {
			plan.EnrollDialAddress = listener.Addr().String()
		} else {
			plan.GatewayDialAddress = listener.Addr().String()
		}
		relay := tunnelruntime.Relay{Target: target, Kind: "control", Dialer: net.Dialer{Timeout: 5 * time.Second}}
		serving.Add(1)
		go func() { defer serving.Done(); _ = relay.Serve(probeCtx, listener) }()
	}
	return sshtarget.ProbeOnboarding(probeCtx, client, plan)
}

// Relay closes its upstream before its accepted inbound. Waiting for these
// closes after Serve exits ensures no upstream or remote-forward connection
// survives a completed or canceled preflight.
type callbackPreflightListener struct {
	net.Listener
	connections sync.WaitGroup
}

func (listener *callbackPreflightListener) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	listener.connections.Add(1)
	return &callbackPreflightConnection{Conn: connection, release: listener.connections.Done}, nil
}

type callbackPreflightConnection struct {
	net.Conn
	release func()
	once    sync.Once
}

func (connection *callbackPreflightConnection) Close() error {
	err := connection.Conn.Close()
	connection.once.Do(connection.release)
	return err
}
