package telemetry

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Only fixed service-authored stages enter public OTLP diagnostics. Underlying
// SQL errors, names, attributes and payloads remain private.
type admissionStageError struct {
	Stage string
	Cause error
}

func (e *admissionStageError) Error() string { return "TELEMETRY_ADMISSION_REJECTED" }
func (e *admissionStageError) Unwrap() error { return e.Cause }
func admissionPublicReason(err error) string {
	var stage *admissionStageError
	if !errors.As(err, &stage) {
		return ""
	}
	prefix := ""
	switch stage.Stage {
	case "transport_identity":
		prefix = "TRANSPORT_IDENTITY"
	case "forwarded_identity":
		prefix = "FORWARDED_IDENTITY"
	case "application_identity":
		prefix = "APPLICATION_IDENTITY"
	case "source":
		prefix = "SOURCE"
	case "namespace_uid":
		prefix = "NAMESPACE_UID"
	case "namespace_object":
		prefix = "NAMESPACE_OBJECT"
	default:
		return ""
	}
	if errors.Is(err, ErrSourceAwaitingApply) {
		return prefix + "_METADATA_PENDING"
	}
	if errors.Is(err, ErrUnavailable) {
		return prefix + "_CONTROL_UNAVAILABLE"
	}
	return prefix + "_REJECTED"
}

var ErrSourceAwaitingApply = errors.New("telemetry source is awaiting configuration apply")

func controlReadError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errIngestRejected
	}
	return ErrUnavailable
}

func retryableAdmission(err error) bool {
	return errors.Is(err, ErrUnavailable) || errors.Is(err, ErrSourceAwaitingApply)
}
func admissionRPCError(err error, message string) error {
	if retryableAdmission(err) {
		message := "telemetry admission temporarily unavailable"
		if reason := admissionPublicReason(err); reason != "" {
			message += ": " + reason
		}
		return status.Error(codes.Unavailable, message)
	}
	return status.Error(codes.PermissionDenied, message)
}
func admissionHTTPError(w http.ResponseWriter, err error, message string, fallback int) {
	if retryableAdmission(err) {
		w.Header().Set("Retry-After", "1")
		message := "telemetry admission temporarily unavailable"
		if reason := admissionPublicReason(err); reason != "" {
			message += ": " + reason
		}
		http.Error(w, message, http.StatusServiceUnavailable)
		return
	}
	http.Error(w, message, fallback)
}
