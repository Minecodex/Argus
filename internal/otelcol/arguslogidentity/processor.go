// Package arguslogidentity validates an Argus log assignment after the native
// filelog receiver and k8sattributes processor. It performs no collection or
// Kubernetes API access. Missing metadata is retryable, not a successful drop.
package arguslogidentity

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

type Config struct {
	Namespace    string   `mapstructure:"namespace"`
	WorkloadType string   `mapstructure:"workload_type"`
	WorkloadUID  string   `mapstructure:"workload_uid"`
	NodeName     string   `mapstructure:"node_name"`
	Containers   []string `mapstructure:"containers"`
}

func (c *Config) Validate() error {
	id, e := uuid.Parse(c.WorkloadUID)
	if e != nil || id == uuid.Nil || c.Namespace == "" || c.NodeName == "" || len(c.Containers) == 0 || len(c.Containers) > 32 || !slices.Contains([]string{"pod", "deployment", "replicaset", "daemonset", "statefulset", "job", "cronjob"}, c.WorkloadType) {
		return errors.New("log assignment identity is invalid")
	}
	return nil
}
func NewFactory() processor.Factory {
	return processor.NewFactory(component.MustNewType("argus_log_identity"), func() component.Config { return &Config{} }, processor.WithLogs(createLogs, component.StabilityLevelBeta))
}
func createLogs(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Logs) (processor.Logs, error) {
	c := cfg.(*Config)
	return processorhelper.NewLogs(ctx, set, cfg, next, func(_ context.Context, value plog.Logs) (plog.Logs, error) { return c.process(value) }, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}
func text(attrs pcommon.Map, key string) string {
	v, ok := attrs.Get(key)
	if !ok || v.Type() != pcommon.ValueTypeStr {
		return ""
	}
	return v.Str()
}

func (c *Config) belongs(attrs pcommon.Map) (bool, error) {
	if text(attrs, "k8s.namespace.name") != c.Namespace || !slices.Contains(c.Containers, text(attrs, "k8s.container.name")) {
		return false, nil
	}
	if _, e := uuid.Parse(text(attrs, "k8s.pod.uid")); e != nil {
		return false, consumererror.NewPermanent(errors.New("log Pod identity is invalid"))
	}
	if text(attrs, "k8s.pod.start_time") == "" || text(attrs, "k8s.node.name") == "" {
		return false, errors.New("log Pod metadata is not yet available")
	}
	if text(attrs, "k8s.node.name") != c.NodeName {
		return false, nil
	}
	owner := text(attrs, "k8s."+c.WorkloadType+".uid")
	if owner == "" {
		// Direct controller UIDs come from the known Pod's owner reference.
		// Only Deployment/ReplicaSet and CronJob/Job need a second informer.
		// A known standalone Pod must not stall an unrelated controller rule.
		if c.WorkloadType == "deployment" && text(attrs, "k8s.replicaset.uid") != "" || c.WorkloadType == "cronjob" && text(attrs, "k8s.job.uid") != "" {
			return false, errors.New("log workload metadata is not yet available")
		}
		return false, nil
	}
	return owner == c.WorkloadUID, nil
}
func (c *Config) process(value plog.Logs) (plog.Logs, error) {
	resources := value.ResourceLogs()
	// Validate the entire input before mutating it: a retry must retain every
	// original record, including those already matched earlier in the batch.
	for i := 0; i < resources.Len(); i++ {
		if _, err := c.belongs(resources.At(i).Resource().Attributes()); err != nil {
			return value, err
		}
	}
	resources.RemoveIf(func(resource plog.ResourceLogs) bool {
		attrs := resource.Resource().Attributes()
		belongs, _ := c.belongs(attrs)
		if belongs {
			attrs.RemoveIf(func(key string, _ pcommon.Value) bool { return strings.HasPrefix(key, "argus.") })
		}
		return !belongs
	})
	return value, nil
}
