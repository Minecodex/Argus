package argusdev

type plan8DeployedStage struct {
	name string
	run  func() error
}

func plan11ComparisonPhase(phase string) bool {
	return phase == "plan11-compare" || phase == "plan11-review"
}

func plan11ComparisonEnvironmentAllowed(env *E2EEnvironment, host *plan6HostLoop) bool {
	return env != nil && env.Options.Suite == "plan8" && plan11ComparisonPhase(env.Options.Plan8Phase) && env.leaseAcquired && host != nil
}

func plan11ReviewModelNames(repeats int, selected []string) []string {
	if len(selected) == 0 {
		selected = []string{"http"}
	}
	names := append([]string{}, plan8SelectedModelNames("plan11-access")...)
	names = append(names, plan11TargetModelNames(selected)...)
	return append(names, plan11ComparisonModelNames(repeats)...)
}

// All fixture/configuration/authorization changes finish before the comparison
// freezes Writer and switches profiles. The underlying stages stay shared.
func plan8OrderedDeployedStages(phase string, stages []plan8DeployedStage) []plan8DeployedStage {
	if phase == "plan11-authoring-ui-replay" {
		// The production endpoint policy intentionally rejects the local scripted
		// provider. Complete that controlled L4 fixture before the normal switch;
		// the final stage rechecks its published data using the normal backend.
		ordered := make([]plan8DeployedStage, 0, len(stages))
		var replay plan8DeployedStage
		for _, stage := range stages {
			if stage.name == "plan11-authoring-ui-replay" {
				replay = stage
			}
		}
		for _, stage := range stages {
			if stage.name == replay.name {
				continue
			}
			if stage.name == "normal-production-backend" && replay.name != "" {
				ordered = append(ordered, replay)
			}
			ordered = append(ordered, stage)
		}
		return ordered
	}
	if phase != "plan11-review" {
		return stages
	}
	for i, stage := range stages {
		if stage.name == "plan11-profile-comparison" {
			ordered := make([]plan8DeployedStage, 0, len(stages))
			ordered = append(ordered, stages[:i]...)
			ordered = append(ordered, stages[i+1:]...)
			return append(ordered, stage)
		}
	}
	return stages
}
