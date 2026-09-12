package remoteaccess

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
)

const maxGuacamoleInstructionBytes = 64 << 10

type rdpBridge struct {
	listener net.Listener
	backend  BackendSession
	done     chan error
}

type guacamoleFrame struct {
	raw    []byte
	opcode string
	values []string
	err    error
}

func (gateway WebSocketGateway) serveRDP(ctx context.Context, connection *websocket.Conn, sessionID uuid.UUID, target ConnectionTarget, hello ClientFrame) {
	defer clear(target.CredentialPayload)
	if gateway.GuacdAddress == "" || target.Platform != "windows" || len(target.CredentialPayload) == 0 {
		_ = gateway.Service.Finish(context.Background(), sessionID, target.Session.SessionFence, "failed", "RDP_CONFIGURATION_INVALID")
		closeProtocol(connection, "REMOTE_ACCESS_CONNECTION_LOST")
		return
	}
	sessionCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend, err := gateway.Backends.Open(sessionCtx, target, hello.Cols, hello.Rows)
	if err != nil {
		gateway.logFailure("rdp_backend", err)
		_ = gateway.Service.Finish(context.Background(), sessionID, target.Session.SessionFence, "failed", "RDP_NOT_AVAILABLE")
		closeProtocol(connection, "REMOTE_ACCESS_CONNECTION_LOST")
		return
	}
	defer backend.Close(context.Background(), "rdp_session_closed")
	bridge, err := startRDPBridge(sessionCtx, backend)
	if err != nil {
		_ = gateway.Service.Finish(context.Background(), sessionID, target.Session.SessionFence, "failed", "RDP_BRIDGE_FAILED")
		closeProtocol(connection, "REMOTE_ACCESS_CONNECTION_LOST")
		return
	}
	defer bridge.Close()
	guacd, ready, err := connectGuacd(ctx, gateway.GuacdAddress, bridge.Address(), target.Username, target.CredentialPayload, hello.Cols, hello.Rows)
	clear(target.CredentialPayload)
	if err != nil {
		gateway.logFailure("guacd_handshake", err)
		_ = gateway.Service.Finish(context.Background(), sessionID, target.Session.SessionFence, "failed", "RDP_GUACD_FAILED")
		closeProtocol(connection, "REMOTE_ACCESS_CONNECTION_LOST")
		return
	}
	defer guacd.Close()
	if err = gateway.Service.MarkActive(ctx, sessionID, target.Session.SessionFence); err != nil {
		closeProtocol(connection, "REMOTE_ACCESS_CONNECTION_LOST")
		return
	}
	var recording *GatewayRecording
	if target.Session.RecordingMode != "disabled" {
		opened, openErr := gateway.Service.OpenRecording(ctx, sessionID, gateway.ObjectStore)
		if openErr != nil && target.Session.RecordingMode != "optional" {
			_ = gateway.Service.Finish(context.Background(), sessionID, target.Session.SessionFence, "failed", "REMOTE_ACCESS_RECORDING_UNAVAILABLE")
			closeProtocol(connection, "REMOTE_ACCESS_RECORDING_UNAVAILABLE")
			return
		}
		if openErr == nil {
			recording = &opened
		}
	}
	writer := &websocketWriter{connection: connection}
	if err = writer.write(ctx, serverFrame{Type: "server_ready", SessionID: sessionID.String(), Mode: "rdp_guacamole", Nonce: hello.Nonce,
		IdleTimeout: int64(target.IdleTimeout / time.Second), MaxDuration: int64(target.MaxDuration / time.Second)}); err != nil {
		gateway.finishRecording(context.Background(), recording, "incomplete")
		_ = gateway.Service.Finish(context.Background(), sessionID, target.Session.SessionFence, "connection_lost", "CLIENT_DISCONNECTED")
		return
	}
	if err = connection.Write(ctx, websocket.MessageText, ready); err != nil {
		gateway.finishRecording(context.Background(), recording, "incomplete")
		_ = gateway.Service.Finish(context.Background(), sessionID, target.Session.SessionFence, "connection_lost", "CLIENT_DISCONNECTED")
		return
	}
	browserFrames := make(chan receivedClientFrame, 1)
	guacdFrames := make(chan guacamoleFrame, 1)
	go receiveWebSocketFrames(ctx, connection, browserFrames)
	go receiveGuacamoleFrames(sessionCtx, guacd, guacdFrames)
	terminationEvents, unregister := gateway.Terminations.Register(sessionID, target.Session.SessionFence)
	defer unregister()
	status, reason := "terminated", "remote_closed"
	started, lastActivity, lastAuthorizationCheck := gateway.now(), gateway.now(), gateway.now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			status, reason = "connection_lost", "CLIENT_DISCONNECTED"
			goto finished
		case <-gateway.Drain:
			status, reason = "terminated", "gateway_drain"
			goto finished
		case termination := <-terminationEvents:
			status, reason = "invalidated", termination.Reason
			goto finished
		case bridgeErr := <-bridge.done:
			if bridgeErr != nil && !errors.Is(bridgeErr, io.EOF) {
				status, reason = "connection_lost", "RDP_BRIDGE_FAILED"
			}
			goto finished
		case <-ticker.C:
			now := gateway.now()
			if recording != nil {
				chunks, flushErr := recording.Recorder.FlushDue(ctx)
				if flushErr == nil {
					flushErr = gateway.Service.PersistChunks(ctx, *recording, chunks)
				}
				if flushErr != nil && target.Session.RecordingMode != "optional" {
					status, reason = "failed", "REMOTE_ACCESS_RECORDING_UNAVAILABLE"
					goto finished
				}
			}
			if now.Sub(lastAuthorizationCheck) >= 30*time.Second {
				if gateway.Service.CheckActive(ctx, sessionID, target.Session.SessionFence, target.Session.AuthorizationVersion) != nil {
					status, reason = "invalidated", "authorization_revoked"
					goto finished
				}
				lastAuthorizationCheck = now
			}
			if idleTimeoutReached(now, lastActivity, target.IdleTimeout) {
				status, reason = "expired", "idle_timeout"
				goto finished
			}
			if now.Sub(started) >= target.MaxDuration || !now.Before(target.LeaseExpiresAt) {
				status, reason = "expired", "maximum_duration"
				goto finished
			}
		case item := <-browserFrames:
			if item.err != nil || item.messageType != websocket.MessageText {
				status, reason = "connection_lost", "CLIENT_DISCONNECTED"
				goto finished
			}
			opcode, _, parseErr := parseGuacamoleInstruction(item.raw)
			if parseErr != nil || prohibitedGuacamoleOpcode(opcode) {
				status, reason = "failed", "RDP_CHANNEL_DENIED"
				goto finished
			}
			lastActivity = gateway.now()
			if err = writeAll(guacd, item.raw); err != nil {
				status, reason = "connection_lost", "RDP_GUACD_FAILED"
				goto finished
			}
			if err = gateway.recordGuacamole(ctx, recording, "i", item.raw); err != nil && target.Session.RecordingMode != "optional" {
				status, reason = "failed", "REMOTE_ACCESS_RECORDING_UNAVAILABLE"
				goto finished
			}
		case item := <-guacdFrames:
			if item.err != nil {
				if !errors.Is(item.err, io.EOF) {
					status, reason = "connection_lost", "RDP_GUACD_FAILED"
				}
				goto finished
			}
			if prohibitedGuacamoleOpcode(item.opcode) {
				continue
			}
			if err = connection.Write(ctx, websocket.MessageText, item.raw); err != nil {
				status, reason = "connection_lost", "CLIENT_DISCONNECTED"
				goto finished
			}
			if err = gateway.recordGuacamole(ctx, recording, "o", item.raw); err != nil && target.Session.RecordingMode != "optional" {
				status, reason = "failed", "REMOTE_ACCESS_RECORDING_UNAVAILABLE"
				goto finished
			}
		}
	}

