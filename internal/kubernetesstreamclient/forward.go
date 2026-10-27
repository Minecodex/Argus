// Package kubernetesstreamclient exposes one fixed TCP destination through an
// authenticated Kubernetes session. It never accepts remote destination names.
package kubernetesstreamclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/kakj-go/Argus/internal/interactive"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
)

func ValidateURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/v1/kubernetes/sessions/") || u.Hostname() == "" {
		return errors.New("invalid Kubernetes stream URL")
	}
	if u.Scheme == "wss" {
		return nil
	}
	host := strings.Trim(u.Hostname(), "[]")
	if u.Scheme == "ws" && (host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()) {
		return nil
	}
	return errors.New("Kubernetes stream requires TLS; ws is allowed only on loopback")
}

type Options struct {
	URL, Ticket string
	HTTPClient  *http.Client
	Ready       func()
}
type channel struct {
	conn net.Conn
	done chan struct{}
}
type proxy struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ws       *websocket.Conn
	mu       sync.Mutex
	channels map[uint32]*channel
	next     uint32
	workers  sync.WaitGroup
}

func Forward(parent context.Context, listener net.Listener, options Options) error {
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok || !address.IP.IsLoopback() {
		return errors.New("TCP listener must bind a loopback IP")
	}
	if e := ValidateURL(options.URL); e != nil {
		return e
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, options.URL, &websocket.DialOptions{HTTPClient: options.HTTPClient})
	if err != nil {
		return errors.New("Kubernetes stream connection unavailable")
	}
	defer ws.CloseNow()
	ws.SetReadLimit(64 << 10)
	hello := interactive.Input{Protocol: kubernetesstream.WebSocketProtocol, Type: "client_hello", Sequence: 1, Ticket: options.Ticket, Cols: 100, Rows: 30}
	raw, _ := json.Marshal(hello)
	hello.Ticket = ""
	options.Ticket = ""
	handshake, c := context.WithTimeout(ctx, 20*time.Second)
	err = ws.Write(handshake, websocket.MessageText, raw)
	clear(raw)
	if err != nil {
		c()
		return errors.New("Kubernetes stream handshake failed")
	}
	kind, raw, err := ws.Read(handshake)
	c()
	var ready struct {
		Protocol, Type, Nonce, Mode string
		Sequence                    uint64
	}
	if err != nil || kind != websocket.MessageText || json.Unmarshal(raw, &ready) != nil || ready.Protocol != kubernetesstream.WebSocketProtocol || ready.Type != "server_ready" || ready.Sequence != 1 || ready.Mode != "port_forward" || len(ready.Nonce) < 24 {
		return errors.New("Kubernetes forwarding session denied")
	}
	p := &proxy{ctx: ctx, cancel: cancel, ws: ws, channels: map[uint32]*channel{}}
	acceptDone := make(chan struct{})
	defer func() {
		cancel()
		_ = listener.Close()
		_ = ws.CloseNow()
		p.mu.Lock()
		for _, c := range p.channels {
			_ = c.conn.Close()
		}
		p.mu.Unlock()
		<-acceptDone
		p.workers.Wait()
	}()
	go func() { <-ctx.Done(); _ = listener.Close(); _ = ws.CloseNow() }()
	errorsChannel := make(chan error, 1)
	go func() {
		defer close(acceptDone)
		e := p.accept(listener)
		select {
		case errorsChannel <- e:
		case <-ctx.Done():
		}
	}()
	if options.Ready != nil {
		options.Ready()
	}
	received := uint64(1)
	for {
		kind, raw, err = ws.Read(ctx)
		if err != nil {
			select {
			case e := <-errorsChannel:
				if e != nil {
					return e
				}
			default:
			}
			if parent.Err() != nil {
				return parent.Err()
			}
			return errors.New("Kubernetes forwarding session ended")
		}
		if kind == websocket.MessageText {
			var state struct {
				Protocol, Type, Status, Reason string
				Sequence                       uint64
			}
			if json.Unmarshal(raw, &state) != nil || state.Protocol != kubernetesstream.WebSocketProtocol || state.Sequence != received+1 {
				return kubernetesstream.ErrProtocol
			}
			received++
			if state.Type == "session_state" {
				if state.Status == "closed" {
					return nil
				}
				return errors.New("Kubernetes forwarding session invalidated")
			}
			if state.Type != "pong" {
				return kubernetesstream.ErrProtocol
			}
			continue
		}
		if kind != websocket.MessageBinary {
			return kubernetesstream.ErrProtocol
		}
		code, id, data, e := kubernetesstream.ParsePacket(raw)
		if e != nil {
			return e
		}
		p.mu.Lock()
		ch := p.channels[id]
		p.mu.Unlock()
		if ch == nil {
			if code == kubernetesstream.CloseChannel {
				continue
			}
			return kubernetesstream.ErrProtocol
		}
		switch code {
		case kubernetesstream.ChannelData:
			_ = ch.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, e = ch.conn.Write(data); e != nil {
				p.closeChannel(id, ch)
				_ = p.write(kubernetesstream.Packet(kubernetesstream.CloseChannel, id, nil))
			}
		case kubernetesstream.CloseChannel:
			p.closeChannel(id, ch)
		default:
			return kubernetesstream.ErrProtocol
		}
	}
}
func (p *proxy) write(raw []byte) error {
	ctx, c := context.WithTimeout(p.ctx, 5*time.Second)
	defer c()
	return p.ws.Write(ctx, websocket.MessageBinary, raw)
}
func (p *proxy) accept(listener net.Listener) error {
	for {
		conn, e := listener.Accept()
		if e != nil {
			return e
		}
		p.mu.Lock()
		if len(p.channels) >= 8 || p.next == ^uint32(0) {
			p.mu.Unlock()
			_ = conn.Close()
			continue
		}
		p.next++
		id := p.next
		ch := &channel{conn: conn, done: make(chan struct{})}
		p.channels[id] = ch
		p.mu.Unlock()
		if e = p.write(kubernetesstream.Packet(kubernetesstream.OpenChannel, id, nil)); e != nil {
			p.closeChannel(id, ch)
			p.cancel()
			return e
		}
		p.workers.Add(1)
		go p.upload(id, ch)
	}
}
func (p *proxy) upload(id uint32, ch *channel) {
	defer p.workers.Done()
	buf := make([]byte, 16<<10)
	for {
		n, e := ch.conn.Read(buf)
		if n > 0 {
			if err := p.write(kubernetesstream.Packet(kubernetesstream.ChannelData, id, buf[:n])); err != nil {
				p.cancel()
				return
			}
		}
		if e != nil {
			if errors.Is(e, io.EOF) {
				if err := p.write(kubernetesstream.Packet(kubernetesstream.ChannelEOF, id, nil)); err != nil {
					p.cancel()
				}
				return
			}
			_ = p.write(kubernetesstream.Packet(kubernetesstream.CloseChannel, id, nil))
			p.closeChannel(id, ch)
			return
		}
	}
}
func (p *proxy) closeChannel(id uint32, ch *channel) {
	p.mu.Lock()
	if p.channels[id] == ch {
		delete(p.channels, id)
		close(ch.done)
	}
	p.mu.Unlock()
	_ = ch.conn.Close()
}
func Listen(raw string) (net.Listener, error) {
	host, port, e := net.SplitHostPort(raw)
	ip := net.ParseIP(host)
	if e != nil || ip == nil || !ip.IsLoopback() || port == "" {
		return nil, fmt.Errorf("listen must use an explicit loopback IP and port")
	}
	return net.Listen("tcp", raw)
}
