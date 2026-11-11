package telemetry

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPermanentAdmissionReasonPreservesDenialAndPrivacy(t *testing.T) {
	err := &admissionStageError{Stage: "source", Cause: errors.Join(errIngestRejected, errors.New("password=private-canary"))}
	rpc := admissionRPCError(err, "rejected")
	if status.Code(rpc) != codes.PermissionDenied || !strings.Contains(rpc.Error(), "SOURCE_REJECTED") || strings.Contains(rpc.Error(), "private-canary") {
		t.Fatal("permanent admission stage was hidden or leaked private facts")
	}
	w := httptest.NewRecorder()
	admissionHTTPError(w, err, "rejected", 403)
	if w.Code != 403 || w.Header().Get("Retry-After") != "" || !strings.Contains(w.Body.String(), "SOURCE_REJECTED") || strings.Contains(w.Body.String(), "private-canary") {
		t.Fatal("permanent HTTP admission changed retry or privacy boundary")
	}
}