finished:
	_ = connection.Close(websocket.StatusNormalClosure, reason)
	recordingStatus := "available"
	if status == "connection_lost" || status == "invalidated" {
		recordingStatus = "incomplete"
	} else if status == "failed" {
		recordingStatus = "failed"
	}
	gateway.finishRecording(context.Background(), recording, recordingStatus)
	_ = gateway.Service.FinishFenceTolerant(context.Background(), sessionID, target.Session.SessionFence, status, reason)
}

func startRDPBridge(ctx context.Context, backend BackendSession) (*rdpBridge, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	bridge := &rdpBridge{listener: listener, backend: backend, done: make(chan error, 1)}
	go bridge.run(ctx)
	return bridge, nil
}

func (bridge *rdpBridge) Address() string { return bridge.listener.Addr().String() }
func (bridge *rdpBridge) Close()          { _ = bridge.listener.Close() }

func (bridge *rdpBridge) run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = bridge.listener.Close()
	}()
	connection, err := bridge.listener.Accept()
	if err != nil {
		bridge.done <- err
		return
	}
	defer connection.Close()
	errorsChannel := make(chan error, 2)
	go func() {
		buffer := make([]byte, 32<<10)
		for {
			count, readErr := connection.Read(buffer)
			if count > 0 {
				readErr = bridge.backend.Send(ctx, ClientFrame{Type: "input", Data: string(buffer[:count])})
			}
			if readErr != nil {
				errorsChannel <- readErr
				return
			}
		}
	}()
	go func() {
		for {
			frame, receiveErr := bridge.backend.Receive(ctx)
			if receiveErr != nil {
				errorsChannel <- receiveErr
				return
			}
			if frame.Type == "output" {
				if writeErr := writeAll(connection, frame.Data); writeErr != nil {
					errorsChannel <- writeErr
					return
				}
			} else if frame.Type == "state" && terminalState(frame.Status) {
				errorsChannel <- io.EOF
				return
			}
		}
	}()
	select {
	case err = <-errorsChannel:
	case <-ctx.Done():
		err = ctx.Err()
	}
	bridge.done <- err
}

