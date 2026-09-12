package remoteaccess

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRealGuacdHandshakeCompatibility(t *testing.T) {
	address := strings.TrimSpace(os.Getenv("ARGUS_GUACD_INTEGRATION_ADDRESS"))
	if address == "" {
		t.Skip("ARGUS_GUACD_INTEGRATION_ADDRESS is not configured")
	}
	connection, ready, err := connectGuacd(t.Context(), address, "127.0.0.1:1", "Administrator", []byte("integration-password"), 1024, 768)
	if err != nil {
		t.Fatalf("real guacd rejected the Argus handshake: %v", err)
	}
	defer connection.Close()
	if !bytes.Contains(ready, []byte(".ready")) {
		t.Fatalf("real guacd omitted its ready instruction: %q", ready)
	}
	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(connection)
	terminated := false
	for range 64 {
		_, opcode, _, readErr := readGuacamoleInstruction(reader)
		if readErr != nil {
			t.Fatalf("real guacd did not report the unavailable RDP target: %v", readErr)
		}
		if opcode == "error" || opcode == "disconnect" {
			terminated = true
			break
		}
	}
	if !terminated {
		t.Fatal("real guacd did not terminate the intentionally unavailable RDP target")
	}
}

func TestGuacamoleInstructionRoundTripAndChannelDenylist(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	go func() { _ = writeGuacamoleInstruction(client, "key", "65", "1", "中文") }()
	raw, opcode, values, err := readGuacamoleInstruction(bufio.NewReader(server))
	if err != nil || opcode != "key" || !slices.Equal(values, []string{"65", "1", "中文"}) || len(raw) == 0 {
		t.Fatalf("Guacamole instruction round trip failed: opcode=%q values=%v err=%v", opcode, values, err)
	}
	for _, opcode := range []string{"clipboard", "file", "pipe", "blob", "ack"} {
		if !prohibitedGuacamoleOpcode(opcode) {
			t.Fatalf("RDP channel %q was not denied", opcode)
		}
	}
	if prohibitedGuacamoleOpcode("mouse") || prohibitedGuacamoleOpcode("key") || prohibitedGuacamoleOpcode("size") {
		t.Fatal("RDP input channel was denied")
	}
}

func TestConnectGuacdKeepsCredentialOutOfBrowserReadyFrame(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	password := []byte("short-lived-rdp-password")
	observed := make(chan []string, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		_, _, _, _ = readGuacamoleInstruction(reader)
		_ = writeGuacamoleInstruction(connection, "args", "hostname", "port", "username", "password", "security", "disable-copy", "disable-paste", "enable-drive")
		for index := 0; index < 5; index++ {
			_, _, _, _ = readGuacamoleInstruction(reader)
		}
		_, opcode, values, _ := readGuacamoleInstruction(reader)
		if opcode == "connect" {
			observed <- values
		}
		_ = writeGuacamoleInstruction(connection, "ready", "connection-id")
	}()
	connection, ready, err := connectGuacd(context.Background(), listener.Addr().String(), "127.0.0.1:33999", "Administrator", password, 1000, 500)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	values := <-observed
	if !slices.Contains(values, string(password)) {
		t.Fatal("guacd did not receive the leased RDP credential")
	}
	if bytes.Contains(ready, password) {
		t.Fatal("RDP credential leaked into the browser-ready instruction")
	}
}
