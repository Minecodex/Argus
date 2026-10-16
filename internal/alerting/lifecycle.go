package alerting

import "time"

// advance only receives successfully evaluated observations. Failure handling
// separately resets pending/recovery counters without fabricating a recovery.
func advance(lifecycle string, state InstanceState, obs Observation, p PolicySpec, slot time.Time) (string, InstanceState, string) {
	value := obs.Value
	state.Value = &value
	state.Metric = obs.Rule.Expression
	state.Members = obs.Members
	state.RuleID = obs.RuleID
	state.Threshold = obs.Rule.Threshold
	state.RecoveryThreshold = obs.Rule.RecoveryThreshold
	state.Unit = obs.Rule.Unit
	state.Labels = obs.Labels
	state.LastSuccessfulAt = &slot
	if !state.LastSlot.IsZero() && slot.Sub(state.LastSlot) > time.Duration(p.IntervalSeconds)*time.Second+time.Second {
		state.PendingSince = nil
		state.RecoveringSince = nil
	}
	state.LastSlot = slot
	event := ""
	switch lifecycle {
	case "normal", "":
		lifecycle = "normal"
		if obs.Firing {
			state.PendingSince = &slot
			lifecycle = "pending"
		}
	case "pending":
		if !obs.Firing {
			lifecycle = "normal"
			state.PendingSince = nil
		} else if state.PendingSince == nil {
			state.PendingSince = &slot
		}
	case "firing":
		if obs.Recovering {
			lifecycle = "recovering"
			state.RecoveringSince = &slot
		}
	case "recovering":
		if !obs.Recovering {
			lifecycle = "firing"
			state.RecoveringSince = nil
		} else if state.RecoveringSince == nil {
			state.RecoveringSince = &slot
		}
	}
	if lifecycle == "pending" && state.PendingSince != nil && slot.Sub(*state.PendingSince) >= time.Duration(p.ForSeconds)*time.Second {
		lifecycle = "firing"
		event = "first"
		state.PendingSince = nil
	}
	if lifecycle == "recovering" && state.RecoveringSince != nil && slot.Sub(*state.RecoveringSince) >= time.Duration(p.RecoverySeconds)*time.Second {
		lifecycle = "resolved"
		event = "recovery"
		state.RecoveringSince = nil
	}
	return lifecycle, state, event
}
func evaluationFailure(state InstanceState) InstanceState {
	state.PendingSince = nil
	state.RecoveringSince = nil
	return state
}
