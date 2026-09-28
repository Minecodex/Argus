package selfmonitor

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func HTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz", "/readyz", "/metrics":
			next.ServeHTTP(w, r)
			return
		}
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		method := r.Method
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE":
		default:
			method = "_OTHER"
		}
		ctx, span := otel.Tracer("argus.http").Start(ctx, "HTTP "+method, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			// Use the registered template after routing, never URL/query/body/headers.
			route := "unmatched"
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			span.SetName(method + " " + route)
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			span.SetAttributes(attribute.String("http.request.method", method), attribute.String("http.route", route), attribute.Int("http.response.status_code", status))
			if status >= 500 {
				span.SetStatus(codes.Error, "HTTP server error")
			}
		}()
		next.ServeHTTP(wrapped, r.WithContext(ctx))
	})
}
