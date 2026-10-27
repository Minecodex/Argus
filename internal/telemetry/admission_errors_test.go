package telemetry

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTransientAdmissionRetainsOTLPRetrySemantics(t *testing.T) {
	for _, err := range []error{ErrUnavailable, ErrSourceAwaitingApply} {
		if status.Code(admissionRPCError(err, "permanent rejection")) != codes.Unavailable {
			t.Fatal("transient failure cannot be retried")
		}
		writer := httptest.NewRecorder()
		admissionHTTPError(writer, err, "private-control-canary", 400)
		if writer.Code != 503 || writer.Header().Get("Retry-After") == "" || strings.Contains(writer.Body.String(), "canary") {
			t.Fatal("HTTP retry or diagnostic privacy lost")
		}
	}
	if status.Code(admissionRPCError(errIngestRejected, "rejected")) != codes.PermissionDenied {
		t.Fatal("untrusted source became retryable admission")
	}
	if !errors.Is(controlReadError(pgx.ErrNoRows), errIngestRejected) || !errors.Is(controlReadError(errors.New("closed connection")), ErrUnavailable) {
		t.Fatal("control read error classification lost")
	}
}
