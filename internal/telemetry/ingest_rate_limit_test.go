package telemetry

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIngestQuotaProvidesBoundedRetryAcrossTransports(t *testing.T) {
	for _, delay := range []time.Duration{-time.Second, 1500 * time.Millisecond, 3 * time.Minute} {
		err := fmt.Errorf("private backend details: %w", &ingestRateLimitError{RetryAfter: delay})
		rpc := status.Convert(ingestPublishRPCError(err))
		if rpc.Code() != codes.ResourceExhausted || rpc.Message() != "INGEST_RATE_LIMITED" {
			t.Fatal("quota was not retryable without leaking backend details", rpc)
		}
		retry, reason := false, false
		for _, detail := range rpc.Details() {
			switch d := detail.(type) {
			case *errdetails.RetryInfo:
				retry = d.RetryDelay.AsDuration() >= time.Second && d.RetryDelay.AsDuration() <= time.Minute
			case *errdetails.ErrorInfo:
				reason = d.Reason == "INGEST_RATE_LIMITED" && d.Domain == "argus.telemetry"
			}
		}
		w := httptest.NewRecorder()
		ingestPublishHTTPError(w, err)
		seconds, e := strconv.Atoi(w.Header().Get("Retry-After"))
		if !retry || !reason || w.Code != http.StatusTooManyRequests || e != nil || seconds < 1 || seconds > 60 {
			t.Fatal("gRPC and HTTP quota responses lost retry guidance")
		}
	}
	plain := errors.New("private broker address")
	if rpc := status.Convert(ingestPublishRPCError(plain)); rpc.Code() != codes.Unavailable || rpc.Message() != "telemetry ingest unavailable" {
		t.Fatal("ordinary backend failure changed public semantics")
	}
	w := httptest.NewRecorder()
	ingestPublishHTTPError(w, plain)
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "" {
		t.Fatal("ordinary backend failure was mislabeled quota")
	}
}