func connectGuacd(ctx context.Context, guacdAddress, bridgeAddress, username string, password []byte, cols, rows int) (net.Conn, []byte, error) {
	dialer := net.Dialer{Timeout: 10 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", guacdAddress)
	if err != nil {
		return nil, nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = connection.Close()
		}
	}()
	_ = connection.SetDeadline(time.Now().Add(15 * time.Second))
	if err = writeGuacamoleInstruction(connection, "select", "rdp"); err != nil {
		return nil, nil, err
	}
	reader := bufio.NewReaderSize(connection, maxGuacamoleInstructionBytes)
	raw, opcode, arguments, err := readGuacamoleInstruction(reader)
	_ = raw
	if err != nil || opcode != "args" {
		return nil, nil, errors.New("guacd did not return RDP arguments")
	}
	host, port, err := net.SplitHostPort(bridgeAddress)
	if err != nil {
		return nil, nil, err
	}
	for _, instruction := range [][]string{{"size", strconv.Itoa(cols), strconv.Itoa(rows), "96"}, {"audio"}, {"video"}, {"image", "image/png", "image/jpeg", "image/webp"}, {"timezone", "UTC"}} {
		if err = writeGuacamoleInstruction(connection, instruction[0], instruction[1:]...); err != nil {
			return nil, nil, err
		}
	}
	values := make([]string, len(arguments))
	for index, argument := range arguments {
		switch argument {
		case "hostname":
			values[index] = host
		case "port":
			values[index] = port
		case "username":
			values[index] = username
		case "password":
			values[index] = string(password)
		case "security":
			values[index] = "nla"
		case "ignore-cert":
			values[index] = "true"
		case "disable-copy", "disable-paste", "disable-audio":
			values[index] = "true"
		case "enable-drive", "enable-printing", "enable-audio-input", "enable-wallpaper":
			values[index] = "false"
		case "width":
			values[index] = strconv.Itoa(cols)
		case "height":
			values[index] = strconv.Itoa(rows)
		case "dpi":
			values[index] = "96"
		case "timezone":
			values[index] = "UTC"
		}
	}
	if err = writeGuacamoleInstruction(connection, "connect", values...); err != nil {
		return nil, nil, err
	}
	ready, readyOpcode, _, err := readGuacamoleInstruction(reader)
	if err != nil || readyOpcode != "ready" {
		return nil, nil, errors.New("guacd RDP connection failed")
	}
	_ = connection.SetDeadline(time.Time{})
	failed = false
	return &bufferedConnection{Conn: connection, reader: reader}, ready, nil
}

