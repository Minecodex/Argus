package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/kakj-go/Argus/internal/storage/postgres"
)

func TestPlanV5IdempotencyConflictHasPublicConflictContract(t *testing.T) {
	err := fmt.Errorf("Workspace operation: %w", postgres.ErrIdempotencyConflict)
	if status := planV5Status(err); status != http.StatusConflict {
		t.Fatalf("idempotency conflict became %d", status)
	}
	value := planV5Error[map[string]any](context.Background(), err)
	if value["code"] != "IDEMPOTENCY_CONFLICT" || value["message_key"] != "errors.common.idempotency_conflict" {
		t.Fatalf("bad public conflict: %v", value)
	}
}
