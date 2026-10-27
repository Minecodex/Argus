package argusdev

import (
	"errors"
	"fmt"
	"maps"
)

// Each group retains the frozen per-batch retry, capacity and per-check budget.
// The protocol/browser producer keeps the sdk identity and artifact locations.
func plan8Checks(suite, dir string) ([]plan8Check, error) {
	checks, err := plan8BaseChecks(suite, dir)
	if err != nil {
		return nil, err
	}
	return plan8BoundedSDKChecks(checks)
}

func plan8BoundedSDKChecks(checks []plan8Check) ([]plan8Check, error) {
	groups := []struct {
		id, scope string
		tests     []string
	}{
		{"sdk", "real_sdk_protocol_identity_application_pipeline_and_native_browser", []string{"TestNativeSDKApplicationPipelineDiscoveryAPMLogsAndKeyRevocation", "TestNativeSDKGRPCProtocolsDiscoverServicesAndFenceRevokedKeys", "TestNativeJaegerThriftHTTPAuthenticationDiscoveryAndRotation", "TestNativeSDKRejectsInvalidServiceIdentityBeforeBatching"}},
		{"sdk-capacity", "actual_frozen_sdk_and_32_dedicated_target_capacity", []string{"TestNativeSDKCapacityProfileStoresMeasuredSeriesAndHonorsRevocation", "TestNativeDedicatedHTTP32TargetFrozenCapacity"}},
		{"sdk-queue", "actual_native_queue_crash_recovery_and_owned_filesystem_exhaustion", []string{"TestNativeSDKQueueCapacityRecoversAcceptedPointsAfterOutageAndCrash", "TestNativeQueueFilesystemFullRejectsThenRecoversAcknowledgedPoints"}},
		{"sdk-gap", "actual_five_minute_exhaustion_gap_restart_and_control_outage", []string{"TestNativeFiveMinuteExportFailurePersistsGapAcrossRestartAndControlOutage"}},
		{"sdk-tls-quota", "actual_tls_identity_query_budget_and_ingest_quota", []string{"TestNativeSDKRejectsWrongTLSIdentityAndHonorsActualQuerySeriesQuota", "TestNativeIngestQuotaExplicitlyRejectsAndRecovers"}},
		{"sdk-delivery", "actual_delivery_control_binding_and_certificate_revocation", []string{"TestNativeDeliveryControlRejectsForgedTargetsHeadersAndRevokedCertificate"}},
	}
	result := []plan8Check{}
	for _, check := range checks {
		if check.ID != "sdk" {
			result = append(result, check)
			continue
		}
		selected := map[string]bool{}
		for _, name := range check.Tests {
			if selected[name] {
				return nil, errors.New("duplicate SDK acceptance producer")
			}
			selected[name] = true
		}
		for _, group := range groups {
			copy := check
			copy.ID = group.id
			copy.Level = group.scope
			copy.Args = append([]string(nil), check.Args...)
			copy.RequiredEnv = append([]string(nil), check.RequiredEnv...)
			copy.Env = maps.Clone(check.Env)
			copy.Tests = []string{}
			for _, name := range group.tests {
				if selected[name] {
					copy.Tests = append(copy.Tests, name)
					delete(selected, name)
				}
			}
			if len(copy.Tests) > 0 {
				result = append(result, copy)
			}
		}
		if len(selected) > 0 {
			return nil, fmt.Errorf("SDK acceptance producer lacks an explicit bounded group: %v", selected)
		}
	}
	return result, nil
}
