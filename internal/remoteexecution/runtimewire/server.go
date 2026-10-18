package runtimewire

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution/runner"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type server struct {
	manager     *runner.Manager
	writer      io.Writer
	writeMu, mu sync.Mutex
	pending     map[string]chan *ProxyResponse
	batchID     string
}

func Serve(ctx context.Context, manager *runner.Manager, input io.Reader, output io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := &server{manager: manager, writer: output, pending: map[string]chan *ProxyResponse{}}
	if manager.KubernetesInterpreter == "kubectl" {
		cleanup, err := s.startProxy(ctx)
		if err != nil {
			return err
		}
		defer cleanup()
	}
	var tasks sync.WaitGroup
	slots := make(chan struct{}, 16)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), MaxFrameBytes)
readLoop:
	for scanner.Scan() {
		var request Message
		if json.Unmarshal(scanner.Bytes(), &request) != nil || request.ID == "" {
			cancel()
			break
		}
		if request.ProxyResponse != nil {
			s.mu.Lock()
			ch := s.pending[request.ID]
			delete(s.pending, request.ID)
			s.mu.Unlock()
			if ch != nil {
				ch <- request.ProxyResponse
			}
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			break readLoop
		}
		tasks.Add(1)
		go func(request Message) {
			defer tasks.Done()
			defer func() { <-slots }()
			result, err := s.execute(ctx, request)
			response := Message{ID: request.ID}
			if err != nil {
				response.Error = "REMOTE_RUNTIME_FAILED"
				if strings.HasPrefix(err.Error(), "REMOTE_") {
					response.Error = err.Error()
				}
			} else {
				response.Payload, _ = protojson.Marshal(result)
			}
			_ = s.send(response)
		}(request)
	}
	cancel()
	tasks.Wait()
	manager.Close()
	return scanner.Err()
}
func (s *server) send(message Message) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(raw) > MaxFrameBytes {
		return errors.New("REMOTE_FRAME_LIMIT")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.writer.Write(append(raw, '\n'))
	return err
}
func (s *server) execute(ctx context.Context, request Message) (proto.Message, error) {
	switch request.Kind {
	case "remote_execution_batch":
		var p connectorv1.RemoteExecutionBatch
		if err := protojson.Unmarshal(request.Payload, &p); err != nil {
			return nil, err
		}
		s.mu.Lock()
		if s.batchID != "" {
			s.mu.Unlock()
			return nil, errors.New("REMOTE_SESSION_BUSY")
		}
		s.batchID = request.ID
		s.mu.Unlock()
		defer func() { s.mu.Lock(); s.batchID = ""; s.mu.Unlock() }()
		return s.manager.Execute(ctx, &p)
	case "remote_execution_control":
		var p connectorv1.RemoteExecutionControl
		if err := protojson.Unmarshal(request.Payload, &p); err != nil {
			return nil, err
		}
		return s.manager.Control(ctx, &p)
	case "remote_execution_file":
		var p connectorv1.RemoteExecutionFile
		if err := protojson.Unmarshal(request.Payload, &p); err != nil {
			return nil, err
		}
		return s.manager.File(ctx, &p)
	default:
		return nil, errors.New("REMOTE_INPUT_INVALID")
	}
}
func (s *server) startProxy(ctx context.Context) (func(), error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "argus-kubectl-")
	if err != nil {
		listener.Close()
		return nil, err
	}
	config := "apiVersion: v1\nkind: Config\nclusters:\n- name: approved\n  cluster:\n    server: http://" + listener.Addr().String() + "\ncontexts:\n- name: approved\n  context:\n    cluster: approved\ncurrent-context: approved\n"
	if err = os.WriteFile(filepath.Join(root, "config"), []byte(config), 0600); err != nil {
		listener.Close()
		return nil, err
	}
	if err = os.Setenv("KUBECONFIG", filepath.Join(root, "config")); err != nil {
		listener.Close()
		return nil, err
	}
	if err = os.Setenv("ARGUS_KUBECTL_PROXY", "http://"+listener.Addr().String()); err != nil {
		listener.Close()
		return nil, err
	}
	web := &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, MaxHeaderBytes: 16 << 10, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		batchID := s.batchID
		s.mu.Unlock()
		if batchID == "" {
			http.Error(w, "No approved batch is running", http.StatusForbidden)
			return
		}
		if r.Header.Get("Upgrade") != "" {
			http.Error(w, "Use the Pod exec interpreter for streaming commands", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBytes+1))
		if err != nil || len(body) > MaxRequestBytes {
			http.Error(w, "Request exceeds limit", http.StatusRequestEntityTooLarge)
			return
		}
		id := uuid.NewString()
		ch := make(chan *ProxyResponse, 1)
		s.mu.Lock()
		s.pending[id] = ch
		s.mu.Unlock()
		defer func() { s.mu.Lock(); delete(s.pending, id); s.mu.Unlock() }()
		if err = s.send(Message{ID: id, ProxyRequest: &ProxyRequest{BatchID: batchID, Method: r.Method, URI: r.URL.RequestURI(), Header: r.Header, Body: body}}); err != nil {
			http.Error(w, "Connector unavailable", http.StatusBadGateway)
			return
		}
		timer := time.NewTimer(30 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			http.Error(w, "Session closed", http.StatusServiceUnavailable)
		case <-r.Context().Done():
			return
		case <-timer.C:
			http.Error(w, "Request timed out", http.StatusGatewayTimeout)
		case response := <-ch:
			for _, name := range []string{"Content-Type", "Warning", "Retry-After"} {
				for _, v := range response.Header.Values(name) {
					w.Header().Add(name, v)
				}
			}
			if response.Status < 100 || response.Status > 599 || len(response.Body) > MaxResponseBytes {
				http.Error(w, "Invalid API response", http.StatusBadGateway)
				return
			}
			w.WriteHeader(response.Status)
			_, _ = w.Write(response.Body)
		}
	})}
	go func() { _ = web.Serve(listener) }()
	return func() { _ = web.Close(); _ = os.RemoveAll(root) }, nil
}
