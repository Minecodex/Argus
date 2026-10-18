package runtimewire

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/connectorprotocol"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type Proxy func(context.Context, ProxyRequest) ProxyResponse
type Client struct {
	input          io.WriteCloser
	closeTransport func()
	proxy          Proxy
	mu, writeMu    sync.Mutex
	pending        map[string]chan Message
	done           chan struct{}
	once           sync.Once
	batchID        string
	batchContext   context.Context
	batchCancel    context.CancelFunc
	proxySlots     chan struct{}
}

func NewClient(input io.WriteCloser, output io.Reader, closeTransport func(), proxy Proxy) *Client {
	c := &Client{input: input, closeTransport: closeTransport, proxy: proxy, pending: map[string]chan Message{}, done: make(chan struct{}), proxySlots: make(chan struct{}, 8)}
	go c.read(output)
	return c
}
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.done)
		c.mu.Lock()
		if c.batchCancel != nil {
			c.batchCancel()
		}
		c.mu.Unlock()
		c.input.Close()
		if c.closeTransport != nil {
			c.closeTransport()
		}
	})
}
func (c *Client) send(message Message) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(raw) > MaxFrameBytes {
		return errors.New("REMOTE_FRAME_LIMIT")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err = c.input.Write(append(raw, '\n'))
	return err
}
func (c *Client) read(output io.Reader) {
	defer c.Close()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64<<10), MaxFrameBytes)
	for scanner.Scan() {
		var message Message
		if json.Unmarshal(scanner.Bytes(), &message) != nil || message.ID == "" {
			return
		}
		if message.ProxyRequest != nil {
			select {
			case c.proxySlots <- struct{}{}:
				go c.proxyCall(message)
			default:
				_ = c.send(Message{ID: message.ID, ProxyResponse: &ProxyResponse{Status: http.StatusTooManyRequests}})
			}
			continue
		}
		c.mu.Lock()
		ch := c.pending[message.ID]
		delete(c.pending, message.ID)
		c.mu.Unlock()
		if ch != nil {
			ch <- message
		}
	}
}
func (c *Client) proxyCall(message Message) {
	defer func() { <-c.proxySlots }()
	response := ProxyResponse{Status: http.StatusForbidden}
	c.mu.Lock()
	batchID, batchContext := c.batchID, c.batchContext
	c.mu.Unlock()
	if c.proxy != nil && batchContext != nil && batchContext.Err() == nil && message.ProxyRequest.BatchID == batchID && len(message.ProxyRequest.Body) <= MaxRequestBytes {
		response = c.proxy(batchContext, *message.ProxyRequest)
	}
	_ = c.send(Message{ID: message.ID, ProxyResponse: &response})
}
func (c *Client) Call(ctx context.Context, kind string, payload proto.Message) (proto.Message, error) {
	raw, err := protojson.Marshal(payload)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	if kind == "remote_execution_batch" {
		batchContext, cancel := context.WithCancel(ctx)
		c.mu.Lock()
		if c.batchContext != nil {
			c.mu.Unlock()
			cancel()
			return nil, errors.New("REMOTE_SESSION_BUSY")
		}
		c.batchID, c.batchContext, c.batchCancel = id, batchContext, cancel
		c.mu.Unlock()
		defer func() {
			cancel()
			c.mu.Lock()
			c.batchID, c.batchContext, c.batchCancel = "", nil, nil
			c.mu.Unlock()
		}()
	}
	ch := make(chan Message, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if err = c.send(Message{ID: id, Kind: kind, Payload: raw}); err != nil {
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	}
	select {
	case <-ctx.Done():
		return nil, errors.New("REMOTE_RESULT_UNKNOWN")
	case <-c.done:
		return nil, errors.New("REMOTE_CONTEXT_LOST")
	case response := <-ch:
		if response.Error != "" {
			return nil, errors.New(response.Error)
		}
		result, err := connectorprotocol.Result(kind)
		if err != nil {
			return nil, err
		}
		if err = protojson.Unmarshal(response.Payload, result); err != nil {
			return nil, errors.New("REMOTE_COMPLETION_INVALID")
		}
		return result, nil
	}
}
