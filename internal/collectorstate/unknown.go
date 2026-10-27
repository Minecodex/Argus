package collectorstate

// An unknown outcome cannot authorize a new physical source or discard the
// pending responsibility before a verified outcome or explicit repair.
func OutcomeUnknown(code string) bool {
	switch code {
	case "COLLECTOR_ROLLBACK_UNVERIFIED", "CONNECTOR_COMMAND_TIMED_OUT", "EXECUTION_RESULT_UNKNOWN", "CONNECTOR_DISCONNECTED":
		return true
	default:
		return false
	}
}
