package kubernetesstream

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strconv"

	"github.com/kakj-go/Argus/internal/interactive"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	OpenChannel  byte = 1
	ChannelData  byte = 2
	CloseChannel byte = 3
	ChannelEOF   byte = 4
)

func Packet(kind byte, id uint32, data []byte) []byte {
	out := make([]byte, 5+len(data))
	out[0] = kind
	binary.BigEndian.PutUint32(out[1:5], id)
	copy(out[5:], data)
	return out
}
func ParsePacket(value []byte) (byte, uint32, []byte, error) {
	if len(value) < 5 || len(value) > MaxFrameBytes || value[0] < 1 || value[0] > 4 || binary.BigEndian.Uint32(value[1:5]) == 0 {
		return 0, 0, nil, ErrProtocol
	}
	kind := value[0]
	if kind != ChannelData && len(value) != 5 || kind == ChannelData && len(value) == 5 {
		return 0, 0, nil, ErrProtocol
	}
	return kind, binary.BigEndian.Uint32(value[1:5]), value[5:], nil
}

func (s *Session) runForward() error {
	request := s.api.CoreV1().RESTClient().Post().Namespace(s.target.Pin.Namespace).Resource("pods").Name(s.target.Pin.Pod).SubResource("portforward")
	transport, upgrader, err := spdy.RoundTripperFor(s.config)
	if err != nil {
		return err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrProtocol }}, http.MethodPost, request.URL())
	ready := make(chan struct{})
	forward, err := portforward.NewOnAddresses(dialer, []string{"127.0.0.1"}, []string{"0:" + strconv.Itoa(int(s.target.Port))}, s.ctx.Done(), ready, io.Discard, io.Discard)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- forward.ForwardPorts() }()
	select {
	case <-ready:
		ports, err := forward.GetPorts()
		if err != nil || len(ports) != 1 {
			return ErrProtocol
		}
		s.localPort = ports[0].Local
		s.markReady()
	case err := <-done:
		return err
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	return <-done
}
func (s *Session) forwardInput(ctx context.Context, raw []byte) error {
	kind, id, data, err := ParsePacket(raw)
	if err != nil {
		return err
	}
	s.mu.Lock()
	connection := s.connections[id]
	if kind == OpenChannel {
		if connection != nil || id <= s.lastChannel || len(s.connections) >= 8 || s.localPort == 0 {
			s.mu.Unlock()
			return ErrProtocol
		}
		s.lastChannel = id
		s.mu.Unlock()
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(s.localPort))))
		if err != nil {
			return err
		}
		s.mu.Lock()
		select {
		case <-s.finished:
			s.mu.Unlock()
			conn.Close()
			return io.EOF
		default:
		}
		s.connections[id] = conn
		s.mu.Unlock()
		go s.forwardOutput(id, conn)
		return nil
	}
	s.mu.Unlock()
	if connection == nil {
		if kind == CloseChannel {
			return nil
		}
		return ErrProtocol
	}
	switch kind {
	case ChannelData:
		_, err = connection.Write(data)
		return err
	case ChannelEOF:
		if conn, ok := connection.(*net.TCPConn); ok {
			return conn.CloseWrite()
		}
		return ErrProtocol
	case CloseChannel:
		s.mu.Lock()
		delete(s.connections, id)
		s.mu.Unlock()
		return connection.Close()
	}
	return ErrProtocol
}
func (s *Session) forwardOutput(id uint32, connection net.Conn) {
	defer func() {
		s.mu.Lock()
		delete(s.connections, id)
		s.mu.Unlock()
		connection.Close()
		s.emit(interactive.Frame{Type: "output", Stream: "stdout", Data: Packet(CloseChannel, id, nil)})
	}()
	buffer := make([]byte, 16<<10)
	for {
		n, err := connection.Read(buffer)
		if n > 0 && !s.emit(interactive.Frame{Type: "output", Stream: "stdout", Data: Packet(ChannelData, id, buffer[:n])}) {
			return
		}
		if err != nil {
			return
		}
	}
}
