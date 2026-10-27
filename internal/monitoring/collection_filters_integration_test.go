package monitoring

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func collectionFilterInput(t *testing.T, raw string) ListInput {
	t.Helper()
	var input ListInput
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		t.Fatal(err)
	}
	return input
}

func TestPostgresCollectionFiltersBeforePagination(t *testing.T) {
	f := newFixture(t)
	collector := f.collector()
	now := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	f.s.Now = func() time.Time { return now }
	prefix := uuid.New()
	ids := make([]uuid.UUID, 7)
	fixtures := []struct {
		integration             string
		enabled                 bool
		apply, health, interval string
		sample                  any
		granted                 bool
	}{
		{"mysql", true, "effective", "healthy", "30s", now.Add(-time.Minute), true},
		{"redis", false, "effective", "stopped", "30s", nil, true},
		{"mysql", false, "failed", "failed", "30s", now.Add(-5 * time.Minute), true},
		{"mysql", true, "pending", "unknown", "1m30s", now.Add(-3 * time.Minute), true},
		{"mysql", true, "applying", "degraded", "30s", now.Add(-2 * time.Minute), true},
		{"mysql", true, "unknown", "unknown", "invalid", now.Add(-121 * time.Second), true},
		{"mysql", false, "failed", "failed", "30s", nil, false},
	}
	for i, item := range fixtures {
		id := prefix
		id[14] = 0
		id[15] = byte(i + 1)
		ids[i] = id
		target := uuid.New()
		f.exec("INSERT INTO monitoring_targets(id,enterprise_id,name,kind,engine,identity_config,created_by) VALUES($1,$2,'filter target','database',$3,'{}',$4)", target, f.a.EnterpriseID, item.integration, f.a.SubjectID)
		if item.granted {
			f.exec("INSERT INTO data_authorization_grants(id,enterprise_id,subject_type,subject_id,resource_type,resource_id) VALUES($1,$2,'user',$3,'monitoring_target',$4)", uuid.New(), f.a.EnterpriseID, f.a.SubjectID, target)
		}
		config, _ := json.Marshal(map[string]string{"interval": item.interval})
		f.exec("INSERT INTO monitoring_collections(id,enterprise_id,name,integration_key,integration_version,target_type,target_id,collector_id,config,desired_enabled,apply_status,health,last_sample_at,created_by) VALUES($1,$2,'Filter collection',$3,1,'monitoring_target',$4,$5,$6,$7,$8,$9,$10,$11)", id, f.a.EnterpriseID, item.integration, target, collector, config, item.enabled, item.apply, item.health, item.sample, f.a.SubjectID)
	}
	for _, tc := range []struct {
		name, input string
		wanted      []int
	}{
		{"enabled false", `{"desired_enabled":false}`, []int{1, 2}},
		{"apply status", `{"apply_status":"failed"}`, []int{2}},
		{"health", `{"health":"healthy"}`, []int{0}},
		{"fresh including interval and boundary", `{"freshness":"fresh"}`, []int{0, 3, 4}},
		{"stale including invalid-interval fallback", `{"freshness":"stale"}`, []int{2, 5}},
		{"no data", `{"freshness":"no_data"}`, []int{1}},
		{"combined", `{"integration":"mysql","desired_enabled":false,"apply_status":"failed","health":"failed","freshness":"stale","search":"Filter"}`, []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := collectionFilterInput(t, tc.input)
			input.Limit = 1
			actual := []uuid.UUID{}
			for attempt := 0; attempt < 10; attempt++ {
				page, err := f.s.Collections(t.Context(), f.a, input)
				if err != nil {
					t.Fatal(err)
				}
				for _, row := range page.Items {
					actual = append(actual, row.ID)
				}
				if page.NextCursor == "" {
					break
				}
				input.Cursor = page.NextCursor
			}
			wanted := []uuid.UUID{}
			for _, index := range tc.wanted {
				wanted = append(wanted, ids[index])
			}
			if !slices.Equal(actual, wanted) {
				t.Fatalf("filtered pages=%v want=%v", actual, wanted)
			}
		})
	}
	hidden, err := f.s.Collections(t.Context(), f.other, collectionFilterInput(t, `{"desired_enabled":false}`))
	if err != nil || len(hidden.Items) != 0 {
		t.Fatalf("filter widened resource authorization: %#v %v", hidden, err)
	}
	for _, raw := range []string{`{"apply_status":"ready"}`, `{"health":"good"}`, `{"freshness":"recent"}`} {
		_, err := f.s.Collections(t.Context(), f.a, collectionFilterInput(t, raw))
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid filter accepted: %s %v", raw, err)
		}
	}
}

func TestCollectionCursorBindsFilterDimensions(t *testing.T) {
	service := Service{CursorKey: []byte("01234567890123456789012345678901")}
	actor := Actor{EnterpriseID: uuid.New(), SubjectID: uuid.New(), SubjectType: "user", AuthorizationVersion: 1}
	original := collectionFilterInput(t, `{"desired_enabled":false,"apply_status":"effective","health":"healthy","freshness":"fresh"}`)
	_, _, binding, err := service.page(actor, "collections", original)
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := service.next(binding, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"desired_enabled":true,"apply_status":"effective","health":"healthy","freshness":"fresh"}`,
		`{"apply_status":"effective","health":"healthy","freshness":"fresh"}`,
		`{"desired_enabled":false,"apply_status":"pending","health":"healthy","freshness":"fresh"}`,
		`{"desired_enabled":false,"apply_status":"effective","health":"failed","freshness":"fresh"}`,
		`{"desired_enabled":false,"apply_status":"effective","health":"healthy","freshness":"stale"}`,
	} {
		next := collectionFilterInput(t, raw)
		next.Cursor = cursor
		if _, _, _, err := service.page(actor, "collections", next); err == nil {
			t.Fatalf("cursor accepted different filters: %s", raw)
		}
	}
}
