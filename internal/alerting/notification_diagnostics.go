package alerting

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kakj-go/Argus/internal/runtime"
)

func notificationCredentialError(stage string, task runtime.Task, batch deliveryBatch, cause error) error {
	sqlstate := ""
	var pg *pgconn.PgError
	if errors.As(cause, &pg) {
		sqlstate = pg.Code
	}
	code := "NOTIFICATION_CREDENTIAL_" + strings.ToUpper(stage) + "_FAILED"
	// Transient retries otherwise overwrite the task's last error. Record only
	// safe identifiers/classification, never a credential, URL or broker body.
	slog.Warn("notification credential stage failed", "stage", stage, "sqlstate", sqlstate, "error_code", code, "task_id", task.ID, "batch_id", batch.Persistent.ID)
	return runtime.Error{ErrorCode: code, Cause: fmt.Errorf("notification credential %s: %w", stage, cause)}
}
