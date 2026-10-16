package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
)

func (a *App) plan6HostWaitNotifications(ctx context.Context, env *E2EEnvironment, h *plan6HostLoop, event string) error {
	if len(h.ConditionIDs) != 2 || len(h.Channels) != 4 {
		return fmt.Errorf("Host notification members/channels are not frozen")
	}
	data, err := a.plan6Wait(ctx, env, "host-notifications-"+event, "/alerts/deliveries?policy_id="+h.PolicyID+"&limit=100", func(v map[string]any) bool {
		seen := map[string]map[string]bool{}
		for _, row := range objectItems(v) {
			message, _ := row["message"].(map[string]any)
			if row["status"] != "accepted" || message["event_type"] != event {
				continue
			}
			channel, _ := row["channel_id"].(string)
			instance, _ := row["instance_id"].(string)
			if seen[channel] == nil {
				seen[channel] = map[string]bool{}
			}
			seen[channel][instance] = true
		}
		for _, channel := range h.Channels {
			for _, instance := range h.ConditionIDs {
				if !seen[channel][instance] {
					return false
				}
			}
		}
		return true
	})
	if err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(data, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "host-notifications-"+event+"-accepted.json"), raw)
}
