package modelmetrics

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
)

type Service struct {
	Store *postgres.Store
	Conn  driver.Conn
}
type dataset struct {
	Calls        []Call
	Quotas       []Quota
	Reservations []Reservation
	Names        map[string]string
	Response     Response
}

func (s Service) read(ctx context.Context, r Request, a Access) (dataset, error) {
	d := dataset{Names: map[string]string{}, Response: Response{From: a.From, To: a.To, Timezone: a.Timezone, Models: []Model{}, Panels: []PanelResult{}, UsageComplete: true, ScopeHash: a.Hash}}
	rows, err := s.Store.Pool.Query(ctx, `SELECT id,name FROM ai_models WHERE enterprise_id=$1 ORDER BY name,id LIMIT 1001`, r.Actor.EnterpriseID)
	if err != nil {
		return d, err
	}
	for rows.Next() {
		var m Model
		if err = rows.Scan(&m.ID, &m.Name); err != nil {
			rows.Close()
			return d, err
		}
		d.Response.Models = append(d.Response.Models, m)
		d.Names[m.ID.String()] = m.Name
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Response.Models) > 1000 {
		return d, ErrBudget
	}
	// Names are resolved only inside the currently authorized department scope.
	includeSubjects := false
	if r.CandidatesOnly {
		_, e := AuthorizePermission(ctx, s.Store, r.Actor, "model.quota.manage")
		if e != nil && !errors.Is(e, ErrDenied) {
			return d, e
		}
		includeSubjects = e == nil
	}
	rows, err = s.Store.Pool.Query(ctx, `SELECT id,name,'department',id FROM departments WHERE enterprise_id=$1 AND ($2::uuid IS NULL OR id=$2) UNION ALL SELECT id,display_name,'user',department_id FROM enterprise_users WHERE enterprise_id=$1 AND ($2::uuid IS NULL OR department_id=$2) LIMIT 10001`, r.Actor.EnterpriseID, nullableID(a.DepartmentID))
	if err != nil {
		return d, err
	}
	subjectCount := 0
	for rows.Next() {
		var subject Subject
		if err = rows.Scan(&subject.ID, &subject.Name, &subject.Kind, &subject.DepartmentID); err != nil {
			rows.Close()
			return d, err
		}
		d.Names[subject.ID.String()] = subject.Name
		subjectCount++
		if includeSubjects {
			d.Response.Subjects = append(d.Response.Subjects, subject)
		}
	}
	err = rows.Err()
	rows.Close()
	if subjectCount > 10000 {
		return d, ErrBudget
	}
	if err != nil {
		return d, err
	}
	err = s.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM model_metric_outbox WHERE enterprise_id=$1 AND delivered_at IS NULL AND ($2::uuid IS NULL OR department_id=$2 OR (kind='quota' AND payload->>'subject_type'='user' AND payload->>'subject_id' IN (SELECT id::text FROM enterprise_users WHERE enterprise_id=$1 AND department_id=$2)))`, r.Actor.EnterpriseID, nullableID(a.DepartmentID)).Scan(&d.Response.Pending)
	if err != nil {
		return d, err
	}
	if r.CandidatesOnly || r.ModelMode == "explicit" && len(r.ModelIDs) == 0 {
		return d, nil
	}
	if s.Conn == nil {
		return d, ErrUnavailable
	}
	ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{"max_execution_time": 15, "max_bytes_to_read": 256 << 20, "max_result_rows": 100001, "result_overflow_mode": "throw"}))
	where := `enterprise_id=? AND completed_at>=? AND completed_at<?`
	args := []any{r.Actor.EnterpriseID, a.From, a.To}
	if a.DepartmentID != uuid.Nil {
		where += ` AND department_id=?`
		args = append(args, a.DepartmentID)
	}
	if r.ModelMode == "explicit" {
		where += ` AND model_id IN (?)`
		args = append(args, r.ModelIDs)
	}
	chrows, err := s.Conn.Query(ctx, `SELECT payload,projected_at FROM `+callsTable+` FINAL WHERE `+where+` ORDER BY completed_at,id LIMIT 100001`, args...)
	if err != nil {
		return d, err
	}
	for chrows.Next() {
		var raw string
		var projected time.Time
		if err = chrows.Scan(&raw, &projected); err != nil {
			chrows.Close()
			return d, err
		}
		var c Call
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			chrows.Close()
			return d, err
		}
		d.Calls = append(d.Calls, c)
		if d.Response.ProjectedAt == nil || projected.After(*d.Response.ProjectedAt) {
			v := projected
			d.Response.ProjectedAt = &v
		}
	}
	err = chrows.Err()
	chrows.Close()
	if err != nil {
		return d, err
	}
	if len(d.Calls) > 100000 {
		return d, ErrBudget
	}
	// Quotas show current policy, explicitly labelled as current even for history.
	quotaWhere := `enterprise_id=?`
	quotaArgs := []any{r.Actor.EnterpriseID}
	if a.DepartmentID != uuid.Nil {
		quotaWhere += ` AND subject_type='department' AND subject_id=?`
		quotaArgs = append(quotaArgs, a.DepartmentID)
	}
	if r.ModelMode == "explicit" {
		quotaWhere += ` AND model_id IN (?)`
		quotaArgs = append(quotaArgs, r.ModelIDs)
	}
	chrows, err = s.Conn.Query(ctx, `SELECT argMax(payload,version),max(projected_at) FROM `+quotasTable+` FINAL WHERE `+quotaWhere+` GROUP BY id LIMIT 10001`, quotaArgs...)
	if err != nil {
		return d, err
	}
	quotaRows := 0
	for chrows.Next() {
		var raw string
		var projected time.Time
		if err = chrows.Scan(&raw, &projected); err != nil {
			chrows.Close()
			return d, err
		}
		quotaRows++
		d.observeProjection(projected)
		var q Quota
		if err = json.Unmarshal([]byte(raw), &q); err != nil {
			chrows.Close()
			return d, err
		}
		if !q.Deleted {
			d.Quotas = append(d.Quotas, q)
		}
	}
	err = chrows.Err()
	chrows.Close()
	if quotaRows > 10000 {
		return d, ErrBudget
	}
	if err != nil {
		return d, err
	}
	month := a.From.In(mustMetricLocation(a.Timezone)).Format("2006-01")
	where = `enterprise_id=? AND month=?`
	args = []any{r.Actor.EnterpriseID, month}
	if a.DepartmentID != uuid.Nil {
		where += ` AND department_id=?`
		args = append(args, a.DepartmentID)
	}
	if r.ModelMode == "explicit" {
		where += ` AND model_id IN (?)`
		args = append(args, r.ModelIDs)
	}
	chrows, err = s.Conn.Query(ctx, `SELECT payload,projected_at FROM `+reservationsTable+` FINAL WHERE `+where+` LIMIT 100001`, args...)
	if err != nil {
		return d, err
	}
	for chrows.Next() {
		var raw string
		var projected time.Time
		if err = chrows.Scan(&raw, &projected); err != nil {
			chrows.Close()
			return d, err
		}
		d.observeProjection(projected)
		var reservation Reservation
		if err = json.Unmarshal([]byte(raw), &reservation); err != nil {
			chrows.Close()
			return d, err
		}
		d.Reservations = append(d.Reservations, reservation)
	}
	err = chrows.Err()
	chrows.Close()
	if len(d.Reservations) > 100000 {
		return d, ErrBudget
	}
	return d, err
}
func (d *dataset) observeProjection(at time.Time) {
	if d.Response.ProjectedAt == nil || at.After(*d.Response.ProjectedAt) {
		v := at
		d.Response.ProjectedAt = &v
	}
}
func mustMetricLocation(name string) *time.Location {
	location, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return location
}
func nullableID(id uuid.UUID) any {
	if id == uuid.Nil {
		return nil
	}
	return id
}
