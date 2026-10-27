package argusapplicationidentity

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

type Binding struct {
	EndpointID      string `mapstructure:"endpoint_id"`
	EndpointVersion int64  `mapstructure:"endpoint_version"`
	SourceID        string `mapstructure:"source_id"`
	SourceRevision  int64  `mapstructure:"source_revision"`
}
type Config struct {
	Bindings []Binding `mapstructure:"bindings"`
}

func (c *Config) Validate() error {
	seen := map[string]bool{}
	if len(c.Bindings) == 0 || len(c.Bindings) > 256 {
		return errors.New("application source bindings required")
	}
	for _, b := range c.Bindings {
		id, e := uuid.Parse(b.EndpointID)
		source, e2 := uuid.Parse(b.SourceID)
		if e != nil || e2 != nil || id == uuid.Nil || source == uuid.Nil || b.EndpointVersion < 1 || b.SourceRevision < 1 || seen[b.EndpointID] {
			return errors.New("application source binding invalid")
		}
		seen[b.EndpointID] = true
	}
	return nil
}
func NewFactory() processor.Factory {
	return processor.NewFactory(component.MustNewType("argus_application_identity"), func() component.Config { return &Config{} }, processor.WithMetrics(createMetrics, component.StabilityLevelBeta), processor.WithLogs(createLogs, component.StabilityLevelBeta), processor.WithTraces(createTraces, component.StabilityLevelBeta))
}
func authenticate(ctx context.Context, c *Config, signal string) (Binding, string, error) {
	rejected := consumererror.NewPermanent(errors.New("application source identity rejected"))
	info := client.FromContext(ctx)
	if info.Auth == nil {
		return Binding{}, "", rejected
	}
	if failure, _ := info.Auth.GetAttribute("transient_error").(string); failure != "" {
		return Binding{}, "", errors.New("application ingestion authentication temporarily unavailable")
	}
	key, _ := info.Auth.GetAttribute("key_id").(string)
	endpoint, _ := info.Auth.GetAttribute("endpoint_id").(string)
	version, _ := info.Auth.GetAttribute("endpoint_version").(string)
	signals, _ := info.Auth.GetAttribute("signals").(string)
	id, err := uuid.Parse(key)
	if err != nil || id == uuid.Nil || !slices.Contains(strings.Split(signals, ","), signal) {
		return Binding{}, "", rejected
	}
	for _, binding := range c.Bindings {
		if binding.EndpointID == endpoint && strconv.FormatInt(binding.EndpointVersion, 10) == version {
			return binding, key, nil
		}
	}
	return Binding{}, "", rejected
}
func stamp(attrs pcommon.Map, b Binding, key string) {
	attrs.RemoveIf(func(name string, _ pcommon.Value) bool { return strings.HasPrefix(name, "argus.") })
	attrs.PutStr("argus.source.id", b.SourceID)
	attrs.PutStr("argus.source.revision", strconv.FormatInt(b.SourceRevision, 10))
	attrs.PutStr("argus.application.key.id", key)
}
func createMetrics(ctx context.Context, set processor.Settings, config component.Config, next consumer.Metrics) (processor.Metrics, error) {
	c := config.(*Config)
	return processorhelper.NewMetrics(ctx, set, config, next, func(ctx context.Context, value pmetric.Metrics) (pmetric.Metrics, error) {
		b, key, err := authenticate(ctx, c, "metrics")
		if err != nil {
			return value, err
		}
		resources := value.ResourceMetrics()
		// Validate the whole request before stamping or handing it to batch.
		for i := 0; i < resources.Len(); i++ {
			if err := validateServiceIdentity(resources.At(i).Resource().Attributes()); err != nil {
				return value, err
			}
		}
		for i := 0; i < resources.Len(); i++ {
			stamp(resources.At(i).Resource().Attributes(), b, key)
		}
		return value, nil
	}, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}
func createLogs(ctx context.Context, set processor.Settings, config component.Config, next consumer.Logs) (processor.Logs, error) {
	c := config.(*Config)
	return processorhelper.NewLogs(ctx, set, config, next, func(ctx context.Context, value plog.Logs) (plog.Logs, error) {
		b, key, err := authenticate(ctx, c, "logs")
		if err != nil {
			return value, err
		}
		resources := value.ResourceLogs()
		// Validate the whole request before stamping or handing it to batch.
		for i := 0; i < resources.Len(); i++ {
			if err := validateServiceIdentity(resources.At(i).Resource().Attributes()); err != nil {
				return value, err
			}
		}
		for i := 0; i < resources.Len(); i++ {
			stamp(resources.At(i).Resource().Attributes(), b, key)
		}
		return value, nil
	}, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}
func createTraces(ctx context.Context, set processor.Settings, config component.Config, next consumer.Traces) (processor.Traces, error) {
	c := config.(*Config)
	return processorhelper.NewTraces(ctx, set, config, next, func(ctx context.Context, value ptrace.Traces) (ptrace.Traces, error) {
		b, key, err := authenticate(ctx, c, "traces")
		if err != nil {
			return value, err
		}
		resources := value.ResourceSpans()
		// Validate the whole request before stamping or handing it to batch.
		for i := 0; i < resources.Len(); i++ {
			if err := validateServiceIdentity(resources.At(i).Resource().Attributes()); err != nil {
				return value, err
			}
		}
		for i := 0; i < resources.Len(); i++ {
			stamp(resources.At(i).Resource().Attributes(), b, key)
		}
		return value, nil
	}, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}
