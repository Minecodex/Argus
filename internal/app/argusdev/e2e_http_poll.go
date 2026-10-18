package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

type scenarioHTTPStatusError struct {
	Status  int
	Message string
}

func (e scenarioHTTPStatusError) Error() string { return e.Message }

// PollJSON is exclusively an idempotent observation. Approval, creation and
// execution still use JSON once; an uncertain mutation must never be retried.
func (h *ScenarioHTTP) PollJSON(ctx context.Context, name, clientName, path string, headers map[string]string) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	observation := uuid.NewString()
	for attempt := 1; ; attempt++ {
		stage := fmt.Sprintf("%s-read-%s-%d", name, observation, attempt)
		value, err := h.JSON(ctx, stage, clientName, http.MethodGet, path, http.StatusOK, nil, headers)
		if err == nil || ctx.Err() != nil || attempt == 3 || !retryableObservation(err) {
			return value, err
		}
		status := 0
		var response scenarioHTTPStatusError
		if errors.As(err, &response) {
			status = response.Status
		}
		// No query parameters, headers or error strings (which may contain
		// credentials) enter the retry record. HTTP failures also retain the
		// normal redacted status evidence under this unique attempt name.
		raw, _ := json.MarshalIndent(map[string]any{"stage": name, "observation_id": observation, "attempt": attempt, "method": http.MethodGet, "status": status, "transport_failure": status == 0, "at": time.Now().UTC()}, "", "  ")
		if e := writePrivate(filepath.Join(h.Artifacts, stage+"-retry.json"), raw); e != nil {
			return nil, errors.Join(err, e)
		}
		if e := waitContext(ctx, time.Duration(attempt)*time.Second); e != nil {
			return nil, errors.Join(err, e)
		}
	}
}

func retryableObservation(err error) bool {
	var response scenarioHTTPStatusError
	if errors.As(err, &response) {
		return response.Status == http.StatusBadGateway || response.Status == http.StatusServiceUnavailable || response.Status == http.StatusGatewayTimeout
	}
	var network net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.As(err, &network) && network.Timeout()
}
