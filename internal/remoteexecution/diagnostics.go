package remoteexecution

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func logPreparationFailure(ctx context.Context, request, step uuid.UUID, code string, err error) {
	fields := []any{"request_id", request, "step_id", step, "error_code", code, "cause_type", fmt.Sprintf("%T", err)}
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) {
		// Never log Detail, query text, command payloads or credential material.
		fields = append(fields, "sqlstate", databaseError.Code, "table", databaseError.TableName, "constraint", databaseError.ConstraintName, "column", databaseError.ColumnName)
	}
	slog.WarnContext(ctx, "Remote execution preparation failed", fields...)
}
