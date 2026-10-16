package alerting

import (
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"strings"
	"testing"
	"time"
)

func TestPostgresAlertSignedPagingFiltersAndResourceRevocation(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	policy := f.createPolicy()
	other := f.createPolicy()
	state, _ := json.Marshal(InstanceState{Name: "CPU", PolicyName: policy.Name, Metric: "cpu_usage", Kind: "condition", NotificationState: "pending", Labels: map[string]string{}, PolicySnapshot: policy.Spec})
	rows := []db.AlertInstance{}
	for i := 0; i < 3; i++ {
		row, err := f.s.Store.Queries.CreateAlertInstance(ctx, db.CreateAlertInstanceParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, PolicyID: policy.ID, PolicyVersion: 1, Generation: 1, Fingerprint: uuid.NewString(), ResourceIds: []uuid.UUID{f.host}, SourceIds: []uuid.UUID{}, State: state, Lifecycle: "firing", Health: "normal", Severity: "critical", StartedAt: timestamp(time.Now()), LastEvaluatedAt: timestamp(time.Now())})
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	page, err := f.s.Instances(ctx, f.a, ListFilter{PolicyID: policy.ID, Limit: 1}, false)
	if err != nil || page.Total != 3 || len(page.Items) != 1 || page.NextCursor == nil || page.Complete {
		t.Fatal(page, err)
	}
	if strings.Contains(*page.NextCursor, "offset") {
		t.Fatal("unsigned offset cursor")
	}
	next, err := f.s.Instances(ctx, f.a, ListFilter{PolicyID: policy.ID, Limit: 1, Cursor: *page.NextCursor}, false)
	if err != nil || next.Total != 3 || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatal(next, err)
	}
	if _, err = f.s.Instances(ctx, f.a, ListFilter{PolicyID: other.ID, Limit: 1, Cursor: *page.NextCursor}, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("cursor replayed across filter", err)
	}
	if _, err = f.s.Instances(ctx, f.a, ListFilter{PolicyID: policy.ID, Limit: 1, Cursor: *page.NextCursor + "tamper"}, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("tampered cursor admitted", err)
	}
	f.exec("UPDATE data_authorization_grants SET status='disabled' WHERE enterprise_id=$1 AND subject_type='user'", f.a.EnterpriseID)
	if _, err = f.s.Instances(ctx, f.a, ListFilter{PolicyID: policy.ID, Limit: 1, Cursor: *page.NextCursor}, false); !errors.Is(err, ErrInvalid) {
		t.Fatal("scope change reused cursor", err)
	}
	empty, err := f.s.Instances(ctx, f.a, ListFilter{}, false)
	if err != nil || empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatal("unauthorized count leaked", empty, err)
	}
}

func TestPostgresDeliveryPolicyChannelDateFiltersDoNotBorrowOtherReceipts(t *testing.T) {
	f := newAlertFixture(t)
	ctx := t.Context()
	a, b := uuid.New(), uuid.New()
	channels := []uuid.UUID{}
	for range 2 {
		raw, _ := json.Marshal(ChannelSpec{Provider: "wecom", Enabled: true, CredentialID: uuid.New(), MentionIDs: []string{}})
		ch, err := f.s.Store.Queries.CreateAlertConfiguration(ctx, db.CreateAlertConfigurationParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, Kind: "channel", Name: "Fixture", Spec: raw, CreatedBy: f.a.SubjectID})
		if err != nil {
			t.Fatal(err)
		}
		channels = append(channels, ch.ID)
	}
	for i, id := range []uuid.UUID{a, b} {
		raw, _ := json.Marshal(Message{Subject: "same text", Text: "CPU 95%", PolicyIDs: []uuid.UUID{id}})
		_, err := f.s.Store.Queries.CreateAlertDelivery(ctx, db.CreateAlertDeliveryParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, ChannelID: channels[i], ChannelVersion: 1, RouteVersion: 1, DedupKey: uuid.NewString(), GroupKey: uuid.NewString(), ResourceIds: []uuid.UUID{f.host}, Payload: raw, Status: "accepted", AvailableAt: timestamp(time.Now())})
		if err != nil {
			t.Fatal(err)
		}
	}
	page, err := f.s.Deliveries(ctx, f.a, ListFilter{PolicyID: a, ChannelID: channels[0], NotificationState: "accepted"})
	if err != nil || page.Total != 1 || page.Items[0].ChannelID != channels[0] {
		t.Fatal(page, err)
	}
	none, err := f.s.Deliveries(ctx, f.a, ListFilter{PolicyID: a, ChannelID: channels[1]})
	if err != nil || none.Total != 0 {
		t.Fatal(none, err)
	}
	future := time.Now().Add(time.Hour)
	none, err = f.s.Deliveries(ctx, f.a, ListFilter{From: &future})
	if err != nil || none.Total != 0 {
		t.Fatal(none, err)
	}
}
