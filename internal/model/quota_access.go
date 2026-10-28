package model

import (
	"context"
	"math"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/modelmetrics"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (s Service) ListAuthorizedQuotas(ctx context.Context, actor authorization.Actor) ([]db.ModelQuota, error) {
	department, err := modelmetrics.AuthorizePermission(ctx, s.Store, actor, "model.quota.manage")
	if err != nil {
		return nil, err
	}
	rows, err := s.ListQuotas(ctx, actor.EnterpriseID)
	if err != nil {
		return nil, err
	}
	if department == uuid.Nil {
		return rows, nil
	}
	result := []db.ModelQuota{}
	for _, quota := range rows {
		if quota.SubjectType == "department" && quota.SubjectID == department {
			result = append(result, quota)
		}
		if quota.SubjectType == "user" {
			u, e := s.Store.Queries.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: actor.EnterpriseID, ID: quota.SubjectID})
			if e == nil && u.DepartmentID == department {
				result = append(result, quota)
			}
		}
	}
	return result, nil
}

func checkQuotaWrite(ctx context.Context, q *db.Queries, actor authorization.Actor, modelID uuid.UUID, kind string, subject uuid.UUID, amount float64) error {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || kind != "user" && kind != "department" {
		return modelmetrics.ErrInvalid
	}
	if err := authorization.Authorize(ctx, q, actor, "model.quota.manage"); err != nil {
		return modelmetrics.ErrDenied
	}
	if _, err := q.GetAIModel(ctx, db.GetAIModelParams{EnterpriseID: actor.EnterpriseID, ID: modelID}); err != nil {
		return modelmetrics.ErrDenied
	}
	admin := authorization.Authorize(ctx, q, actor, "model.manage") == nil
	var department uuid.UUID
	if kind == "user" {
		user, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: actor.EnterpriseID, ID: subject})
		if err != nil {
			return modelmetrics.ErrDenied
		}
		department = user.DepartmentID
		if !admin {
			current, err := q.GetEnterpriseUser(ctx, db.GetEnterpriseUserParams{EnterpriseID: actor.EnterpriseID, ID: actor.SubjectID})
			if err != nil || current.DepartmentID != department {
				return modelmetrics.ErrDenied
			}
		}
		quotas, err := q.ListModelQuotas(ctx, actor.EnterpriseID)
		if err != nil {
			return err
		}
		for _, quota := range quotas {
			if quota.ModelID == modelID && quota.SubjectType == "department" && quota.SubjectID == department {
				limit, err := quota.MonthlyAmount.Float64Value()
				if err != nil {
					return err
				}
				if limit.Valid && amount > limit.Float64 {
					return ErrQuotaExceeded
				}
			}
		}
	} else {
		if !admin {
			return modelmetrics.ErrDenied
		}
		if _, err := q.GetDepartment(ctx, db.GetDepartmentParams{EnterpriseID: actor.EnterpriseID, ID: subject}); err != nil {
			return modelmetrics.ErrDenied
		}
	}
	return nil
}
