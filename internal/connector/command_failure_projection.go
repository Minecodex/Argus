package connector

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"math"
)

// Malformed or unrelated command references still terminate as commands, but
// cannot project an outcome onto a Collector outside their trusted tenant.
func finalizeCollectorCommandFailure(ctx context.Context, q *db.Queries, command db.ConnectorCommand, code string) error {
	var request connectorv1.CollectorManagementCommand
	if protojson.Unmarshal(command.Payload, &request) != nil || request.GetDesiredRevision() == 0 || request.GetDesiredRevision() > math.MaxInt64 {
		return nil
	}
	collector, err := uuid.Parse(request.GetCollectorId())
	if err != nil {
		return nil
	}
	current, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: command.EnterpriseID, ID: collector})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if current.DesiredRevision != int64(request.GetDesiredRevision()) {
		return nil
	}
	if request.GetOperation() != "uninstall" {
		_, err = q.GetCollectorConfigRevision(ctx, db.GetCollectorConfigRevisionParams{CollectorID: collector, Revision: int64(request.GetDesiredRevision())})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	return finalizeCollectorManagement(ctx, q, command, "failed", nil, code)
}
