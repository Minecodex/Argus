package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

func (a *App) plan6HostVerifyHistory(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop) error {
	raw, err := a.postgresQuery(ctx, env, fmt.Sprintf(`SELECT coalesce(json_agg(json_build_object('id',i.id,'lifecycle',i.lifecycle,'acknowledged',i.acknowledged,'owner',i.owner_user_id,'resources',i.resource_ids,'source_ids',i.source_ids,'events',(SELECT json_agg(json_build_object('type',e.event_type,'resources',e.resource_ids,'payload',e.payload,'actor',e.actor_id) ORDER BY e.created_at,e.id) FROM alert_events e WHERE e.instance_id=i.id))),'[]') FROM alert_instances i WHERE i.enterprise_id='%s' AND i.policy_id='%s' AND i.state->>'kind'='condition';`, env.State.Values["enterprise_id"], h.PolicyID))
	if err != nil {
		return err
	}
	if err = writePrivate(filepath.Join(env.Options.Artifacts, "host-handling-noise-history.json"), []byte(raw)); err != nil {
		return err
	}
	var rows []struct {
		ID, Lifecycle, Owner string
		Acknowledged         bool
		Resources            []string
		SourceIDs            []string `json:"source_ids"`
		Events               []struct {
			Type, Actor string
			Resources   []string
		}
	}
	if json.Unmarshal([]byte(raw), &rows) != nil || len(rows) != 2 {
		return fmt.Errorf("real Host history is incomplete")
	}
	inhibited := 0
	for _, row := range rows {
		if row.Lifecycle != "resolved" || !row.Acknowledged || row.Owner != env.State.Values["admin_user_id"] || len(row.Resources) != 1 || row.Resources[0] != h.HostID || len(row.SourceIDs) != 1 || row.SourceIDs[0] != h.SourceID {
			return fmt.Errorf("Host handling or historical resource/source scope was lost")
		}
		seen := map[string]bool{}
		for _, event := range row.Events {
			if len(event.Resources) != 1 || event.Resources[0] != h.HostID {
				return fmt.Errorf("Host lifecycle event lost its original resource scope")
			}
			seen[event.Type] = true
			if event.Type == "acknowledge" || event.Type == "assign" || event.Type == "comment" {
				if event.Actor != env.State.Values["admin_user_id"] {
					return fmt.Errorf("manual handling history lost its actor")
				}
			}
		}
		for _, kind := range []string{"first", "acknowledge", "assign", "comment", "muted", "unmuted", "recovery"} {
			if !seen[kind] {
				return fmt.Errorf("Host %s history lacks %s", row.ID, kind)
			}
		}
		if seen["inhibited"] {
			if !seen["uninhibited"] {
				return fmt.Errorf("suppressed warning lacks release history")
			}
			inhibited++
		}
	}
	if inhibited != 1 {
		return fmt.Errorf("critical-to-warning inhibition was not recorded exactly once")
	}
	return nil
}
