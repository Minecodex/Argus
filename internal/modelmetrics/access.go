package modelmetrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type Access struct {
	DepartmentID uuid.UUID
	Timezone     string
	From, To     time.Time
	Hash         string
}

func Authorize(ctx context.Context, store *postgres.Store, actor authorization.Actor) (uuid.UUID, error) {
	return AuthorizePermission(ctx, store, actor, "model.usage.read")
}
func AuthorizePermission(ctx context.Context, store *postgres.Store, actor authorization.Actor, permission string) (uuid.UUID, error) {
	if permission != "model.usage.read" && permission != "model.quota.manage" {
		return uuid.Nil, ErrDenied
	}
	if store == nil || actor.SubjectType != "user" || actor.RunID != uuid.Nil {
		return uuid.Nil, ErrDenied
	}
	var department uuid.UUID
	err := store.Pool.QueryRow(ctx, `SELECT u.department_id FROM enterprise_users u JOIN enterprises e ON e.id=u.enterprise_id JOIN departments d ON d.id=u.department_id AND d.enterprise_id=u.enterprise_id WHERE u.id=$1 AND u.enterprise_id=$2 AND u.authorization_version=$3 AND u.status='active' AND e.status='active' AND d.status='active'`, actor.SubjectID, actor.EnterpriseID, actor.AuthorizationVersion).Scan(&department)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrDenied
		}
		return uuid.Nil, err
	}
	permissions, err := store.Queries.ListEffectiveUserPermissions(ctx, db.ListEffectiveUserPermissionsParams{EnterpriseID: actor.EnterpriseID, UserID: actor.SubjectID, DepartmentID: department})
	if err != nil {
		return uuid.Nil, err
	}
	if !slices.Contains(permissions, "*") && !slices.Contains(permissions, permission) {
		return uuid.Nil, ErrDenied
	}
	if slices.Contains(permissions, "*") || slices.Contains(permissions, "model.manage") {
		return uuid.Nil, nil
	}
	return department, nil
}

func Resolve(ctx context.Context, store *postgres.Store, r Request) (Access, error) {
	if r.TemplateHash != ContractHash() || len(r.ModelIDs) > 100 || r.ModelMode != "all" && r.ModelMode != "explicit" || r.ModelMode == "all" && len(r.ModelIDs) > 0 {
		return Access{}, ErrInvalid
	}
	department, err := Authorize(ctx, store, r.Actor)
	if err != nil {
		return Access{}, err
	}
	enterprise, err := store.Queries.GetEnterprise(ctx, r.Actor.EnterpriseID)
	if err != nil {
		return Access{}, err
	}
	location, err := time.LoadLocation(enterprise.Timezone)
	if err != nil {
		return Access{}, ErrInvalid
	}
	month := r.Month
	if month == "" {
		month = time.Now().In(location).Format("2006-01")
	}
	from, err := time.ParseInLocation("2006-01", month, location)
	if err != nil {
		return Access{}, ErrInvalid
	}
	if from.After(time.Now().In(location)) || from.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, location)) {
		return Access{}, ErrInvalid
	}
	seen := map[uuid.UUID]bool{}
	for _, id := range r.ModelIDs {
		if id == uuid.Nil || seen[id] {
			return Access{}, ErrInvalid
		}
		seen[id] = true
		var exists bool
		err = store.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ai_models WHERE enterprise_id=$1 AND id=$2)`, r.Actor.EnterpriseID, id).Scan(&exists)
		if err != nil {
			return Access{}, err
		}
		if !exists {
			return Access{}, ErrDenied
		}
	}
	a := Access{DepartmentID: department, Timezone: enterprise.Timezone, From: from.UTC(), To: from.AddDate(0, 1, 0).UTC()}
	raw, _ := json.Marshal(struct {
		Actor           authorization.Actor
		Department      uuid.UUID
		Month, Contract string
		Models          []uuid.UUID
		Mode            string
		Timezone        string
		From, To        time.Time
	}{r.Actor, department, month, ContractHash(), r.ModelIDs, r.ModelMode, a.Timezone, a.From, a.To})
	hash := sha256.Sum256(raw)
	a.Hash = hex.EncodeToString(hash[:])
	return a, nil
}
