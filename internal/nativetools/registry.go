// Package nativetools is the shared composition root for public native tools.
// Domain services own authorization, execution and presentation; this package
// only wires the same registrations for Server, Worker and contract generators.
package nativetools

import (
	"fmt"

	"github.com/kakj-go/Argus/internal/alerting"
	"github.com/kakj-go/Argus/internal/dashboard"
	"github.com/kakj-go/Argus/internal/interaction"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/monitoring"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/telemetry"
	"github.com/kakj-go/Argus/internal/toolgateway"
	"github.com/kakj-go/Argus/internal/workspace"
)

type Services struct {
	Resources     toolgateway.ResourceTools
	Monitoring    monitoring.Service
	Kubernetes    kubernetesworkbench.Service
	Lifecycle     toolgateway.LifecycleTools
	Telemetry     *telemetry.Service
	Workspace     workspace.Service
	Remote        *remoteexecution.Service
	Dashboard     dashboard.Service
	DashboardJobs *dashboard.QueryJobs
	Alerting      alerting.Service
	Interaction   interaction.Service
}

func Register(registry *mcp.Registry, s Services) error {
	// Managed defaults are a shared resource discovery policy, independent of
	// whether the caller runs in the HTTP server or the inference worker.
	s.Dashboard.ProvideDefaults = true
	observationTelemetry := telemetry.Service{Store: s.Resources.Store}
	if s.Telemetry != nil {
		observationTelemetry = *s.Telemetry
	}
	observationTelemetry.ModelQueryMeter = func(a telemetry.Actor, base telemetry.EngineQueryBackend) telemetry.EngineQueryBackend {
		return dashboard.MeterObservationBackend(s.Resources.Store, a, base)
	}
	registers := []func(*mcp.Registry) error{
		(toolgateway.ObservationTools{Base: s.Resources, Monitoring: s.Monitoring, Telemetry: observationTelemetry, Dashboard: s.Dashboard, CursorKey: s.Monitoring.CursorKey}).Register,
		(toolgateway.MonitoringTools{Base: s.Resources, Monitoring: s.Monitoring, Kubernetes: s.Kubernetes}).Register,
		s.Resources.Register, s.Lifecycle.Register,
	}
	if s.Telemetry != nil {
		registers = append(registers, (telemetry.Tools{Service: observationTelemetry}).Register,
			(toolgateway.CollectorPreviewTools{Base: s.Resources, Service: *s.Telemetry}).Register)
	}
	registers = append(registers, s.Workspace.RegisterTools)
	if s.Remote != nil {
		registers = append(registers, s.Remote.RegisterTools)
	}
	registers = append(registers, (toolgateway.DashboardTools{Service: s.Dashboard, Jobs: s.DashboardJobs}).Register,
		(alerting.Tools{Service: s.Alerting}).Register, s.Interaction.RegisterTools)
	for _, register := range registers {
		if err := register(registry); err != nil {
			return fmt.Errorf("register native tools: %w", err)
		}
	}
	for _, metadata := range registry.ModelCatalog() {
		if err := metadata.Discovery.Navigation.Validate(); err != nil {
			return fmt.Errorf("tool %s: %w", metadata.ID, err)
		}
	}
	return registry.ValidatePairs()
}

// Catalog constructs production definitions without connecting to services or
// executing a business handler. It deliberately includes host interactions.
func Catalog() (*mcp.Registry, error) {
	store := &postgres.Store{}
	base := toolgateway.ResourceTools{Store: store}
	telemetryDomain := telemetry.Service{Store: store}
	registry := mcp.NewRegistry()
	err := Register(registry, Services{Resources: base, Monitoring: monitoring.Service{Store: store},
		Kubernetes: kubernetesworkbench.Service{Store: store}, Lifecycle: toolgateway.LifecycleTools{Base: base},
		Telemetry: &telemetryDomain, Remote: &remoteexecution.Service{Store: store},
		Dashboard: dashboard.Service{Store: store}, Alerting: alerting.Service{Store: store}, Interaction: interaction.Service{Store: store}})
	return registry, err
}
