package kubernetesstreamgateway

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/remoteaccess"
)

type WebSocketGateway struct {
	Service          kubernetesworkbench.Service
	InstanceID       string
	Backends         BackendFactory
	AllowedOrigins   []string
	Sessions         *remoteaccess.SessionTracker
	RejectNew, Drain <-chan struct{}
	Logger           *slog.Logger
}
type ServerFrame struct {
	Protocol  string    `json:"protocol"`
	Type      string    `json:"type"`
	Sequence  uint64    `json:"sequence"`
	Nonce     string    `json:"nonce,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	Data      string    `json:"data,omitempty"`
	Status    string    `json:"status,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

func (g WebSocketGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	leave, ok := g.Sessions.Enter()
	if !ok {
		http.Error(w, "Gateway draining", 503)
		return
	}
	defer leave()
	select {
	case <-g.RejectNew:
		http.Error(w, "Gateway draining", 503)
		return
	default:
	}
	if r.Method != http.MethodGet || g.Backends == nil || g.Service.Store == nil || g.InstanceID == "" {
		http.Error(w, "Kubernetes stream unavailable", 503)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/kubernetes/sessions/")
	id, err := uuid.Parse(path)
	if err != nil || r.URL.RawQuery != "" {
		http.NotFound(w, r)
		return
	}
	if !g.originAllowed(r) {
		http.Error(w, "origin denied", 403)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: g.originPatterns()})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)
	g.serve(r.Context(), conn, id)
}
func (g WebSocketGateway) originAllowed(r *http.Request) bool {
	raw := r.Header.Get("Origin")
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	for _, allowed := range g.AllowedOrigins {
		if strings.EqualFold(strings.TrimSuffix(allowed, "/"), raw) {
			return true
		}
	}
	return strings.EqualFold(u.Host, r.Host)
}

