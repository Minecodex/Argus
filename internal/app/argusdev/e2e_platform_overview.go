package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// This fixture is scoped to the disposable E2E database. More than 200 rows
// prove the aggregate endpoint cannot silently inherit the old list limit.
func (a *App) verifyPlatformOverview(ctx context.Context, env *E2EEnvironment) error {
	client, err := scenarioHTTP(env)
	if err != nil {
		return err
	}
	read := func(label string) (map[string]any, error) {
		return client.JSON(ctx, label, "platform", http.MethodGet, "/platform/overview", http.StatusOK, nil, map[string]string{"Origin": env.PlatformOrigin()})
	}
	before, err := read("platform-overview-before")
	if err != nil {
		return err
	}
	if _, err = client.JSON(ctx, "platform-overview-enterprise-denied", "enterprise", http.MethodGet, "/platform/overview", http.StatusUnauthorized, nil, map[string]string{"Origin": env.EnterpriseOrigin()}); err != nil {
		return err
	}
	marker := "overview-" + uuid.NewString()
	cleanup := fmt.Sprintf("DELETE FROM sandbox_usage WHERE enterprise_id IN (SELECT id FROM enterprises WHERE code LIKE '%s-%%'); DELETE FROM enterprises WHERE code LIKE '%s-%%';", marker, marker)
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = a.postgresQuery(cleanupCtx, env, cleanup)
	}()
	seed := fmt.Sprintf(`WITH created AS (
		INSERT INTO enterprises (id,name,code,timezone,default_locale)
		SELECT gen_random_uuid(), '%s-'||n, '%s-'||n, 'UTC', 'zh-CN' FROM generate_series(1,205) n RETURNING id
	) INSERT INTO sandbox_usage (enterprise_id,month,session_count,session_seconds)
	SELECT id, date_trunc('month',now() AT TIME ZONE 'UTC')::date, 3, 31 FROM created;`, marker, marker)
	if _, err = a.postgresQuery(ctx, env, seed); err != nil {
		return err
	}
	after, err := read("platform-overview-over-200")
	if err != nil {
		return err
	}
	if after["enterprise_count"].(float64) != before["enterprise_count"].(float64)+205 || after["active_enterprise_count"].(float64) != before["active_enterprise_count"].(float64)+205 {
		return fmt.Errorf("platform overview truncated enterprise counts")
	}
	month := time.Now().UTC().Format("2006-01")
	totals := func(value map[string]any) (float64, float64, error) {
		rows, ok := value["monthly_usage"].([]any)
		if !ok || len(rows) > 12 {
			return 0, 0, fmt.Errorf("invalid monthly overview")
		}
		for _, row := range rows {
			item := row.(map[string]any)
			if item["month"] == month {
				return item["session_count"].(float64), item["session_seconds"].(float64), nil
			}
		}
		return 0, 0, nil
	}
	oldCount, oldSeconds, err := totals(before)
	if err != nil {
		return err
	}
	count, seconds, err := totals(after)
	if err != nil {
		return err
	}
	if count != oldCount+615 || seconds != oldSeconds+6355 {
		return fmt.Errorf("platform overview lost or rounded usage rows: count=%v seconds=%v", count-oldCount, seconds-oldSeconds)
	}
	if _, err = a.postgresQuery(ctx, env, cleanup); err != nil {
		return err
	}
	proof, err := json.Marshal(map[string]any{"status": "passed", "enterprise_fixture_rows": 205, "session_count_delta": count - oldCount, "session_seconds_delta": seconds - oldSeconds, "enterprise_credential_denied": true, "fixture_cleaned": true, "before": before, "after": after})
	if err != nil {
		return err
	}
	return writePrivate(filepath.Join(env.Options.Artifacts, "platform-overview.json"), proof)
}
