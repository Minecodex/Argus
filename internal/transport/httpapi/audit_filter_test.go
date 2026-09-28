package httpapi

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	auditapi "github.com/kakj-go/Argus/internal/gen/openapi/audit"
	"github.com/kakj-go/Argus/internal/pagination"
)

func TestAuditPageBindsAllConditionsAndBoundsDatabaseReads(t *testing.T) {
	at := time.Now().UTC()
	end := at.Add(time.Hour)
	action, actor, kind, id, search := "dashboard.published", "editor", "dashboard", uuid.NewString(), "overview"
	status := auditapi.ListAuditEventsParamsResult("success")
	limit := 2
	p := auditapi.ListAuditEventsParams{Action: &action, ActorId: &actor, ResourceType: &kind, ResourceId: &id, Result: &status, Query: &search, From: &at, To: &end, Limit: &limit}
	binding := pagination.Binding{Audience: "enterprise", EnterpriseID: uuid.NewString(), SubjectType: "enterprise_user", SubjectID: uuid.NewString(), AuthorizationVersion: 4, FilterHash: pagination.HashFilter(auditFilters(p)), Sort: "created_at_desc"}
	signer := pagination.Signer{Key: []byte("01234567890123456789012345678901")}
	q, err := auditPageParams(p, binding, signer)
	if err != nil || q.PageSize != 3 || !q.EnterpriseID.Valid || q.Action.String != action || q.ActorID.String != actor || q.ResourceType.String != kind || q.ResourceID.String != id || q.Result.String != "success" || q.Search.String != search || !q.Since.Time.Equal(at) || !q.Until.Time.Equal(end) {
		t.Fatalf("query lost filtering/bound: %+v %v", q, err)
	}
	last := uuid.NewString()
	cursor, err := signer.Encode(binding, pagination.Position{ID: last, Time: at})
	if err != nil {
		t.Fatal(err)
	}
	p.Cursor = &cursor
	q, err = auditPageParams(p, binding, signer)
	if err != nil || !q.BeforeID.Valid || q.BeforeID.UUID.String() != last || !q.BeforeTime.Time.Equal(at) {
		t.Fatal("cursor position not sent to SQL")
	}
	for _, change := range []func(*auditapi.ListAuditEventsParams){
		func(p *auditapi.ListAuditEventsParams) { v := "other"; p.ActorId = &v }, func(p *auditapi.ListAuditEventsParams) { v := "other"; p.ResourceType = &v }, func(p *auditapi.ListAuditEventsParams) { v := "other"; p.ResourceId = &v }, func(p *auditapi.ListAuditEventsParams) { v := "dashboard.archived"; p.Action = &v }, func(p *auditapi.ListAuditEventsParams) {
			v := auditapi.ListAuditEventsParamsResult("denied")
			p.Result = &v
		}, func(p *auditapi.ListAuditEventsParams) { v := "missing"; p.Query = &v }, func(p *auditapi.ListAuditEventsParams) { v := at.Add(time.Second); p.From = &v }, func(p *auditapi.ListAuditEventsParams) { p.To = &at },
	} {
		changed := p
		change(&changed)
		scoped := binding
		scoped.FilterHash = pagination.HashFilter(auditFilters(changed))
		if _, err := auditPageParams(changed, scoped, signer); !errors.Is(err, pagination.ErrInvalid) {
			t.Fatal("cursor accepted changed filter")
		}
	}
	binding.AuthorizationVersion++
	if _, err := auditPageParams(p, binding, signer); !errors.Is(err, pagination.ErrAuthorizationVersionStale) {
		t.Fatal("stale authorization cursor accepted")
	}
}
