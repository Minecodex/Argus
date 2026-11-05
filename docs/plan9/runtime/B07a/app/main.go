package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	logapi "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type scenario struct {
	Mode  string `json:"mode"`
	Until int64  `json:"until"`
}

func currentScenario() string {
	var s scenario
	raw, err := os.ReadFile("/var/lib/plan9/scenario.json")
	if err == nil && json.Unmarshal(raw, &s) == nil && s.Until > time.Now().Unix() && s.Until <= time.Now().Add(190*time.Second).Unix() {
		return s.Mode
	}
	return "normal"
}

type service struct {
	name string
	cfg  configuration
	t    *telemetry
	db   *redis.Client
	http *http.Client
}

func (s *service) request(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		w.WriteHeader(200)
		return
	}
	path := "/orders"
	if s.name == "orders-worker" {
		path = "/work"
	}
	if r.URL.Path != path {
		http.NotFound(w, r)
		return
	}
	parent := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
	ctx, span := s.t.tracer.Start(parent, "GET "+path, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.request.method", "GET"), attribute.String("http.route", path)))
	defer span.End()
	started := time.Now()
	s.t.active.Add(ctx, 1)
	defer s.t.active.Add(ctx, -1)
	status := 200
	rid := r.Header.Get("X-Request-ID")
	if len(rid) > 80 {
		rid = "bounded-request-id"
	}
	mode := currentScenario()
	payload := map[string]any{"service": s.name, "request_id": rid, "trace_id": span.SpanContext().TraceID().String()}
	if s.name == "orders-gateway" {
		cctx, clientSpan := s.t.tracer.Start(ctx, "GET orders-worker/work", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("server.address", "orders-worker"), attribute.Int("server.port", 18081), attribute.String("http.request.method", "GET")))
		req, _ := http.NewRequestWithContext(cctx, http.MethodGet, "http://127.0.0.1:18081/work?item="+r.URL.Query().Get("item"), nil)
		req.Header.Set("X-Request-ID", rid)
		otel.GetTextMapPropagator().Inject(cctx, propagation.HeaderCarrier(req.Header))
		res, err := s.http.Do(req)
		if err != nil {
			status = 502
			clientSpan.SetStatus(codes.Error, "worker request failed")
		} else {
			status = res.StatusCode
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 65536))
			res.Body.Close()
			payload["worker_response"] = string(raw)
			clientSpan.SetAttributes(attribute.Int("http.response.status_code", status))
			if status >= 500 {
				clientSpan.SetStatus(codes.Error, "worker returned error")
			}
		}
		clientSpan.End()
	} else {
		dctx, dbSpan := s.t.tracer.Start(ctx, "Redis SET/GET", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.system", "redis"), attribute.String("db.system.name", "redis"), attribute.String("server.address", "redis"), attribute.Int("server.port", 6379), attribute.String("db.operation.name", "SET GET")))
		item, _ := strconv.Atoi(r.URL.Query().Get("item"))
		if item < 0 {
			item = 0
		}
		key := fmt.Sprintf("p9:%s:item:%d", s.cfg.Run, item%64)
		value := rid
		err := s.db.Set(dctx, key, value, 120*time.Second).Err()
		if err == nil {
			var actual string
			actual, err = s.db.Get(dctx, key).Result()
			if err == nil && actual != value {
				err = fmt.Errorf("real Redis roundtrip mismatch")
			}
			payload["redis_value"] = actual
		}
		if mode == "dependency" && err == nil {
			_, waitSpan := s.t.tracer.Start(dctx, "Redis BLPOP wait", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("db.system", "redis"), attribute.String("server.address", "redis"), attribute.String("db.operation.name", "BLPOP")))
			waitErr := s.db.Do(dctx, "BLPOP", "p9:"+s.cfg.Run+":empty", 0.6).Err()
			if waitErr != nil && waitErr != redis.Nil {
				err = waitErr
			}
			waitSpan.End()
		}
		if err != nil {
			status = 503
			dbSpan.SetStatus(codes.Error, "Redis operation failed")
		}
		dbSpan.End()
		if mode == "delay" {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case <-timer.C:
			case <-r.Context().Done():
				timer.Stop()
			}
		}
		if mode == "errors" {
			status = 503
			payload["error"] = "controlled business failure"
		}
	}
	elapsed := time.Since(started).Seconds()
	attrs := metric.WithAttributes(attribute.String("http_route", path), attribute.String("http_method", "GET"))
	s.t.requests.Add(ctx, 1, attrs)
	failed := int64(0)
	if status >= 500 {
		failed = 1
		span.SetStatus(codes.Error, "HTTP request failed")
	}
	s.t.failures.Add(ctx, failed, attrs)
	s.t.duration.Record(ctx, elapsed, attrs)
	span.SetAttributes(attribute.Int("http.response.status_code", status), attribute.String("plan9.request.id", rid), attribute.String("plan9.scenario", mode))
	logFields := map[string]any{"run": s.cfg.Run, "request_id": rid, "trace_id": span.SpanContext().TraceID().String(), "service": s.name, "route": path, "status": status, "duration_seconds": elapsed, "scenario": mode}
	line, _ := json.Marshal(logFields)
	var record logapi.Record
	record.SetTimestamp(time.Now())
	record.SetBody(logapi.StringValue(string(line)))
	record.SetSeverity(logapi.SeverityInfo)
	if failed == 1 {
		record.SetSeverity(logapi.SeverityError)
	}
	record.AddAttributes(logapi.String("plan9.run", s.cfg.Run), logapi.String("request.id", rid))
	s.t.logger.Emit(ctx, record)
	fmt.Println(string(line))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
func main() {
	file := flag.String("config", "/var/lib/plan9/private/sdk.json", "private SDK configuration")
	name := flag.String("service", "orders-gateway", "service identity")
	flag.Parse()
	raw, err := os.ReadFile(*file)
	var cfg configuration
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.Key == "" || cfg.Endpoint == "" || (*name != "orders-gateway" && *name != "orders-worker") {
		fmt.Fprintln(os.Stderr, "invalid private SDK configuration")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	t, err := instrument(ctx, cfg, *name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "SDK startup failed")
		os.Exit(2)
	}
	db := redis.NewClient(&redis.Options{Addr: "redis:6379", Username: "app", Password: cfg.RedisPassword, DB: 0, DialTimeout: 3 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, MaxRetries: 1})
	s := &service{name: *name, cfg: cfg, t: t, db: db, http: &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{Proxy: nil}}}
	port := 18080
	if *name == "orders-worker" {
		port = 18081
	}
	server := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", port), Handler: http.HandlerFunc(s.request), ReadHeaderTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			cancel()
		}
	}()
	<-ctx.Done()
	stop, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	server.Shutdown(stop)
	t.shutdown(stop)
	db.Close()
}
