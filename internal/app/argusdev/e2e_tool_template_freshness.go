package argusdev

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/alerting"
)

// These typed facts exercise the real tool/read/UI pipeline. The independent
// PostgreSQL evaluator test verifies how failures actually retain this state.
func (a *App) templateHealthFacts(ctx context.Context, env *E2EEnvironment, policy, resource string, base alerting.InstanceState) error {
	for _, health := range []string{"error", "no_data", "authorization_denied", "recovered"} {
		id, state := uuid.NewString(), base
		state.Name = "Template " + health + " trigger"
		success := time.Now().UTC().Add(-5 * time.Minute)
		actualHealth := health
		if health == "recovered" {
			actualHealth, success = "normal", time.Now().UTC()
		}
		state.LastSuccessfulAt = &success
		state.LastSlot = time.Now().UTC()
		sql := fmt.Sprintf(`INSERT INTO alert_instances(id,enterprise_id,policy_id,policy_version,generation,fingerprint,resource_ids,source_ids,state,lifecycle,health,severity,started_at,last_evaluated_at) VALUES('%s','%s','%s',1,1,'template-health-%s',ARRAY['%s']::uuid[],'{}',%s,'firing','%s','critical',now()-interval '10 minutes',now());`, id, env.State.Values["enterprise_id"], policy, health, resource, templateSQLJSON(state), actualHealth)
		if _, err := a.postgresQuery(ctx, env, sql); err != nil {
			return err
		}
		env.State.Values["template_health_"+health] = id
	}
	return nil
}
