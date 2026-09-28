package httpapi

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	auditapi "github.com/kakj-go/Argus/internal/gen/openapi/audit"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func auditPageParams(p auditapi.ListAuditEventsParams, binding pagination.Binding, signer pagination.Signer) (db.ListAuditEventsPageParams, error) {
	limit := listLimit(p.Limit)
	if limit < 1 || limit > 200 {
		return db.ListAuditEventsPageParams{}, pagination.ErrInvalid
	}
	q := db.ListAuditEventsPageParams{Domain: binding.Audience, Action: auditText(p.Action), ActorID: auditText(p.ActorId), ResourceType: auditText(p.ResourceType), ResourceID: auditText(p.ResourceId), Result: auditText(p.Result), Search: auditText(p.Query), Since: auditTime(p.From), Until: auditTime(p.To), PageSize: int32(limit + 1)}
	if binding.Audience == "enterprise" {
		id, err := uuid.Parse(binding.EnterpriseID)
		if err != nil {
			return q, pagination.ErrInvalid
		}
		q.EnterpriseID = uuid.NullUUID{UUID: id, Valid: true}
	}
	if cursor := cursorValue(p.Cursor); cursor != "" {
		position, err := signer.Decode(cursor, binding)
		if err != nil {
			return q, err
		}
		id, err := uuid.Parse(position.ID)
		if err != nil {
			return q, pagination.ErrInvalid
		}
		q.BeforeID, q.BeforeTime = uuid.NullUUID{UUID: id, Valid: true}, pgtype.Timestamptz{Time: position.Time, Valid: true}
	}
	return q, nil
}

func auditText[T ~string](v *T) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*v), Valid: true}
}
func auditTime(v *time.Time) pgtype.Timestamptz {
	if v == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: v.UTC(), Valid: true}
}