type bufferedConnection struct {
	net.Conn
	reader *bufio.Reader
}

func (connection *bufferedConnection) Read(value []byte) (int, error) {
	return connection.reader.Read(value)
}

func receiveGuacamoleFrames(ctx context.Context, connection net.Conn, output chan<- guacamoleFrame) {
	reader := bufio.NewReaderSize(connection, maxGuacamoleInstructionBytes)
	for {
		raw, opcode, values, err := readGuacamoleInstruction(reader)
		select {
		case output <- guacamoleFrame{raw: raw, opcode: opcode, values: values, err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func writeGuacamoleInstruction(writer io.Writer, opcode string, values ...string) error {
	parts := append([]string{opcode}, values...)
	var instruction strings.Builder
	for index, value := range parts {
		if index > 0 {
			instruction.WriteByte(',')
		}
		instruction.WriteString(strconv.Itoa(len([]byte(value))))
		instruction.WriteByte('.')
		instruction.WriteString(value)
	}
	instruction.WriteByte(';')
	return writeAll(writer, []byte(instruction.String()))
}

func readGuacamoleInstruction(reader *bufio.Reader) ([]byte, string, []string, error) {
	raw, err := reader.ReadBytes(';')
	if err != nil || len(raw) > maxGuacamoleInstructionBytes {
		return nil, "", nil, errors.New("Guacamole instruction is unavailable")
	}
	opcode, values, err := parseGuacamoleInstruction(raw)
	return raw, opcode, values, err
}

func parseGuacamoleInstruction(raw []byte) (string, []string, error) {
	if len(raw) == 0 || len(raw) > maxGuacamoleInstructionBytes || raw[len(raw)-1] != ';' {
		return "", nil, errors.New("Guacamole instruction is invalid")
	}
	values := []string{}
	for offset := 0; offset < len(raw)-1; {
		dot := strings.IndexByte(string(raw[offset:]), '.')
		if dot <= 0 {
			return "", nil, errors.New("Guacamole element length is invalid")
		}
		dot += offset
		length, err := strconv.Atoi(string(raw[offset:dot]))
		if err != nil || length < 0 || dot+1+length > len(raw)-1 {
			return "", nil, errors.New("Guacamole element is truncated")
		}
		start, end := dot+1, dot+1+length
		values = append(values, string(raw[start:end]))
		if end == len(raw)-1 {
			offset = end
			break
		}
		if raw[end] != ',' {
			return "", nil, errors.New("Guacamole element separator is invalid")
		}
		offset = end + 1
	}
	if len(values) == 0 || values[0] == "" {
		return "", nil, errors.New("Guacamole opcode is missing")
	}
	return values[0], values[1:], nil
}

func prohibitedGuacamoleOpcode(opcode string) bool {
	switch opcode {
	case "clipboard", "file", "pipe", "blob", "ack":
		return true
	default:
		return false
	}
}

func writeAll(writer io.Writer, value []byte) error {
	for len(value) > 0 {
		count, err := writer.Write(value)
		if err != nil {
			return err
		}
		if count == 0 {
			return io.ErrShortWrite
		}
		value = value[count:]
	}
	return nil
}

func (gateway WebSocketGateway) recordGuacamole(ctx context.Context, recording *GatewayRecording, eventType string, raw []byte) error {
	if recording == nil {
		return nil
	}
	chunks, err := recording.Recorder.Append(ctx, RecordingEvent{Time: gateway.now().Sub(recording.Started).Seconds(), Type: eventType, Data: string(raw)})
	if err != nil {
		return err
	}
	return gateway.Service.PersistChunks(ctx, *recording, chunks)
}
