package telemetry

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

type ingestRateLimitError struct{ RetryAfter time.Duration }

func (e *ingestRateLimitError) Error() string { return "INGEST_RATE_LIMITED" }

func ingestQuotaStatus(err error) (*status.Status, bool) {
	var limited *ingestRateLimitError
	if !errors.As(err, &limited) {
		return nil, false
	}
	delay := limited.RetryAfter
	if delay < time.Second {
		delay = time.Second
	}
	if delay > time.Minute {
		delay = time.Minute
	}
	result, e := status.New(codes.ResourceExhausted, "INGEST_RATE_LIMITED").WithDetails(&errdetails.ErrorInfo{Reason: "INGEST_RATE_LIMITED", Domain: "argus.telemetry", Metadata: map[string]string{"request_limit": "1200", "window_seconds": "60"}}, &errdetails.RetryInfo{RetryDelay: durationpb.New(delay)})
	if e != nil {
		return status.New(codes.Unavailable, "INGEST_RATE_LIMITED"), true
	}
	return result, true
}

func ingestPublishRPCError(err error) error {
	if result, limited := ingestQuotaStatus(err); limited {
		return result.Err()
	}
	return status.Error(codes.Unavailable, "telemetry ingest unavailable")
}

func ingestPublishHTTPError(writer http.ResponseWriter, err error) {
	if result, limited := ingestQuotaStatus(err); limited {
		delay := time.Second
		for _, detail := range result.Details() {
			if value, ok := detail.(*errdetails.RetryInfo); ok {
				delay = value.GetRetryDelay().AsDuration()
			}
		}
		writer.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(delay.Seconds()))))
		writer.Header().Set("Content-Type", "application/x-protobuf")
		writer.WriteHeader(http.StatusTooManyRequests)
		body, _ := proto.Marshal(result.Proto())
		_, _ = writer.Write(body)
		return
	}
	http.Error(writer, "telemetry ingest unavailable", http.StatusServiceUnavailable)
}
