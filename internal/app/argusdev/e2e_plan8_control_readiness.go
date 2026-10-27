package argusdev

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A successful Deployment rollout does not establish Connector or relay
// readiness. Observe the actual control path before sending any mutation.
func (a *App) plan8WaitCollectorPaths(ctx context.Context, env *E2EEnvironment) error {
	enterprise := env.State.Values["enterprise_id"]
	if uuid.Validate(enterprise) != nil {
		return errors.New("invalid acceptance enterprise")
	}
	query := fmt.Sprintf(`SELECT count(*)=2 AND coalesce(bool_and(
 c.status='converged' AND c.desired_revision=c.effective_revision
 AND x.status='online' AND r.status='active'
 AND (r.transport<>'bastion_tunnel' OR
 (b.status='active' AND b.relay_status='ready' AND bx.status='online'))),false)
 FROM collector_instances c
 JOIN hosts h ON h.enterprise_id=c.enterprise_id AND h.id=c.resource_id AND c.resource_type='host'
 JOIN connectors x ON x.enterprise_id=h.enterprise_id AND x.id=h.connector_id
 JOIN telemetry_routes r ON r.enterprise_id=c.enterprise_id AND r.collector_id=c.id
 LEFT JOIN bastion_scopes b ON b.enterprise_id=h.enterprise_id AND b.id=h.bastion_scope_id
 LEFT JOIN connectors bx ON bx.enterprise_id=b.enterprise_id AND bx.id=b.active_connector_id
 WHERE c.enterprise_id='%s'`, enterprise)
	deadline := time.Now().Add(2 * time.Minute)
	for {
		value, err := a.postgresQuery(ctx, env, query)
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) == "t" {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("deployed Collector control paths did not become ready after rollout")
		}
		if err = waitContext(ctx, time.Second); err != nil {
			return err
		}
	}
}
