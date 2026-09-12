package connector

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"google.golang.org/protobuf/types/known/anypb"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
)

func TestHostConnectorInstallFailureStageIsSafeAndStable(t *testing.T) {
	if got := hostConnectorInstallFailureStage(hostConnectorInstallFailure("enrollment", errors.New("sensitive remote detail"))); got != "enrollment" {
		t.Fatalf("failure stage = %q", got)
	}
	if got := hostConnectorInstallFailureStage(errors.New("unclassified")); got != "unknown" {
		t.Fatalf("unclassified failure stage = %q", got)
	}
}

func TestExecuteHostProbeSSH(t *testing.T) {
	host, port, fingerprint := startSSHServer(t, "connector-password")

	request := &connectorv1.HostConnectionProbe{
		Address: host, Port: port, Protocol: "ssh", Username: "argus", Platform: "linux",
		ExpectedHostKeyFingerprint: fingerprint,
	}
	payload, err := anypb.New(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeHostProbe(context.Background(), payload, []byte("connector-password"))
	if err != nil {
		t.Fatalf("executeHostProbe() error = %v", err)
	}
	if result.HostKeyFingerprint != fingerprint || result.RemoteVersion == "" || result.Architecture != "arm64" {
		t.Fatalf("unexpected probe result: %+v", result)
	}

	if _, err := executeHostProbe(context.Background(), payload, []byte("wrong-password")); err == nil {
		t.Fatal("executeHostProbe() accepted an invalid password")
	}
	request.ExpectedHostKeyFingerprint = "SHA256:changed"
	payload, err = anypb.New(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executeHostProbe(context.Background(), payload, []byte("connector-password")); err == nil {
		t.Fatal("executeHostProbe() accepted a changed Host key")
	}
}

func startSSHServer(t *testing.T, password string) (string, uint32, string) {
	return startSSHServerWithCallbacks(t, password, nil)
}

func startSSHServerWithCallbacks(t *testing.T, password string, callbacks map[string]string) (string, uint32, string) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	configuration := &ssh.ServerConfig{PasswordCallback: func(metadata ssh.ConnMetadata, value []byte) (*ssh.Permissions, error) {
		if metadata.User() != "argus" || string(value) != password {
			return nil, errors.New("authentication rejected")
		}
		return nil, nil
	}}
	configuration.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go serveSSHTestConnectionWithCallbacks(connection, configuration, callbacks)
		}
	}()
	address := listener.Addr().(*net.TCPAddr)
	return address.IP.String(), uint32(address.Port), ssh.FingerprintSHA256(signer.PublicKey())
}

func serveSSHTestConnection(connection net.Conn, configuration *ssh.ServerConfig) {
	serveSSHTestConnectionWithCallbacks(connection, configuration, nil)
}

func serveSSHTestConnectionWithCallbacks(connection net.Conn, configuration *ssh.ServerConfig, callbacks map[string]string) {
	server, channels, requests, err := ssh.NewServerConn(connection, configuration)
	if err != nil {
		_ = connection.Close()
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for incoming := range channels {
		if incoming.ChannelType() == "direct-tcpip" && callbacks != nil {
			go serveTestCallbackChannel(incoming, callbacks)
			continue
		}
		if incoming.ChannelType() != "session" {
			_ = incoming.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		channel, channelRequests, acceptErr := incoming.Accept()
		if acceptErr != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for request := range channelRequests {
				var payload struct{ Command string }
				if request.Type != "exec" || ssh.Unmarshal(request.Payload, &payload) != nil || !strings.Contains(payload.Command, "architecture=$(uname -m)") {
					_ = request.Reply(false, nil)
					continue
				}
				_ = request.Reply(true, nil)
				_, _ = channel.Write([]byte("architecture=aarch64\ndistribution_version=ubuntu:24.04\nservice_manager=systemd\nprivileged=true\nfree_disk_bytes=1073741824\n"))
				_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
				return
			}
		}()
	}
}

func serveTestCallbackChannel(incoming ssh.NewChannel, callbacks map[string]string) {
	var target struct {
		Host       string
		Port       uint32
		Origin     string
		OriginPort uint32
	}
	if ssh.Unmarshal(incoming.ExtraData(), &target) != nil {
		_ = incoming.Reject(ssh.Prohibited, "invalid target")
		return
	}
	address := callbacks[net.JoinHostPort(target.Host, strconv.Itoa(int(target.Port)))]
	if address == "" {
		_ = incoming.Reject(ssh.ConnectionFailed, "target cannot reach public callbacks")
		return
	}
	upstream, err := net.Dial("tcp", address)
	if err != nil {
		_ = incoming.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	defer upstream.Close()
	channel, requests, err := incoming.Accept()
	if err != nil {
		return
	}
	defer channel.Close()
	go ssh.DiscardRequests(requests)
	go func() { _, _ = io.Copy(upstream, channel); _ = upstream.Close() }()
	_, _ = io.Copy(channel, upstream)
}
