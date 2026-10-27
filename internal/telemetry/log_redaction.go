package telemetry

import collectlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"

// Redaction runs before protobuf serialization and Kafka publication. Reader
// projection remains in place for historical data and derived query output.
func redactLogPayload(request *collectlogs.ExportLogsServiceRequest) error {
	for _, resource := range request.ResourceLogs {
		if resource == nil {
			return errIngestRejected
		}
		if resource.Resource != nil {
			if err := redactOTLPAttributes(resource.Resource.Attributes); err != nil {
				return err
			}
		}
		for _, scope := range resource.ScopeLogs {
			if scope == nil {
				return errIngestRejected
			}
			if scope.Scope != nil {
				if err := redactOTLPAttributes(scope.Scope.Attributes); err != nil {
					return err
				}
			}
			for _, record := range scope.LogRecords {
				if record == nil {
					return errIngestRejected
				}
				if err := redactOTLPValue(record.Body); err != nil {
					return err
				}
				if err := redactOTLPAttributes(record.Attributes); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
