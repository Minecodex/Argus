package dashboard

// Generated targets must honor the same non-null collection contract as
// authored JSON; frontend editors should not need a separate generated shape.
func normalizeTargetCollections(target *Target) {
	if target.ParameterBindings == nil {
		target.ParameterBindings = []ParameterBinding{}
	}
	if original := target.SourceDefinition.Builder; original != nil {
		builder := *original
		if builder.Filters == nil {
			builder.Filters = []Filter{}
		}
		if builder.GroupBy == nil {
			builder.GroupBy = []string{}
		}
		target.SourceDefinition.Builder = &builder
	}
}
func normalizePanelCollections(panel *Panel) {
	if panel.LocalFilters == nil {
		panel.LocalFilters = []LocalFilter{}
	}
	if panel.Targets == nil {
		panel.Targets = []Target{}
	}
	if panel.DetailQueryTargets == nil {
		panel.DetailQueryTargets = []Target{}
	}
	if panel.Drilldowns == nil {
		panel.Drilldowns = []Drilldown{}
	}
	if panel.Thresholds == nil {
		panel.Thresholds = []Threshold{}
	}
	if panel.ApplicableResourceTypes == nil {
		panel.ApplicableResourceTypes = []string{}
	}
	for i := range panel.Targets {
		normalizeTargetCollections(&panel.Targets[i])
	}
	for i := range panel.DetailQueryTargets {
		normalizeTargetCollections(&panel.DetailQueryTargets[i])
	}
}
