package kubernetesstream

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const MaxFrameBytes = 48 << 10

type Session struct {
	ctx               context.Context
	cancel            context.CancelFunc
	target            Target
	api               kubernetes.Interface
	config            *rest.Config
	events            chan interactive.Frame
	ready             chan struct{}
	readyOnce         sync.Once
	finished          chan struct{}
	finishOnce        sync.Once
	stdin             *io.PipeWriter
	pipeRead          *io.PipeReader
	resize            chan remotecommand.TerminalSize
	mu                sync.Mutex
	connections       map[uint32]io.ReadWriteCloser
	lastChannel       uint32
	localPort         uint16
	lastValidation    time.Time
	terminal          interactive.Frame
	terminalDelivered bool
	startupError      error
}

func Open(ctx context.Context, config *rest.Config, target Target, cols, rows int) (*Session, error) {
	if target.Validate() != nil || cols < 20 || cols > 500 || rows < 5 || rows > 300 {
		return nil, ErrProtocol
	}
	cfg := rest.CopyConfig(config)
	cfg.Host = target.APIServer
	cfg.Timeout = 0
	api, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, kubernetesinventory.ErrUnavailable
	}
	lifetime, cancel := context.WithCancel(ctx)
	s := &Session{ctx: lifetime, cancel: cancel, target: target, api: api, config: cfg, events: make(chan interactive.Frame, 32), ready: make(chan struct{}), finished: make(chan struct{}), resize: make(chan remotecommand.TerminalSize, 1), connections: map[uint32]io.ReadWriteCloser{}}
	if err = ValidateTarget(ctx, api, target); err != nil {
		cancel()
		return nil, err
	}
	s.lastValidation = time.Now()
	if target.Mode == "exec" {
		s.pipeRead, s.stdin = io.Pipe()
	}
	go func() {
		var e error
		if target.Mode == "exec" {
			e = s.runExec(cols, rows)
		} else {
			e = s.runForward()
		}
		reason := "remote_closed"
		state := "closed"
		if e != nil && lifetime.Err() == nil {
			reason = "KUBERNETES_STREAM_BACKEND_LOST"
			state = "connection_lost"
			failure := accessError(e)
			s.mu.Lock()
			s.startupError = failure
			s.mu.Unlock()
			if errors.Is(failure, kubernetesinventory.ErrForbidden) {
				reason = failure.Error()
			}
		}
		s.finish(state, reason)
	}()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	select {
	case <-s.ready:
		if err = ValidateTarget(ctx, api, target); err != nil {
			_ = s.Close(ctx, "target_changed")
			return nil, err
		}
		select {
		case <-s.finished:
			return nil, s.openError()
		default:
		}
		go s.guard()
		return s, nil
	case <-s.finished:
		return nil, s.openError()
	case <-timer.C:
		_ = s.Close(ctx, "handshake_timeout")
		return nil, kubernetesinventory.ErrUnavailable
	case <-ctx.Done():
		_ = s.Close(context.Background(), "cancelled")
		return nil, ctx.Err()
	}
}
func (s *Session) openError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.startupError != nil {
		return s.startupError
	}
	return kubernetesinventory.ErrUnavailable
}
func (s *Session) markReady() { s.readyOnce.Do(func() { close(s.ready) }) }
func (s *Session) emit(frame interactive.Frame) bool {
	select {
	case s.events <- frame:
		return true
	case <-s.ctx.Done():
		return false
	}
}
func (s *Session) finish(state, reason string) {
	s.finishOnce.Do(func() {
		s.mu.Lock()
		for id, c := range s.connections {
			_ = c.Close()
			delete(s.connections, id)
		}
		s.terminal = interactive.Frame{Type: "state", Status: state, Reason: reason}
		s.mu.Unlock()
		if s.stdin != nil {
			_ = s.stdin.Close()
		}
		close(s.finished)
		s.cancel()
	})
}
func (s *Session) guard() {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			check, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			err := ValidateTarget(check, s.api, s.target)
			cancel()
			if err != nil {
				state, reason := "connection_lost", kubernetesinventory.ErrUnavailable.Error()
				if errors.Is(err, kubernetesinventory.ErrIdentityChanged) || errors.Is(err, kubernetesinventory.ErrForbidden) {
					state, reason = "invalidated", err.Error()
				}
				s.finish(state, reason)
				return
			}
		}
	}
}
func (s *Session) Send(ctx context.Context, frame interactive.Input) error {
	if len(frame.Data) > MaxFrameBytes {
		return ErrProtocol
	}
	select {
	case <-s.finished:
		return io.EOF
	default:
	}
	switch frame.Type {
	case "input":
		if s.target.Mode == "port_forward" {
			return s.forwardInput(ctx, []byte(frame.Data))
		}
		if s.stdin == nil {
			return ErrProtocol
		}
		_, err := s.stdin.Write([]byte(frame.Data))
		return err
	case "resize":
		if s.target.Mode != "exec" || frame.Cols < 20 || frame.Cols > 500 || frame.Rows < 5 || frame.Rows > 300 {
			return ErrProtocol
		}
		select {
		case s.resize <- remotecommand.TerminalSize{Width: uint16(frame.Cols), Height: uint16(frame.Rows)}:
		default:
		}
		return nil
	case "close":
		return s.Close(ctx, "client_close")
	default:
		return ErrProtocol
	}
}
func (s *Session) Receive(ctx context.Context) (interactive.Frame, error) {
	s.mu.Lock()
	ended := s.terminalDelivered
	s.mu.Unlock()
	if ended {
		return interactive.Frame{}, io.EOF
	}
	select {
	case frame := <-s.events:
		return frame, nil
	case <-s.finished:
		select {
		case frame := <-s.events:
			return frame, nil
		default:
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.terminalDelivered {
				return interactive.Frame{}, io.EOF
			}
			s.terminalDelivered = true
			return s.terminal, nil
		}
	case <-ctx.Done():
		return interactive.Frame{}, ctx.Err()
	}
}
func (s *Session) Close(_ context.Context, reason string) error {
	s.finish("closed", reason)
	return nil
}

type outputWriter struct {
	s      *Session
	stream string
}

func (w outputWriter) Write(value []byte) (int, error) {
	w.s.markReady()
	for offset := 0; offset < len(value); {
		n := min(len(value)-offset, 16<<10)
		if !w.s.emit(interactive.Frame{Type: "output", Stream: w.stream, Data: append([]byte(nil), value[offset:offset+n]...)}) {
			return offset, io.EOF
		}
		offset += n
	}
	return len(value), nil
}

type terminalQueue struct{ s *Session }

func (q terminalQueue) Next() *remotecommand.TerminalSize {
	select {
	case value := <-q.s.resize:
		return &value
	case <-q.s.ctx.Done():
		return nil
	}
}