func (g WebSocketGateway) originPatterns() []string {
	// Accept uses path.Match; configured origins are exact URLs, not globs.
	escape := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`)
	patterns := make([]string, 0, len(g.AllowedOrigins))
	for _, origin := range g.AllowedOrigins {
		patterns = append(patterns, escape.Replace(strings.TrimSuffix(origin, "/")))
	}
	return patterns
}

func sendFrame(ctx context.Context, conn *websocket.Conn, sequence *uint64, frame ServerFrame) error {
	*sequence += 1
	frame.Protocol, frame.Sequence = kubernetesstream.WebSocketProtocol, *sequence
	raw, _ := json.Marshal(frame)
	write, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(write, websocket.MessageText, raw)
}
func (g WebSocketGateway) serve(parent context.Context, conn *websocket.Conn, id uuid.UUID) {
	helloCtx, cancel := context.WithTimeout(parent, 5*time.Second)
	kind, raw, err := conn.Read(helloCtx)
	cancel()
	var hello interactive.Input
	if err != nil || kind != websocket.MessageText || json.Unmarshal(raw, &hello) != nil || hello.Protocol != kubernetesstream.WebSocketProtocol || hello.Type != "client_hello" || hello.Sequence != 1 || hello.Nonce != "" || hello.Cols < 20 || hello.Cols > 500 || hello.Rows < 5 || hello.Rows > 300 {
		_ = conn.Close(websocket.StatusPolicyViolation, "KUBERNETES_STREAM_PROTOCOL_ERROR")
		return
	}
	claimCtx, claimCancel := context.WithTimeout(parent, 5*time.Second)
	value, err := g.Service.ClaimStream(claimCtx, id, hello.Ticket, g.InstanceID)
	claimCancel()
	hello.Ticket = ""
	if err != nil {
		_ = conn.Close(websocket.StatusPolicyViolation, "KUBERNETES_STREAM_TICKET_DENIED")
		return
	}
	ctx, stop := context.WithDeadline(parent, value.Row.ExpiresAt.Time)
	defer stop()
	var bytesIn atomic.Int64
	var bytesOut int64
	status, reason := "connection_lost", "client_disconnected"
	finalized := false
	finish := func() {
		if finalized {
			return
		}
		finish, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		err := g.Service.FinishStream(finish, id, value.Row.Fence, status, reason, bytesIn.Load(), bytesOut)
		finalized = err == nil
		if err != nil && g.Logger != nil {
			g.Logger.Error("finalize Kubernetes stream metadata failed", "session_id", id, "error", err)
		}
	}
	defer finish()
	backendCtx, stopBackend := context.WithCancel(ctx)
	defer stopBackend()
	backend, err := g.Backends.Open(backendCtx, value, hello.Cols, hello.Rows)
	if err != nil {
		status, reason = "connection_lost", "KUBERNETES_STREAM_BACKEND_UNAVAILABLE"
		if errors.Is(err, kubernetesinventory.ErrForbidden) {
			reason = kubernetesinventory.ErrForbidden.Error()
		}
		_ = conn.Close(websocket.StatusInternalError, reason)
		return
	}
	defer backend.Close(context.Background(), "gateway_closed")
	if err = g.Service.ActivateStream(ctx, id, value.Row.Fence, g.InstanceID); err != nil {
		status, reason = "invalidated", "KUBERNETES_STREAM_AUTHORIZATION_CHANGED"
		return
	}
	nonceBytes := make([]byte, 24)
	if _, err = rand.Read(nonceBytes); err != nil {
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	sequence := uint64(0)
	if sendFrame(ctx, conn, &sequence, ServerFrame{Type: "server_ready", Nonce: nonce, Mode: value.Row.Mode, Status: "active", ExpiresAt: value.Row.ExpiresAt.Time}) != nil {
		return
	}
	reads := make(chan wsRead, 1)
	outputs := make(chan backendRead, 1)
	inputs := make(chan interactive.Input, 8)
	sendErrors := make(chan error, 1)
	senderDone := make(chan struct{})
	go readWS(ctx, conn, reads)
	go readBackend(backendCtx, backend, outputs)
	go func() {
		defer close(senderDone)
		for {
			select {
			case <-backendCtx.Done():
				return
			case frame := <-inputs:
				err := backend.Send(backendCtx, frame)
				if err != nil {
					select {
					case sendErrors <- err:
					case <-backendCtx.Done():
					}
					return
				}
				if frame.Type == "input" {
					bytesIn.Add(int64(len(frame.Data)))
				}
			}
		}
	}()
	defer func() {
		stopBackend()
		_ = backend.Close(context.Background(), "gateway_closed")
		select {
		case <-senderDone:
		case <-time.After(2 * time.Second):
		}
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastInput, lastAuth := time.Now(), time.Now()
	clientSequence := uint64(1)
	check := func() bool {
		checkCtx, c := context.WithTimeout(ctx, time.Second)
		defer c()
		if err := g.Service.HeartbeatStream(checkCtx, id, value.Row.Fence, g.InstanceID); err != nil {
			row, e := g.Service.Store.Queries.GetKubernetesStreamSession(checkCtx, id)
			if e == nil && row.Status == "closing" {
				status, reason = "closed", "client_close"
			} else {
				status, reason = "invalidated", "KUBERNETES_STREAM_AUTHORIZATION_CHANGED"
			}
			return false
		}
		lastAuth = time.Now()
		return true
	}
	defer func() {
		stopBackend()
		_ = backend.Close(context.Background(), "gateway_closed")
		select {
		case <-senderDone:
		case <-time.After(2 * time.Second):
		}
		finish()
		_ = sendFrame(context.Background(), conn, &sequence, ServerFrame{Type: "session_state", Status: status, Reason: reason})
		_ = conn.Close(websocket.StatusNormalClosure, reason)
	}()
	for {
		select {
		case <-ctx.Done():
			status, reason = "closed", "session_expired"
			return
		case <-g.Drain:
			status, reason = "connection_lost", "gateway_draining"
			return
		case <-sendErrors:
			return
		case <-ticker.C:
			if time.Since(lastInput) > 5*time.Minute {
				status, reason = "closed", "idle_timeout"
				return
			}
			if !check() {
				return
			}
		case item := <-reads:
			if item.err != nil {
				return
			}
			if time.Since(lastAuth) >= time.Second && !check() {
				return
			}
			var frame interactive.Input
			if item.kind == websocket.MessageBinary && value.Row.Mode == "port_forward" {
				if len(item.raw) > kubernetesstream.MaxFrameBytes {
					status, reason = "invalidated", "KUBERNETES_STREAM_PROTOCOL_ERROR"
					return
				}
				frame = interactive.Input{Type: "input", Data: string(item.raw)}
			} else if item.kind == websocket.MessageText && json.Unmarshal(item.raw, &frame) == nil && frame.Protocol == kubernetesstream.WebSocketProtocol && frame.Nonce == nonce && frame.Sequence == clientSequence+1 {
				clientSequence = frame.Sequence
				if frame.Type == "close" {
					status, reason = "closed", "client_close"
					return
				}
				if frame.Type == "ping" {
					if sendFrame(ctx, conn, &sequence, ServerFrame{Type: "pong"}) != nil {
						return
					}
					continue
				}
				if frame.Type != "resize" && (frame.Type != "input" || value.Row.Mode != "exec" || len(frame.Data) > kubernetesstream.MaxFrameBytes) {
					status, reason = "invalidated", "KUBERNETES_STREAM_PROTOCOL_ERROR"
					return
				}
			} else {
				status, reason = "invalidated", "KUBERNETES_STREAM_PROTOCOL_ERROR"
				return
			}
			lastInput = time.Now()
			select {
			case inputs <- frame:
			default:
				status, reason = "connection_lost", "input_backpressure"
				return
			}
		case item := <-outputs:
			if item.err != nil {
				if errors.Is(item.err, io.EOF) {
					status, reason = "closed", "remote_closed"
				}
				return
			}
			if item.frame.Type == "state" {
				status, reason = item.frame.Status, item.frame.Reason
				if reason == "KUBERNETES_OBJECT_IDENTITY_CHANGED" {
					status = "invalidated"
				}
				if status != "closed" && status != "invalidated" {
					status = "connection_lost"
				}
				return
			}
			if time.Since(lastAuth) >= time.Second && !check() {
				return
			}
			var e error
			if value.Row.Mode == "port_forward" {
				write, c := context.WithTimeout(ctx, 5*time.Second)
				e = conn.Write(write, websocket.MessageBinary, item.frame.Data)
				c()
			} else {
				e = sendFrame(ctx, conn, &sequence, ServerFrame{Type: "output", Data: base64.StdEncoding.EncodeToString(item.frame.Data)})
			}
			if e != nil {
				return
			}
			bytesOut += int64(len(item.frame.Data))
			lastInput = time.Now()
		}
	}
}

type wsRead struct {
	kind websocket.MessageType
	raw  []byte
	err  error
}

func readWS(ctx context.Context, conn *websocket.Conn, out chan<- wsRead) {
	for {
		kind, raw, err := conn.Read(ctx)
		select {
		case out <- wsRead{kind, raw, err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

type backendRead struct {
	frame interactive.Frame
	err   error
}

func readBackend(ctx context.Context, s interactive.Session, out chan<- backendRead) {
	for {
		frame, err := s.Receive(ctx)
		select {
		case out <- backendRead{frame, err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}
