package kubernetesworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"time"
)

type Operation struct {
	ID              uuid.UUID       `json:"id"`
	ClusterID       uuid.UUID       `json:"cluster_id"`
	NamespaceID     *uuid.UUID      `json:"namespace_id,omitempty"`
	NamespaceUID    string          `json:"namespace_uid"`
	Status          string          `json:"status"`
	Operation       string          `json:"operation"`
	Type            string          `json:"type"`
	Name            string          `json:"name"`
	UID             string          `json:"uid"`
	ErrorCode       string          `json:"error_code"`
	Version         int64           `json:"version"`
	Result          json.RawMessage `json:"result"`
	PendingActionID *uuid.UUID      `json:"pending_action_id,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	FinishedAt      *time.Time      `json:"finished_at,omitempty"`
}

func operationDTO(row db.KubernetesOperation) Operation {
	out := Operation{ID: row.ID, ClusterID: row.ClusterID, NamespaceUID: row.NamespaceUid, Status: row.Status, Operation: row.Operation, Type: row.TypeKey, Name: row.TargetName, UID: row.TargetUid, ErrorCode: row.ErrorCode, Version: row.Version, Result: row.Result, CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
	if row.NamespaceID.Valid {
		out.NamespaceID = &row.NamespaceID.UUID
	}
	if row.PendingActionID.Valid {
		out.PendingActionID = &row.PendingActionID.UUID
	}
	if row.StartedAt.Valid {
		out.StartedAt = &row.StartedAt.Time
	}
	if row.FinishedAt.Valid {
		out.FinishedAt = &row.FinishedAt.Time
	}
	return out
}
func (s Service) GetOperation(ctx context.Context, a Actor, clusterID, id uuid.UUID) (Operation, error) {
	_, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return Operation{}, err
	}
	row, err := s.Store.Queries.GetKubernetesOperation(ctx, db.GetKubernetesOperationParams{EnterpriseID: a.EnterpriseID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Operation{}, authorization.ErrAccessDenied
	}
	if err != nil {
		return Operation{}, err
	}
	if row.ClusterID != clusterID {
		return Operation{}, authorization.ErrAccessDenied
	}
	if !scope.ClusterWide {
		allowed := false
		for _, ns := range scope.Namespaces {
			if row.NamespaceID.Valid && row.NamespaceID.UUID == ns.ID && row.NamespaceUid == ns.UID {
				allowed = true
			}
		}
		if !allowed {
			return Operation{}, authorization.ErrAccessDenied
		}
	}
	return operationDTO(row), nil
}

type OperationPage struct {
	Items      []Operation `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

func (s Service) ListOperations(ctx context.Context, a Actor, clusterID uuid.UUID, limit int, cursor string) (OperationPage, error) {
	out := OperationPage{Items: []Operation{}}
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 200 {
		return out, kubernetesinventory.ErrInvalid
	}
	_, scope, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return out, err
	}
	ids := []uuid.UUID{}
	for _, ns := range scope.Namespaces {
		ids = append(ids, ns.ID)
	}
	binding := pagination.Binding{Audience: "kubernetes_operations/v1", EnterpriseID: a.EnterpriseID.String(), SubjectID: a.SubjectID.String(), SubjectType: a.SubjectType, AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter(scope), Sort: "id"}
	signer := pagination.Signer{Key: s.CursorKey}
	after := uuid.Nil
	if cursor != "" {
		position, e := signer.Decode(cursor, binding)
		if e != nil {
			return out, e
		}
		after, e = uuid.Parse(position.ID)
		if e != nil {
			return out, pagination.ErrInvalid
		}
	}
	rows, err := s.Store.Queries.ListKubernetesOperations(ctx, db.ListKubernetesOperationsParams{EnterpriseID: a.EnterpriseID, ClusterID: clusterID, ClusterWide: scope.ClusterWide, NamespaceIds: ids, AfterID: after, PageLimit: int32(limit + 1)})
	if err != nil {
		return out, err
	}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor, err = signer.Encode(binding, pagination.Position{ID: rows[len(rows)-1].ID.String()})
		if err != nil {
			return out, err
		}
	}
	for _, row := range rows {
		out.Items = append(out.Items, operationDTO(row))
	}
	return out, nil
}
