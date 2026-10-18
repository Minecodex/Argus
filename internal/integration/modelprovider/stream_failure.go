package modelprovider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"reflect"
)

// Preserve errors.Is/As while separating transport reads, protocol decoding and
// the caller's persistence/lease callback. None of these labels contain data.
type streamStageError struct {
	stage string
	cause error
}

func (e *streamStageError) Error() string { return e.cause.Error() }
func (e *streamStageError) Unwrap() error { return e.cause }

func streamFailureOrigin(err error) string {
	var staged *streamStageError
	if errors.As(err, &staged) {
		return staged.stage
	}
	return "request_or_protocol"
}

func streamFailureType(err error) string {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		if dns.IsTimeout {
			return "dns_timeout"
		}
		if dns.IsNotFound {
			return "dns_not_found"
		}
		return "dns_error"
	}
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		switch reflect.TypeOf(cause).String() {
		case "http.http2StreamError", "http2.StreamError":
			return "http2_stream_error"
		case "http.http2ConnectionError", "http2.ConnectionError":
			return "http2_connection_error"
		}
	}
	var state interface{ SQLState() string }
	if errors.As(err, &state) {
		switch state.SQLState() {
		case "40001":
			return "database_serialization"
		case "40P01":
			return "database_deadlock"
		case "57014":
			return "database_cancelled"
		}
		return "database_error"
	}
	return "other"
}

// Only bounded classes enter logs. Error text can contain endpoints, credentials
// or provider content and must never be used as a label or diagnostic value.
func streamFailureKind(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, ErrIncompleteStream):
		return "incomplete_stream"
	case errors.Is(err, ErrInvalidUsage):
		return "invalid_usage"
	case errors.Is(err, ErrBudgetNotEnforced):
		return "output_budget_not_enforced"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected_eof"
	}
	var syntax *json.SyntaxError
	var shape *json.UnmarshalTypeError
	var network net.Error
	if errors.As(err, &syntax) {
		return "invalid_json"
	}
	if errors.As(err, &shape) {
		return "invalid_json_type"
	}
	if errors.As(err, &network) {
		return "transport_error"
	}
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		switch cause.Error() {
		case "model tool batch exceeds limit":
			return "tool_batch_limit"
		case "model tool call exceeds size limit":
			return "tool_size_limit"
		case "model returned an incomplete tool call":
			return "incomplete_tool_call"
		case "model tool call lacks identity":
			return "tool_id_missing"
		case "model tool call lacks name":
			return "tool_name_missing"
		case "model provider reported an error":
			return "provider_error"
		case "multiple model choices are unsupported", "unexpected model choice index":
			return "invalid_choices"
		case "model content after terminal response", "model event after terminal response":
			return "data_after_terminal"
		case "duplicate model terminal response":
			return "duplicate_terminal"
		case "model refused the request", "model response failed or was refused":
			return "refused_or_failed"
		case "negative model tool index", "model tool identity changed", "model tool delta lacks identity":
			return "invalid_tool_identity"
		case "model tool calls lack tool_calls finish reason":
			return "invalid_tool_finish_reason"
		case "invalid completed model status":
			return "invalid_completion_status"
		case "unsupported model response output item", "unsupported model response event":
			return "unsupported_response_event"
		}
	}
	return "unclassified_stream_or_sink_error"
}
