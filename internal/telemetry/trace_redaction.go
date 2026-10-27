package telemetry

import collecttraces "go.opentelemetry.io/proto/otlp/collector/trace/v1"

func redactOTLPText(value *string) error {
	item := stringValue(*value)
	if err := redactOTLPValue(item); err != nil {
		return err
	}
	*value = item.GetStringValue()
	return nil
}

// Trace attributes, events and links carry the same credential-bearing fields
// as logs. Remove recognized values before the Kafka durability boundary.
func redactTracePayload(request *collecttraces.ExportTraceServiceRequest) error {
	for _, resource := range request.ResourceSpans {
		if resource == nil {
			return errIngestRejected
		}
		if resource.Resource != nil {
			if err := redactOTLPAttributes(resource.Resource.Attributes); err != nil {
				return err
			}
		}
		for _, scope := range resource.ScopeSpans {
			if scope == nil {
				return errIngestRejected
			}
			if scope.Scope != nil {
				if err := redactOTLPAttributes(scope.Scope.Attributes); err != nil {
					return err
				}
			}
			for _, span := range scope.Spans {
				if span == nil {
					return errIngestRejected
				}
				if err := redactOTLPAttributes(span.Attributes); err != nil {
					return err
				}
				for _, value := range []*string{&span.Name, &span.TraceState} {
					if err := redactOTLPText(value); err != nil {
						return err
					}
				}
				if span.Status != nil {
					if err := redactOTLPText(&span.Status.Message); err != nil {
						return err
					}
				}
				for _, event := range span.Events {
					if event == nil {
						return errIngestRejected
					}
					if err := redactOTLPAttributes(event.Attributes); err != nil {
						return err
					}
					if err := redactOTLPText(&event.Name); err != nil {
						return err
					}
				}
				for _, link := range span.Links {
					if link == nil {
						return errIngestRejected
					}
					if err := redactOTLPAttributes(link.Attributes); err != nil {
						return err
					}
					if err := redactOTLPText(&link.TraceState); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
