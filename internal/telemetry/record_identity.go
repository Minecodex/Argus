package telemetry

import (
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
)

// Kafka headers identify the authenticated sender. The v3 normalized payload
// contains the independently verified identity of every Resource block.
func recordResourceIdentity(record *kgo.Record, sender TrustedIdentity, attrs []*commonpb.KeyValue) (TrustedIdentity, error) {
	version := ""
	for _, header := range record.Headers {
		if header.Key == "argus-normalization-version" {
			version = string(header.Value)
		}
	}
	if version == "" {
		return sender, nil
	}
	if version != "v3" {
		return TrustedIdentity{}, fmt.Errorf("%w: normalization version", errPermanentRecord)
	}
	values := attributesMap(attrs)
	if values["argus.enterprise.id"] != sender.EnterpriseID.String() || values["argus.collector.id"] != sender.CollectorID.String() {
		return TrustedIdentity{}, fmt.Errorf("%w: resource sender mismatch", errPermanentRecord)
	}
	id, err := uuid.Parse(values["argus.resource.id"])
	if err != nil || id == uuid.Nil || !slices.Contains([]string{"host", "kubernetes_cluster", "monitoring_target", "application"}, values["argus.resource.type"]) {
		return TrustedIdentity{}, fmt.Errorf("%w: resource identity", errPermanentRecord)
	}
	if id != sender.ResourceID {
		source, err := recordSource(attrs)
		if err != nil || source.ID == uuid.Nil || source.Revision < 1 {
			return TrustedIdentity{}, fmt.Errorf("%w: unbound resource identity", errPermanentRecord)
		}
	}
	sender.ResourceID, sender.ResourceType = id, values["argus.resource.type"]
	return sender, nil
}
