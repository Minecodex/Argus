package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func pointer[T any](value T) *T { return &value }

func planV5Status(err error) int {
	if errors.Is(err, postgres.ErrIdempotencyConflict) {
		return http.StatusConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return http.StatusNotFound
	}
	var coded interface{ Code() string }
	if !errors.As(err, &coded) {
		return http.StatusInternalServerError
	}
	code := coded.Code()
	switch {
	case strings.Contains(code, "FORBIDDEN"), strings.Contains(code, "AUTHORIZATION"), strings.Contains(code, "PERMISSION"):
		return http.StatusForbidden
	case strings.Contains(code, "NOT_FOUND"):
		return http.StatusNotFound
	case strings.Contains(code, "CONFLICT"), strings.Contains(code, "CHANGED"), strings.Contains(code, "VERSION_UNAVAILABLE"):
		return http.StatusConflict
	case strings.Contains(code, "UNAVAILABLE"), strings.Contains(code, "NOT_CONFIGURED"):
		return http.StatusServiceUnavailable
	default:
		return http.StatusUnprocessableEntity
	}
}

func planV5Error[T any](ctx context.Context, err error) T {
	code := "INTERNAL_ERROR"
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		code = coded.Code()
	}
	if errors.Is(err, pgx.ErrNoRows) {
		code = "RESOURCE_NOT_FOUND"
	}
	id := "server-generated-request"
	if current, ok := RequestFromContext(ctx); ok {
		id = current.RequestID
	}
	body := map[string]any{"code": code, "message_key": "errors.planv5." + strings.ToLower(code), "request_id": id}
	if errors.Is(err, postgres.ErrIdempotencyConflict) {
		code = "IDEMPOTENCY_CONFLICT"
		body["code"] = code
		body["message_key"] = "errors.common.idempotency_conflict"
	}
	var detail toolruntime.Error
	if errors.As(err, &detail) {
		if detail.Message != "" {
			body["message"] = detail.Message
		}
		if detail.Details != nil {
			body["params"] = detail.Details
		}
	}
	data, _ := json.Marshal(body)
	var result T
	_ = json.Unmarshal(data, &result)
	logMappedError(ctx, code, err)
	return result
}
