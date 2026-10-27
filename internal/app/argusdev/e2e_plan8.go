package argusdev

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type plan8Check struct {
	ID, Level, Program       string
	Args, RequiredEnv, Tests []string
	Env                      map[string]string
}
type plan8CheckResult struct {
	ID              string            `json:"id"`
	Level           string            `json:"verification_level"`
	Status          string            `json:"status"`
	Log             string            `json:"log"`
	Missing         []string          `json:"missing_prerequisites,omitempty"`
	Tests           map[string]string `json:"tests,omitempty"`
	RequiredTests   []string          `json:"required_tests,omitempty"`
	Error           string            `json:"error,omitempty"`
	DurationSeconds float64           `json:"duration_seconds"`
}
type plan8RunReport struct {
	SchemaVersion       string              `json:"schema_version"`
	RunID               string              `json:"run_id"`
	Suite               string              `json:"suite"`
	Status              string              `json:"status"`
	PlanComplete        bool                `json:"plan_complete"`
	ReleaseEligible     bool                `json:"release_eligible"`
	StartedAt           time.Time           `json:"started_at"`
	FinishedAt          time.Time           `json:"finished_at"`
	SourceBefore        plan8SourceSnapshot `json:"source_before"`
	SourceAfter         plan8SourceSnapshot `json:"source_after"`
	SourceUnchanged     bool                `json:"source_unchanged"`
	Checks              []plan8CheckResult  `json:"checks"`
	FormalGates         []plan8GateStatus   `json:"formal_gates"`
	MissingFormalPhases []string            `json:"missing_formal_phases"`
	Windows             string              `json:"windows_real_testing"`
	Cleanup             string              `json:"cleanup_verification"`
}
type plan8GateStatus struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// This entry point does not equate component success with plan acceptance.
// Full mode fails closed until named deployed/model/release phases exist.
func (a *App) runPlan8(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "verify" {
		return a.verifyPlan8Evidence(ctx, args[1:])
	}
	flags := flag.NewFlagSet("e2e-plan8", flag.ContinueOnError)
	flags.SetOutput(a.stderr)
	suite := flags.String("suite", "full", "engineering|domain|host|sdk|delivery|k8s|k8s-state|drain|control-positive|pod-logs|streams|helm|helm-sources|resources|cache-web|brokers|network-probes|kafka|databases|namespace|completion-core|assignment|csi|backup|operations|runtime-behavior|context|reconnect|components|connector-rotation|full")
	runID := flags.String("run-id", "plan8-"+time.Now().UTC().Format("20060102-150405"), "unique evidence run label")
	list := flags.Bool("list", false, "show selected checks without executing")
	formalEvidence := flags.String("formal-evidence", "", "same-source cases and support/built-in/release matrices for full mode")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`).MatchString(*runID) {
		return fmt.Errorf("%w: invalid Plan8 run label or arguments", errUsage)
	}
	artifactRoot := filepath.Join(a.root, "artifacts")
	checks, err := plan8Checks(*suite, "")
	if err != nil {
		return err
	}
	if *list {
		for _, check := range checks {
			fmt.Fprintf(a.stdout, "%s [%s] %s\n", check.ID, check.Level, strings.Join(check.Tests, ", "))
		}
		return nil
	}
	if err = os.MkdirAll(artifactRoot, 0755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(artifactRoot, *runID+"-")
	if err != nil {
		return err
	}
	checks, err = plan8Checks(*suite, dir)
	if err != nil {
		return err
	}
	report := plan8RunReport{SchemaVersion: "argus.plan8_acceptance_run/v1", RunID: *runID, Suite: *suite, Status: "running", StartedAt: time.Now().UTC(), Checks: []plan8CheckResult{}, FormalGates: []plan8GateStatus{}, Windows: "deferred_by_user_not_verified", Cleanup: "component-owned cleanup; full environment audit not performed", MissingFormalPhases: []string{"formal_case_evidence_not_evaluated"}}
	for i := 1; i <= 76; i++ {
		report.FormalGates = append(report.FormalGates, plan8GateStatus{fmt.Sprintf("P8-A%02d", i), "not_run"})
	}
	write := func() error {
		raw, e := json.MarshalIndent(report, "", "  ")
		if e != nil {
			return e
		}
		return os.WriteFile(filepath.Join(dir, "manifest.json"), append(raw, '\n'), 0600)
	}
	report.SourceBefore, err = a.plan8Snapshot(ctx)
	if err != nil {
		return err
	}
	if err = write(); err != nil {
		return err
	}
	var failures []error
	for _, check := range checks {
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err())
			break
		}
		fmt.Fprintf(a.stdout, "Plan8 %s [%s]\n", check.ID, check.Level)
		result := a.plan8Execute(ctx, dir, check)
		report.Checks = append(report.Checks, result)
		if result.Status != "passed" {
			failures = append(failures, fmt.Errorf("%s: %s", check.ID, result.Status))
		}
		if err = write(); err != nil {
			return err
		}
	}
	report.SourceAfter, err = a.plan8Snapshot(ctx)
	if err != nil {
		failures = append(failures, err)
	} else {
		report.SourceUnchanged = report.SourceBefore.Fingerprint == report.SourceAfter.Fingerprint
		if !report.SourceUnchanged {
			failures = append(failures, errors.New("source or Collector artifacts changed during verification"))
		}
	}
	report.FinishedAt = time.Now().UTC()
	report.Status = "passed_selected_components"
	if len(failures) > 0 {
		report.Status = "failed"
	}
	if *suite == "full" {
		evidence := *formalEvidence
		if evidence == "" {
			evidence = dir
		}
		err := a.verifyPlan8Evidence(ctx, []string{"--evidence", evidence})
		report.MissingFormalPhases = nil
		if err != nil {
			if len(failures) == 0 {
				report.Status = "incomplete"
			}
			report.MissingFormalPhases = []string{"same_source_formal_cases_or_support_builtin_release_matrices_incomplete"}
			failures = append(failures, err)
		} else if len(failures) == 0 {
			report.Status, report.PlanComplete, report.ReleaseEligible = "passed", true, true
		}
		summaryPath := filepath.Join(evidence, "acceptance-summary.json")
		if !filepath.IsAbs(summaryPath) {
			summaryPath = filepath.Join(a.root, summaryPath)
		}
		if raw, e := os.ReadFile(summaryPath); e == nil {
			var summary struct {
				Cases []plan8GateStatus `json:"cases"`
			}
			if json.Unmarshal(raw, &summary) == nil && len(summary.Cases) == 76 {
				report.FormalGates = summary.Cases
			}
		}
	}
	if err = write(); err != nil {
		return err
	}
	fmt.Fprintf(a.stdout, "Plan8 evidence: %s; status=%s; plan_complete=%t; release_eligible=%t\n", filepath.Join(dir, "manifest.json"), report.Status, report.PlanComplete, report.ReleaseEligible)
	return errors.Join(failures...)
}

func (a *App) plan8Execute(ctx context.Context, dir string, check plan8Check) plan8CheckResult {
	start := time.Now()
	out := plan8CheckResult{ID: check.ID, Level: check.Level, Status: "not_run", Log: check.ID + ".jsonl", Tests: map[string]string{}, RequiredTests: append([]string{}, check.Tests...)}
	for _, key := range check.RequiredEnv {
		if os.Getenv(key) == "" {
			out.Missing = append(out.Missing, key)
		}
	}
	if len(out.Missing) > 0 {
		out.Status = "prerequisite_missing"
		return out
	}
	for _, key := range check.RequiredEnv {
		if key == "ARGUS_MONITORING_TEST_DATABASE_URL" || key == "ARGUS_CLICKHOUSE_TEST_ADDRESS" || key == "ARGUS_PLAN8_REDIS_TEST_URL" {
			if e := a.plan8OwnedBackend(ctx, key, os.Getenv(key)); e != nil {
				out.Status = "prerequisite_rejected"
				out.Error = e.Error()
				return out
			}
		}
	}
	file, err := os.OpenFile(filepath.Join(dir, out.Log), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		out.Status = "failed"
		out.Error = err.Error()
		return out
	}
	defer file.Close()
	log := newPlan8EventLog(file, check.Tests)
	caseContext, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	arguments, argumentErr := check.executionArgs()
	if argumentErr != nil {
		err = argumentErr
	} else {
		err = a.runner.RunIO(caseContext, check.Env, nil, log, log, check.Program, arguments...)
	}
	flushErr := log.finish()
	out.Tests = log.tests
	out.DurationSeconds = time.Since(start).Seconds()
	out.Status = "passed"
	if err != nil || flushErr != nil {
		out.Status = "failed"
		out.Error = errors.Join(err, flushErr).Error()
	}
	return out
}

func plan8BaseChecks(suite, dir string) ([]plan8Check, error) {
	goCase := func(id, level, pkg string, tests []string, env map[string]string, required ...string) plan8Check {
		quoted := []string{}
		for _, name := range tests {
			quoted = append(quoted, regexp.QuoteMeta(name))
		}
		return plan8Check{ID: id, Level: level, Program: "go", Args: []string{"test", "-json", "-count=1", "-p", "1", "-run", "^(" + strings.Join(quoted, "|") + ")$", pkg}, Tests: tests, Env: env, RequiredEnv: required}
	}
	pg := "ARGUS_MONITORING_TEST_DATABASE_URL"
	binary := "ARGUS_MONITORING_COLLECTOR_BINARY"
	image := "ARGUS_JOURNALD_SYSTEMD_TEST_IMAGE"
	ch := "ARGUS_CLICKHOUSE_TEST_ADDRESS"
	password := "ARGUS_CLICKHOUSE_TEST_PASSWORD"
	ownPath, servicePath := filepath.Join(dir, "native-own.json"), filepath.Join(dir, "native-services.json")
	engineering := plan8Check{ID: "contracts", Level: "engineering", Program: "go", Args: []string{"run", "./cmd/argus-dev", "contracts", "check"}, Env: map[string]string{"CI": "1", "NO_UPDATE_NOTIFIER": "1"}}
	domain := goCase("domain", "postgres_domain", "./internal/monitoring", []string{"TestPostgresApplicationPreviewCommitIsolation", "TestPostgresServiceDiscoveryIdentityAndForeignKeys", "TestPostgresNamespaceGrantDoesNotFollowRecreatedName", "TestPostgresMonitoringCredentialIsPrivate", "TestPostgresSDKConcurrentDiscoveryAndApplicationBoundaries", "TestPostgresDNSRejectsNoopAndUnverifiedCollectorVersions", "TestPostgresSDKConnectionGuideKeepsApplicationScopeAndCurrentConfiguration", "TestPostgresRejectedSDKDiagnosticsRequireSecretPossessionAndCollectorBinding"}, nil, pg)
	domain.Tests = append(domain.Tests, "TestPostgresSDKRejectsOldDistributionAtCreationAuthenticationAndDiscovery")
	domain.Tests = append(domain.Tests, "TestPostgresKubernetesLogPreparationOutsideActionTransaction", "TestPostgresKubernetesLogPreparedSnapshotCannotBypassRevalidation", "TestPostgresKubernetesLogPreviewReplayDoesNotRepeatExternalReads")
	domain.Tests = append(domain.Tests, "TestPostgresAcceptanceSameNamedObjectsRemainIsolatedAcrossEnterprises", "TestPostgresAcceptanceCreateFailureRollsBackTargetCredentialAssignmentAndOutbox")
	domain.Tests = append(domain.Tests, "TestPostgresHostFilesKeepContainerLogsInKubernetesEntry")
	registryUpgrade := goCase("authorization-registry-upgrade", "actual_pg_schema19_builtin_registry_idempotence_concurrency_audit_rollback_cli_and_custom_scope_preservation", "./internal/authorization", []string{"TestOwnedRegistryUpgradeIsAtomicIdempotentAndScoped", "TestOwnedRegistryConcurrentStartupReconcilesOnce", "TestOwnedRegistryAuditFailureRollsBackAllChanges", "TestOwnedRegistryRejectsNewerRegistryWithoutMutation", "TestOwnedRegistryMigrationCommandRepairsExistingRoles", "TestPostgresEnterpriseCreationAndRegistryStartupSerialize", "TestPostgresEnterpriseCreationRejectsNewerRegistry"}, nil, pg)
	registryUpgrade.Args = append(registryUpgrade.Args, "./internal/platform")
	modelRecords := goCase("model-record-concurrency", "actual_pg_dispatch_usage_and_step_row_conflicts_atomic_provenance_and_stale_task_fence", "./internal/agent", []string{"TestModelDispatchReceiptSurvivesConcurrentRowObservation", "TestModelDispatchReceiptRejectsReplacedTaskLease", "TestModelOutcomeReceiptRetainsKnownUsageDuringConcurrentObservation", "TestModelStepReceiptSurvivesConcurrentObservation", "TestModelOutcomeAndStepReceiptsRejectReplacedTaskLease"}, map[string]string{"ARGUS_P5_TEST_DATABASE_URL": os.Getenv(pg)}, pg)
	metricScan := goCase("metric-scan-pruning", "actual_clickhouse_selected_series_index_pruning_with_400000_unrelated_samples_and_unchanged_scope_budget", "./internal/telemetry", []string{"TestClickHouseSelectedSeriesUsesBoundedScanAmidUnrelatedSamples"}, nil, ch, password)
	runtimeRoles := goCase("runtime-database-roles", "actual_pg_late_role_reconciliation_readonly_trust_bootstrap_collector_enrollment_rotation_sdk_writer_audit_and_forbidden_writes", "./internal/telemetry", []string{"TestPostgresTelemetryRuntimeRolesAndLateRoleReconciliation", "TestPostgresTelemetryEnrollmentDeadlineUsesOneDatabaseClock", "TestPostgresTelemetryEnrollmentLifetimeBoundsAndRollback", "TestPostgresTelemetryEnrollmentConsumptionUsesCurrentDatabaseTime", "TestCollectorPKIRepairUsesDatabaseDeadlineAndRollsBackRevocation"}, nil, pg)
	runtimeRoles.Args = append(runtimeRoles.Args, "./internal/app/argusctl")
	helmBootstrap := goCase("installer-bootstrap", "actual_helm_migration_handoff_runtime_prerequisite_and_ordinary_install_rollback", "./internal/app/argusctl", []string{"TestLiveHelmBootstrapApplySeparatesRuntimeReadiness"}, map[string]string{"ARGUS_PLAN8_HELM_BOOTSTRAP": "1"}, "ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	helmBootstrap.Tests = append(helmBootstrap.Tests, "TestLiveInstallationToolsHookUsesServiceAccountAndRejectsRevocation")
	helmBootstrap.Env["ARGUS_PLAN8_INSTALL_TOOLS_LIVE"] = "1"
	helmBootstrap.Env["ARGUS_PLAN8_INSTALL_TOOLS_EVIDENCE"] = filepath.Join(dir, "native-installation-tools.json")
	helmBootstrap.RequiredEnv = append(helmBootstrap.RequiredEnv, "ARGUS_PLAN8_INSTALL_TOOLS_CONTEXT", "ARGUS_PLAN8_INSTALL_TOOLS_IMAGE")
	tools := goCase("tools", "native_tools_and_postgres", "./internal/toolgateway", []string{"TestMonitoringDomainsDiscoveryUsesSharedCategoriesAndHiddenCommits", "TestMonitoringPrivateInputRejectedAtEveryNestingLevel", "TestNativeToolVersionFreezesSchemaPermissionsAndExecutionContract", "TestPostgresMonitoringNativeDomainsIsolationPreviewAndPresentationRevocation"}, nil, pg)
	tools.Tests = append(tools.Tests, "TestUndescribedArgumentsCannotBypassDiscoveryError")
	tools.Tests = append(tools.Tests, "TestMonitoringPreviewWorkloadSchemaSeparatesObservedIdentity")
	own := goCase("own", "native_linux_container", "./internal/collectormanager", []string{"TestNativeOwnHostAndCollectorMetricsHaveIndependentLifecycle"}, map[string]string{"ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_BUILTIN_METRIC_EVIDENCE": ownPath}, binary)
	services := goCase("services", "native_linux_systemd_container", "./internal/collectormanager", []string{"TestNativeHostProcessAndRealSystemdServiceMetrics"}, map[string]string{"ARGUS_PROCESS_SERVICE_METRIC_EVIDENCE": servicePath}, binary, image)
	services.Tests = append(services.Tests, "TestNativeSystemdFileLogViewPreservesHostPathsAndPrivateRuntime")
	services.Env["ARGUS_MONITORING_NATIVE_TARGETS"] = "1"
	services.Env["ARGUS_SYSTEMD_FILELOG_VIEW_EVIDENCE"] = filepath.Join(dir, "native-systemd-file-view.json")
	sdk := goCase("sdk", "real_sdk_backend_and_portal_control_registration_fixture", "./internal/collectormanager", []string{"TestNativeSDKApplicationPipelineDiscoveryAPMLogsAndKeyRevocation", "TestNativeSDKGRPCProtocolsDiscoverServicesAndFenceRevokedKeys", "TestNativeJaegerThriftHTTPAuthenticationDiscoveryAndRotation", "TestNativeSDKRejectsInvalidServiceIdentityBeforeBatching"}, map[string]string{"ARGUS_MONITORING_NATIVE_PIPELINE": "1", "ARGUS_MONITORING_NATIVE_BROWSER": "1"}, pg, ch, password, binary)
	sdk.Env["ARGUS_PLAN8_PICKER_BROWSER_EVIDENCE"] = filepath.Join(dir, "native-picker-browser")
	sdk.Tests = append(sdk.Tests, "TestNativeSDKCapacityProfileStoresMeasuredSeriesAndHonorsRevocation", "TestNativeDedicatedHTTP32TargetFrozenCapacity")
	sdk.Tests = append(sdk.Tests, "TestNativeSDKQueueCapacityRecoversAcceptedPointsAfterOutageAndCrash")
	sdk.Env["ARGUS_PLAN8_QUEUE_CAPACITY_EVIDENCE"] = filepath.Join(dir, "capacity-queue-restart-component.json")
	sdk.Tests = append(sdk.Tests, "TestNativeQueueFilesystemFullRejectsThenRecoversAcknowledgedPoints", "TestNativeSDKRejectsWrongTLSIdentityAndHonorsActualQuerySeriesQuota")
	sdk.Env["ARGUS_PLAN8_DISK_FAULT_EVIDENCE"] = filepath.Join(dir, "native-disk-full-component.json")
	sdk.Env["ARGUS_PLAN8_TLS_QUOTA_EVIDENCE"] = filepath.Join(dir, "native-tls-query-quota-component.json")
	sdk.Tests = append(sdk.Tests, "TestNativeIngestQuotaExplicitlyRejectsAndRecovers")
	sdk.Env["ARGUS_PLAN8_QUOTA_EVIDENCE"] = filepath.Join(dir, "native-ingest-quota-component.json")
	sdk.Env["ARGUS_MONITORING_NATIVE_TARGETS"] = "1"
	sdk.Env["ARGUS_PLAN8_DEDICATED_CAPACITY_EVIDENCE"] = filepath.Join(dir, "capacity-dedicated-component.json")
	sdk.Tests = append(sdk.Tests, "TestNativeFiveMinuteExportFailurePersistsGapAcrossRestartAndControlOutage", "TestNativeDeliveryControlRejectsForgedTargetsHeadersAndRevokedCertificate")
	sdk.Env["ARGUS_PLAN8_CAPACITY_PROFILE"] = "local-linux-amd64-v1"
	sdk.Env["ARGUS_PLAN8_CAPACITY_EVIDENCE"] = filepath.Join(dir, "capacity-sdk-component.json")
	sdk.Env["ARGUS_PLAN8_DELIVERY_EVIDENCE"] = filepath.Join(dir, "native-delivery-five-minute.json")
	deliveryStatus := goCase("delivery-status", "actual_owned_postgres_durable_outbox_exhaustion_recovery_alert_scope_and_notifications", "./internal/alerting", []string{"TestPostgresCollectorDeliveryReportOutboxAlertIsolationAndReplay", "TestCollectorGapNotificationKeepsTerminalAndRetryCountsDistinct"}, nil, pg)
	deliveryIdentity := goCase("delivery-identity", "native_identity_ledger_counter_semantics_and_100_loopback_observations_without_connection_growth", "./...", []string{"TestDeliveryLedgerSurvivesRestartAndSeparatesRetriedRefusals", "TestDeliveryLedgerRejectsCorruptHistoryInsteadOfResettingCounts", "TestDeliveryMetricParsingRequiresIdentityAndFiniteCounters", "TestDeliveryObservationDoesNotAccumulateIdleConnections", "TestDeliveryConfigRejectsUntrustedControlReportDestinations"}, nil)
	deliveryIdentity.Args = append([]string{"-C", "internal/otelcol/argusidentity"}, deliveryIdentity.Args...)
	views := goCase("views", "actual_capture_writer_clickhouse_queries", "./internal/telemetry", []string{"TestClickHouseOwnTemplateCapturedMetrics", "TestClickHouseProcessServiceTemplateCapturedMetrics"}, map[string]string{"ARGUS_BUILTIN_METRIC_EVIDENCE": ownPath, "ARGUS_PROCESS_SERVICE_METRIC_EVIDENCE": servicePath}, ch, password)
	views.Tests = append(views.Tests, "TestClickHouseStructuredFileLogBodyRemainsSearchableWithoutLosingFields")
	k8sPath := filepath.Join(dir, "native-kubernetes.json")
	k8s := goCase("k8s", "actual_api_kubelet_apiserver_coredns_component", "./internal/collectormanager", []string{"TestNativeKubernetesOwnMetricsUseRealAPIAndKubelet"}, map[string]string{"ARGUS_KUBERNETES_METRIC_EVIDENCE": k8sPath}, binary, "ARGUS_KUBERNETES_METRICS_CONTEXT", "ARGUS_KUBERNETES_METRICS_NODE")
	control := goCase("k8s-control", "actual_control_component_availability_no_fabricated_samples", "./internal/collectormanager", []string{"TestNativeKubernetesControlMetricsKeepUnavailableEndpointsUnobserved"}, map[string]string{"ARGUS_KUBERNETES_CONTROL_METRIC_EVIDENCE": filepath.Join(dir, "native-kubernetes-control.json")}, binary, "ARGUS_KUBERNETES_METRICS_CONTEXT", "ARGUS_KUBERNETES_METRICS_NODE")
	k8sViews := goCase("k8s-views", "actual_kubernetes_capture_writer_clickhouse_queries", "./internal/telemetry", []string{"TestClickHouseKubernetesTemplateCapturedMetrics"}, map[string]string{"ARGUS_KUBERNETES_METRIC_EVIDENCE": k8sPath}, ch, password)
	workspace := goCase("workspace", "postgres_typed_source_revocation_fake_compute_not_real_pvc", "./internal/workspace", []string{"TestPlan8WorkspaceTypedSourceGrantRevocation", "TestWorkspaceImportProvenanceSurvivesMissingMetadataAndRevocation"}, map[string]string{"ARGUS_P5_TEST_DATABASE_URL": os.Getenv(pg)}, pg)
	resourcePath := filepath.Join(dir, "native-postgres-probes.json")
	resources := goCase("resources", "actual_postgresql_http_tcp_certificate_native_targets", "./internal/collectormanager", []string{"TestNativePostgresAndHTTPNetworkCertificateProbes"}, map[string]string{"ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_POSTGRES_PROBE_METRIC_EVIDENCE": resourcePath}, binary)
	resources.Tests = append(resources.Tests, "TestNativeRedisDedicatedReceiverUsesMetadataOnlyACL")
	resourceViews := goCase("resource-views", "actual_postgres_probe_capture_writer_clickhouse_queries", "./internal/telemetry", []string{"TestClickHousePostgresProbeTemplateCapturedMetrics"}, map[string]string{"ARGUS_POSTGRES_PROBE_METRIC_EVIDENCE": resourcePath}, ch, password)
	cachePath := filepath.Join(dir, "native-cache-web.json")
	cache := goCase("cache-web", "actual_memcached_apache_haproxy_native_targets", "./internal/collectormanager", []string{"TestNativeMemcachedApacheAndHAProxyTargets"}, map[string]string{"ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_CACHE_WEB_METRIC_EVIDENCE": cachePath}, binary)
	cacheViews := goCase("cache-web-views", "actual_cache_web_capture_writer_clickhouse_queries", "./internal/telemetry", []string{"TestClickHouseCacheWebTemplateCapturedMetrics"}, map[string]string{"ARGUS_CACHE_WEB_METRIC_EVIDENCE": cachePath}, ch, password)
	brokerPath := filepath.Join(dir, "native-broker-coordination.json")
	brokers := goCase("brokers", "actual_rabbitmq_read_only_management_zookeeper_mntr", "./internal/collectormanager", []string{"TestNativeRabbitMQAndZooKeeperTargets"}, map[string]string{"ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_BROKER_COORDINATION_METRIC_EVIDENCE": brokerPath}, binary)
	brokerViews := goCase("broker-views", "actual_broker_coordination_capture_writer_clickhouse_queries", "./internal/telemetry", []string{"TestClickHouseBrokerCoordinationTemplateCapturedMetrics"}, map[string]string{"ARGUS_BROKER_COORDINATION_METRIC_EVIDENCE": brokerPath}, ch, password)
	networkPath := filepath.Join(dir, "native-network-probes.json")
	networkFailure := filepath.Join(dir, "native-network-loss.json")
	network := goCase("network-probes", "actual_coredns_udp_tcp_icmp_nonroot_permission_failure_and_recovery", "./internal/collectormanager", []string{"TestNativeDNSAndICMPActualTargets"}, map[string]string{"ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_NETWORK_PROBE_METRIC_EVIDENCE": networkPath, "ARGUS_NETWORK_PROBE_FAILURE_EVIDENCE": networkFailure}, binary)
	networkViews := goCase("network-probe-views", "actual_network_capture_writer_clickhouse_queries_and_no_false_zero_latency", "./internal/telemetry", []string{"TestClickHouseNetworkProbeTemplateCapturedMetrics", "TestClickHouseNetworkLossDoesNotRenderZeroLatency"}, map[string]string{"ARGUS_NETWORK_PROBE_METRIC_EVIDENCE": networkPath, "ARGUS_NETWORK_PROBE_FAILURE_EVIDENCE": networkFailure}, ch, password)
	kafkaPath := filepath.Join(dir, "native-kafka.json")
	kafka := goCase("kafka", "actual_kraft_sasl_read_only_metadata_and_consumer_offsets", "./internal/collectormanager", []string{"TestNativeKafkaReadOnlyMetadataAndConsumerOffsets"}, map[string]string{"ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_KAFKA_METRIC_EVIDENCE": kafkaPath}, binary)
	kafkaViews := goCase("kafka-views", "actual_kafka_capture_writer_clickhouse_queries", "./internal/telemetry", []string{"TestClickHouseKafkaTemplateCapturedMetrics"}, map[string]string{"ARGUS_KAFKA_METRIC_EVIDENCE": kafkaPath}, ch, password)
	drainUnit := goCase("drain-unit", "drain_uid_storage_and_read_only_invariants", "./internal/kubernetesinventory", []string{"TestDrainDryRunPDBRecoveryAndReadOnlyObservation", "TestDrainUnsafeStorageAndUnmanagedPodsNeverCordon", "TestDrainSnapshotChangesRejectBeforeSideEffects", "TestDrainRejectsForgedCompletionAndNonDrainContinuation", "TestDrainPVCBindingIsFrozenAndNodeLocalDataBlocksExecution", "TestDrainStaticAndDaemonSetPodsAreExplicitlyRetained"}, nil)
	drainProtocol := goCase("drain-protocol", "connector_explicit_drain_write_phase", "./internal/app/connector", []string{"TestKubernetesMutationRejectsOldProtocolBeforeAPIExecution", "TestKubernetesDrainRejectsMixedReadAndWritePhases"}, nil)
	drainLive := goCase("drain-live", "postgres_real_owned_two_node_drain_workflow", "./internal/kubernetesworkbench", []string{"TestLiveNodeDrainApprovalPDBRecoveryRevocationAndLostResponse"}, nil, pg, "ARGUS_KUBERNETES_DRAIN_CONTEXT", "KUBECONFIG")
	controlPositivePath := filepath.Join(dir, "native-control-positive.json")
	controlPositive := goCase("control-positive", "actual_reachable_control_components_with_native_auth_lifecycle", "./internal/collectormanager", []string{"TestNativeKubernetesControlMetricsUseReachableEndpoints"}, map[string]string{"ARGUS_KUBERNETES_METRICS_CONTEXT": os.Getenv("ARGUS_KUBERNETES_COMPONENTS_CONTEXT"), "ARGUS_KUBERNETES_METRICS_NODE": os.Getenv("ARGUS_KUBERNETES_COMPONENTS_NODE"), "ARGUS_KUBERNETES_METRICS_API_PORT": os.Getenv("ARGUS_KUBERNETES_COMPONENTS_API_PORT"), "ARGUS_KUBERNETES_METRICS_NETWORK": os.Getenv("ARGUS_KUBERNETES_COMPONENTS_NETWORK"), "ARGUS_KUBERNETES_CONTROL_METRIC_EVIDENCE": controlPositivePath}, binary, "ARGUS_KUBERNETES_COMPONENTS_CONTEXT", "ARGUS_KUBERNETES_COMPONENTS_NODE", "ARGUS_KUBERNETES_COMPONENTS_API_PORT", "ARGUS_KUBERNETES_COMPONENTS_NETWORK", "KUBECONFIG")
	controlPositiveViews := goCase("control-positive-views", "actual_control_component_capture_writer_clickhouse_queries", "./internal/telemetry", []string{"TestClickHouseKubernetesControlTemplatesCapturedMetrics"}, map[string]string{"ARGUS_KUBERNETES_CONTROL_METRIC_EVIDENCE": controlPositivePath}, ch, password)
	bootstrapRBAC := goCase("bootstrap-rbac", "actual_installer_manifest_read_only_metrics_authorization", "./internal/collectormanager", []string{"TestCollectorBootstrapMetricsRBACIsReadOnlyAndSystemScoped", "TestLiveCollectorBootstrapMetricsAuthorization"}, nil, "ARGUS_KUBERNETES_COMPONENTS_CONTEXT", "KUBECONFIG")
	logUnit := goCase("pod-log-unit", "pinned_log_identity_and_lossless_second_boundary", "./internal/kubernetespodlogs", []string{"TestLogContinuationRetainsDuplicatesAndSecondPrecision", "TestLogContinuationRejectsRotatedOrReorderedPrefix", "TestLogContinuationWaitsForPartialLineAndMarksBudget", "TestPinnedLogReadRejectsReplacementAfterNativeResponse", "TestLogCredentialPinsServerAndRejectsLocalExecutionAndFiles"}, nil)
	logProtocol := goCase("pod-log-protocol", "authenticated_native_connector_protocol_and_projection", "./internal/app/connector", []string{"TestPinnedLogCommandProtocolAndPrivateResultProjection"}, nil)
	logDomain := goCase("pod-log-domain", "postgres_log_tickets_scope_capacity_and_retirement", "./internal/kubernetesworkbench", []string{"TestPostgresPodLogSessionsBindTicketsAndRecheckRevocation", "TestPostgresPodLogSessionsRejectRestartExpiryAndLogout", "TestPostgresPodLogSessionCapacityAndRetirement"}, nil, pg)
	logHTTP := goCase("pod-log-http", "actual_http_cookie_csrf_ticket_headers_and_revocation", "./internal/transport/httpapi", []string{"TestPostgresPodLogSessionHTTPAuthenticatesTicketsCSRFAndScope"}, nil, pg)
	logLive := goCase("pod-log-live", "actual_owned_pod_logs_previous_restart_and_scope", "./internal/kubernetesworkbench", []string{"TestLivePodLogSessionsCurrentPreviousRestartAndRevocation"}, nil, pg, "ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	streamUnit := goCase("stream-unit", "native_uid_security_service_packet_and_backpressure", "./internal/kubernetesstream", []string{"TestTargetPinsGenerationSecurityAndNamespace", "TestForwardPacketBoundsAndServiceOwnership", "TestTerminalStateSurvivesFullOutputBacklog"}, nil)
	streamClient := goCase("stream-client", "loopback_listener_and_token_free_url_contract", "./internal/kubernetesstreamclient", []string{"TestForwardClientRejectsRemoteListenersAndUnsafeEndpoints"}, nil)
	streamHub := goCase("stream-hub", "connector_capability_negotiation", "./internal/connector", []string{"TestKubernetesStreamsRequireAdvertisedConnectorCapability"}, nil)
	streamConnector := goCase("stream-connector", "native_connector_target_and_protocol_binding", "./internal/app/connector", []string{"TestKubernetesInteractiveProtocolRejectsUnboundAndCrossProtocolTargets"}, nil)
	streamConnector.Tests = append(streamConnector.Tests, "TestCollectorReconciliationReplaysOnlyRequestedPrivateJournalReceipts", "TestCollectorCompletionPersistsWhenTransportHasAlreadyEnded")
	streamDomain := goCase("stream-domain", "postgres_scope_ticket_execution_privilege_credential_and_capacity", "./internal/kubernetesworkbench", []string{"TestPostgresKubernetesStreamTicketsAreBoundAndExecutionClaimIsAtomic", "TestPostgresKubernetesStreamRevocationRouteAndPrivilege", "TestPostgresKubernetesStreamCredentialRotationAndDelivery", "TestPostgresKubernetesStreamCapacityClosingAndRetirement"}, nil, pg)
	streamGateway := goCase("stream-gateway", "real_http_websocket_tcp_and_mtls_peer_with_controlled_destination_adapter", "./internal/kubernetesstreamgateway", []string{"TestPostgresKubernetesWebSocketTicketsRevocationAndAudit", "TestPostgresKubernetesForwardClientMultipleTCPConnectionsAndExplicitClose", "TestPostgresKubernetesPeerMTLSBindingReplayAndRevocation", "TestPostgresKubernetesStreamHTTPIdentityCSRFAndGatewayAttach"}, nil, pg)
	streamLive := goCase("stream-live", "actual_kubelet_tty_resize_service_spdy_tcp_and_uid_invalidation", "./internal/kubernetesstream", []string{"TestLiveNativeTerminalAndMultiplexedServiceForward"}, nil, "ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	helmUnit := goCase("helm-unit", "bounded_native_archive_schema_renderer_storage_and_public_metadata", "./internal/kuberneteshelm", []string{"TestHelmArchiveAndSealedPlanRejectSubstitution", "TestHelmPublicDiffContainsPathsWithoutPrivateValues", "TestHelmRendererIsolatesValuesLogsAndRejectsExternalSchema", "TestHelmRendererRejectsRunawayTemplateAndHonorsCancellation", "TestHelmMetadataAndStorageDoNotEscapeNamespaceOrDecodeBudgets", "TestHelmStreamingBodyBudgetAndManagedFieldsComparison"}, nil)
	helmCore := goCase("helm-command", "persistent_command_excludes_material_and_rejects_protocol_mode_confusion", "./internal/connector", []string{"TestHelmCommandsRejectPersistedSecretsAndProtocolModeConfusion"}, nil)
	helmConnector := goCase("helm-connector", "short_lived_per_write_nonce_epoch_operation_grants_and_protocol", "./internal/app/connector", []string{"TestHelmConnectorPermitsAreSingleWriteBoundAndExpire", "TestHelmConnectorRejectsOldSchemaAndPrivateQueryMaterial"}, nil)
	helmDomain := goCase("helm-domain", "postgres_encrypted_inputs_replay_namespace_uid_object_permissions_fences_and_unknown", "./internal/kubernetesworkbench", []string{"TestPostgresHelmPrivatePreviewReplayAndInputBinding", "TestPostgresHelmChecksEveryRenderedObjectAndNamespaceUID", "TestPostgresHelmRevocationStopsWritesAndUnknownIsNotReplayed", "TestPostgresHelmReleaseLockFencesConcurrentAndUnknownOperations"}, nil, pg)
	helmHTTP := goCase("helm-http", "actual_http_csrf_encrypted_inputs_no_store_replay_and_namespace_scope", "./internal/transport/httpapi", []string{"TestPostgresHelmHTTPProtectedInputsCSRFReplayAndNamespaceScope"}, nil, pg)
	helmLive := goCase("helm-live", "actual_native_lifecycle_hooks_random_values_crd_namespace_conflicts_and_deletion_dependencies", "./internal/kuberneteshelm", []string{"TestLiveHelmNativeLifecycleFrozenValuesHooksAndVersionConflict", "TestLiveHelmDeferredCRDAndNamespaceObjects", "TestLiveHelmTemplateCRDDeletionRechecksDependencies"}, nil, "ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	helmSources := goCase("helm-source-unit", "actual_tls_repository_oci_protocol_auth_ca_digest_redirect_policy_and_budget", "./internal/kuberneteshelm", []string{"TestHelmSourceValidationAndDefaultNetworkPolicy", "TestHelmRepositoryResolutionPinsIdentityAndDoesNotUseAmbientCredentials", "TestHelmSourceRedirectsStripCredentialsAndEnforceBudgetsAndRevalidation", "TestHelmOCIUsesScopedAuthenticationAndVerifiesManifestAndLayerDigests", "TestHelmOCICrossOriginTokenRealmRequiresExplicitTrust"}, nil)
	helmSourceLive := goCase("helm-source-live", "actual_owned_tls_repository_distribution_registry_and_offline_source_native_execution", "./internal/kuberneteshelm", []string{"TestLiveHelmRepositoryAndOCIExecuteAfterSourcesAreOffline"}, map[string]string{"ARGUS_HELM_SOURCE_REGISTRY": "1"}, "ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	helmSourceDomain := goCase("helm-source-domain-live", "actual_pg_source_credential_erasure_approval_worker_per_write_grants_and_kubernetes", "./internal/kubernetesworkbench", []string{"TestLivePostgresHelmSourceApprovalErasesOriginCredentialsAndPinsBytes"}, map[string]string{"ARGUS_HELM_SOURCE_REGISTRY": "1"}, pg, "ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	tools.Tests = append(tools.Tests, "TestHelmToolSchemaKeepsPrivateMaterialOutOfModelContext")
	tools = goCase(tools.ID, tools.Level, "./internal/toolgateway", tools.Tests, nil, pg)
	databasePath := filepath.Join(dir, "native-databases")
	databases := goCase("databases-native", "actual_read_only_mongodb_elasticsearch_sqlserver_oracle_auth_recovery_and_lifecycle", "./internal/collectormanager", []string{"TestNativeMongoDBTarget", "TestNativeElasticsearchTarget", "TestNativeSQLServerTarget", "TestNativeOracleTarget"}, map[string]string{"ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_DATABASE_METRIC_EVIDENCE_DIR": databasePath}, binary)
	databaseViews := goCase("databases-views", "actual_native_capture_writer_clickhouse_24_default_queries", "./internal/telemetry", []string{"TestClickHouseRemainingDatabaseTemplateCapturedMetrics"}, map[string]string{"ARGUS_DATABASE_METRIC_EVIDENCE_DIR": databasePath}, ch, password)
	namespaceKeys := goCase("namespace-key-unit", "canonical_namespace_source_selectors", "./internal/telemetry/sourcekey", []string{"TestNamespaceSelectorsAreCanonicalAndNeverBecomeWholeSource"}, nil)
	namespaceGrants := goCase("namespace-grants", "actual_pg_namespace_metric_source_authority_and_frozen_uid_keys", "./internal/monitoring", []string{"TestPostgresNamespaceMetricSourcesKeepUIDBoundSelectors"}, nil, pg)
	namespaceLifecycle := goCase("namespace-lifecycle", "actual_pg_namespace_snapshot_lease_route_and_foreground_races_and_confirmed_drain_cancel", "./internal/kubernetesworkbench", []string{"TestPostgresNamespaceInventorySnapshotAndRaceFences", "TestPostgresDrainCancelPreviewConfirmAndVersionFence"}, nil, pg)
	namespaceLive := goCase("namespace-live", "actual_kubernetes_namespace_recreation_paginated_metadata_sync_and_pg_uid_authority", "./internal/kubernetesworkbench", []string{"TestLiveNamespaceInventorySynchronizesActualUIDReplacement"}, nil, pg, "ARGUS_KUBERNETES_INVENTORY_CONTEXT")
	namespaceQuery := goCase("namespace-query", "actual_pg_uid_source_admission_uninstall_dependencies_and_writer_clickhouse_namespace_aggregation_catalog_overview", "./internal/telemetry/...", []string{"TestPostgresMetricNamespaceUIDCannotFollowRecreatedName", "TestPostgresSourceApplyWindowIsRetryableAndRetiredConfigurationIsFenced", "TestClickHouseNamespaceMetricsFilterBeforeAggregationCatalogAndOverview"}, nil, pg, ch, password)
	namespaceQuery.Tests = append(namespaceQuery.Tests, "TestClickHouseMaximumSourceIsolationAcrossSignals", "TestClickHouseMaximumNamespaceMetricsFilterBeforeAggregationCatalogAndOverview", "TestClickHouseSourceTransportKeepsEmptyFacetedMixedAndDuplicateSets")
	namespaceQuery.Tests = append(namespaceQuery.Tests, "TestMixedNamespaceAttributionRetainsClusterDataWithoutGrantingNamespace", "TestActualOpaqueKubernetesUIDIsResolvedOnlyFromTrustedInventory", "TestNamespaceMetricQualificationPreservesCoverageAndBudgetSemantics")
	apmSources := goCase("apm-multisource", "actual_clickhouse_application_service_aggregation_duplicate_span_identity_and_source_auth_boundary", "./internal/telemetry", []string{"TestAPMApplicationAggregatesSourcesWithoutCountingDuplicateSpans", "TestAPMClickHouseReceivedSamples", "TestAPMStableServiceIdentitySeparatesSameNamedNamespaces"}, nil, ch, password)
	claimUnit := goCase("claim-instances-unit", "fixed_instance_target_responsibilities_verified_host_aliases_and_unbound_nodes", "./internal/collectionclaim", []string{"TestInstanceResponsibilitiesReplaceDisabledProfilesAndKeepTargetIdentity", "TestNodeClaimsAliasOnlyVerifiedHostsAndRetainUnboundNodes"}, nil)
	assignments := goCase("assignment-lifecycle", "actual_pg_candidate_cutover_claims_and_unchanged_route_transport_preservation", "./internal/monitoring", []string{"TestPostgresCollectionMigrationKeepsVerifiedExecutionUntilCutover", "TestPostgresInstanceClaimMigrationRollbackAndPrimaryOwnership", "TestPostgresMonitoringLeaseAndOutcomeFencing", "TestPostgresReceiverChangesPreserveExistingRouteAndTunnel"}, nil, pg)
	csi := goCase("csi-expansion-live", "actual_mounted_csi_pvc_expansion_via_approval_worker_final_capacity_and_preserved_file", "./internal/kubernetesworkbench", []string{"TestLiveCSIExpansionApprovalWaitsForActualCapacity"}, nil, pg, "ARGUS_KUBERNETES_INVENTORY_CONTEXT", "ARGUS_CSI_EXPANSION_DRIVER")
	backup := goCase("postgres-backup-restore", "actual_owned_database_upgrade_dump_restore_cross_drive_transfer_openbao_shared_reader_and_preserved_pki_recovery", "./internal/storage/postgres", []string{"TestPlan8PostgresUpgradeInterruptedDDLAndBackupRestore", "TestLivePodFileTransferPreservesBinaryAndArchiveAcrossLocalDrives", "TestLiveOpenBaoEncryptedBackupRestoresSourceKeyAfterRestart", "TestLiveRestoreRetainsManagedIssuerAndClientServerChains"}, map[string]string{"ARGUS_PLAN8_KUBECTL_TRANSFER": "1", "ARGUS_PLAN8_OPENBAO_RECOVERY": "1", "ARGUS_PLAN8_PKI_RECOVERY": "1"}, pg, "ARGUS_PLAN8_BACKUP_CONTAINER", "ARGUS_KUBERNETES_INVENTORY_CONTEXT", "ARGUS_KUBERNETES_POLICY_CONTEXT")
	backup.Args = append(backup.Args, "./internal/app/argusctl")
	nativeOperations := goCase("k8s-native-operations", "actual_api_pagination_uid_dry_run_scale_restart_history_job_cronjob_service_pvc_custom_resources_and_crd_dependencies", "./internal/kubernetesinventory", []string{"TestLiveInventoryPaginationProjectionAndNamespaceRecreation", "TestLiveMutationDryRunConflictScaleRestartAndDeletion", "TestLiveCustomTypeDiscoveryAndRecreation", "TestLiveCRDDeleteRefusesExistingInstances", "TestLiveWorkloadRollbackUsesActualControllerHistory", "TestLiveManualJobFromCronJobChecksSourceAndActualCompletion", "TestLiveJobApprovalDigestSurvivesGeneratedIdentityAndCompletes", "TestLiveServiceAllocationAndPVCBindingPreserveApprovedIntent", "TestLivePVCUnsupportedExpansionAndShrinkHaveNoWrite"}, nil, "ARGUS_KUBERNETES_INVENTORY_CONTEXT", "ARGUS_PVC_NONEXPANDING_STORAGE_CLASS")
	operationWorkflow := goCase("k8s-operation-workflow", "actual_pg_approval_worker_api_controller_rollbacks_cronjob_rbac_scope_and_native_escalation_denial", "./internal/kubernetesworkbench", []string{"TestLiveKubernetesOperationWorkflow", "TestLiveRBACApprovalScopeDelegationAndNativeAPIRejection"}, nil, pg, "ARGUS_KUBERNETES_INVENTORY_CONTEXT", "ARGUS_PVC_NONEXPANDING_STORAGE_CLASS")
	policyTraffic := goCase("network-policy-traffic", "actual_pg_approval_and_cni_enforced_allow_deny_selective_allow_delete_traffic", "./internal/kubernetesworkbench", []string{"TestLiveNetworkPolicyApprovalControlsActualTraffic"}, nil, pg, "ARGUS_KUBERNETES_POLICY_CONTEXT", "KUBECONFIG")
	hpaBehavior := goCase("hpa-cpu-behavior", "actual_pg_approval_and_cpu_metrics_target_scale_up_and_down", "./internal/kubernetesworkbench", []string{"TestLiveHPAApprovalObservesRealCPUScaleUpAndDown"}, nil, pg, "ARGUS_KUBERNETES_HPA_CONTEXT")
	admission := goCase("context-admission", "actual_pg_fixed_input_hard_boundary_rejection_without_run_or_tool_side_effects", "./internal/conversation", []string{"TestMessageAdmissionRejectsFixedInputAtRuntimeBoundary"}, map[string]string{"ARGUS_P5_TEST_DATABASE_URL": os.Getenv(pg)}, pg)
	contextWire := goCase("context-runtime-wire", "actual_pg_request_admission_and_durable_inference_budget_with_controlled_model_endpoint", "./internal/agent", []string{"TestPreflightMeasuresSameFixedRequestAsAgentRuntime", "TestPostgresInferenceBudgetFinalizationAndDurableRecovery"}, map[string]string{"ARGUS_P5_TEST_DATABASE_URL": os.Getenv(pg)}, pg)
	contextWire.Args = append([]string{"test", "-tags=m4e2e"}, contextWire.Args[1:]...)
	actionBatch := goCase("action-batch", "actual_owned_postgres_preview_read_batch_confirmation_barrier_external_write_denial_and_two_crash_windows", "./internal/agent", []string{"TestPostgresActionPreviewCompletesReadTailAndRejectsExternalWriteBeforeWaiting", "TestPostgresActionBatchRecoveryClosesBothPreviewCrashWindows", "TestPreviewRecoveryRestoresHostControlAndDeterministicVerification", "TestCommittedPlanBarrierRecoversWithoutModelOrLaterToolDispatch", "TestUserInputDurablyPausesAllFourModeCombinations", "TestNativeQueryDeliveryWaitsForCommittedSourceAndRetainsRecoveryIntent"}, map[string]string{"ARGUS_P5_TEST_DATABASE_URL": os.Getenv(pg)}, pg)
	disconnect := goCase("connector-disconnect", "actual_pg_connection_fences_atomic_projections_private_dispatch_and_verified_route_failures", "./internal/connector", []string{"TestPostgresDisconnectProjectsAtomicallyBeforeNewSession", "TestPostgresGatewayMonitoringDispatchUsesConfiguredPrivateCredentialChannel", "TestPostgresHeartbeatFencesEveryProjectionAndRollsBackInvalidFrames", "TestPostgresCollectorFailurePreservesOnlyVerifiedEffectiveRoute", "TestControlSessionTimesOutForSilentGateway", "TestControlSessionRequiresAdvancingAcknowledgements", "TestControlHandshakeHasBoundedWelcomeWait"}, nil, pg)
	disconnect.Args = append(disconnect.Args, "./internal/app/connector")
	disconnect.Tests = append(disconnect.Tests, "TestPostgresServerRejectsUndeliveredCommandsWithoutOpeningClientTransition", "TestPostgresCredentialDependencyFailuresStayRetryable")
	disconnect.Tests = append(disconnect.Tests, "TestPostgresCollectorCommandExpirationProjectsFailedAndUnknownOutcomes")
	disconnect.Tests = append(disconnect.Tests, "TestPostgresCollectorReconnectUsesVerifiedReceiptAndRejectsStaleOrForgedResults", "TestPostgresCollectorReconnectCannotOverwriteNewIntent")
	directExpiration := goCase("direct-expiration", "actual_owned_postgres_database_expiration_unknown_fences_late_recovery_and_cancelled_completion", "./internal/directexecutor", []string{"TestPostgresDirectCollectorExpirationProjectsFailedAndUnknownOutcomes"}, nil, pg)
	registry := goCase("connector-registry", "actual_redis_lua_rejects_stale_set_and_disconnect_with_concurrent_epochs", "./internal/storage/redis", []string{"TestRedisConnectorRegistryRejectsStaleSetAndDisconnect"}, nil, "ARGUS_PLAN8_REDIS_TEST_URL")
	routeRecovery := goCase("route-recovery", "actual_owned_postgres_current_config_tunnel_epoch_fence_lease_credentials_and_idempotent_route_recovery", "./internal/collectorstate", []string{"TestPostgresRouteRecoveryRequiresCurrentVerifiedConfigAndTunnelLease"}, nil, pg)
	routeRecovery.Tests = append(routeRecovery.Tests, "TestPostgresRepeatedCollectorOutcomeDoesNotInvalidateUnchangedIntent")
	k8sState := goCase("k8s-state", "owned_real_pvc_unprivileged_pod_replacement_and_volume_cleanup_with_ownership_guards", "./internal/collectormanager", []string{"TestKubernetesGatewayStateClaimOwnershipAndPodReplacement", "TestKubernetesCandidateValidationDoesNotMountLiveStateClaim", "TestKubernetesGatewayStateForeignOwnerStopsUninstallBeforeWrites", "TestLiveKubernetesGatewayStatePVCReplacesPodAndCleansOwnedVolume"}, map[string]string{"ARGUS_PLAN8_K8S_STATE_LIVE": "1", "ARGUS_PLAN8_K8S_STATE_EVIDENCE": filepath.Join(dir, "k8s-state-pvc.json")}, "ARGUS_PLAN8_K8S_STATE_KUBECONFIG")
	hostLogRules := goCase("host-file-rules", "host_file_rule_kubernetes_directory_ownership", "./internal/logcollection", []string{"TestHostFileLogKeepsKubernetesContainerPathsInDedicatedEntry"}, nil)
	hostLogConfig := goCase("host-file-config", "native_resolved_path_guard_and_automatic_exclusion_contract", "./internal/otelcol/configbundle", []string{"TestHostFileLogGuardExcludesKubernetesPathsBeforeUntrustedParsing"}, nil)
	services.Tests = append(services.Tests, "TestNativeHostFilelogRejectsKubernetesSymlinkBeforeParsing")
	services.Env["ARGUS_MONITORING_NATIVE_LOGS"] = "1"
	actualRollback := goCase("actual-runtime-rollback", "actual_native_systemd_activation_failure_rollback_and_revision_fence_with_controlled_transport", "./internal/collectormanager", []string{"TestOwnedActualCollectorActivationRollbackAndRevisionFence"}, map[string]string{"ARGUS_PLAN8_ACTUAL_ROLLBACK": "1", "ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_PLAN8_ACTUAL_ROLLBACK_EVIDENCE": filepath.Join(dir, "actual-runtime-rollback.json")}, binary, image)
	coexistEvidence := filepath.Join(dir, "actual-node-coexist.json")
	nodeCoexist := goCase("actual-node-coexist", "actual_same_node_host_and_daemonset_metrics_and_log_ownership_controlled_capture", "./internal/collectormanager", []string{"TestOwnedNodeHostAndDaemonSetHaveDistinctSourcesAndNoContainerLogDoubleRead"}, map[string]string{"ARGUS_PLAN8_NODE_COEXIST": "1", "ARGUS_MONITORING_NATIVE_TARGETS": "1", "ARGUS_PLAN8_NODE_COEXIST_EVIDENCE": coexistEvidence}, binary, "ARGUS_PLAN8_COEXIST_NODE", "ARGUS_PLAN8_COEXIST_CONTEXT", "KUBECONFIG")
	nodeBinding := goCase("actual-node-binding", "actual_live_node_facts_postgres_domain_preview_confirmation_and_controlled_identity_fault", "./internal/monitoring", []string{"TestPostgresActualNodeBindingConfirmationAndControlledIdentityDrift"}, map[string]string{"ARGUS_PLAN8_NODE_BINDING": "1", "ARGUS_PLAN8_NODE_COEXIST_EVIDENCE": coexistEvidence}, pg, "ARGUS_PLAN8_COEXIST_CONTEXT", "KUBECONFIG")
	connectorRotation := goCase("connector-rotation", "real_crypto_filesystem_rotation_then_existing_identity_repair", "./internal/app/connector", []string{"TestCertificateRotationPersistsIdentityUsableForSubsequentRepair"}, nil)
	switch suite {
	case "connector-rotation":
		return []plan8Check{connectorRotation}, nil
	case "engineering":
		return []plan8Check{engineering}, nil
	case "k8s-state":
		return []plan8Check{k8sState}, nil
	case "domain":
		return []plan8Check{domain, registryUpgrade, modelRecords, runtimeRoles, tools, workspace}, nil
	case "host":
		return []plan8Check{own, services, views}, nil
	case "sdk":
		return []plan8Check{sdk}, nil
	case "delivery":
		return []plan8Check{deliveryIdentity, deliveryStatus, sdk}, nil
	case "k8s":
		return []plan8Check{k8s, control, k8sViews}, nil
	case "drain":
		return []plan8Check{drainUnit, drainProtocol, drainLive}, nil
	case "control-positive":
		return []plan8Check{bootstrapRBAC, controlPositive, controlPositiveViews}, nil
	case "pod-logs":
		return []plan8Check{logUnit, logProtocol, logDomain, logHTTP, logLive}, nil
	case "streams":
		return []plan8Check{streamUnit, streamClient, streamHub, streamConnector, streamDomain, streamGateway, streamLive}, nil
	case "helm":
		return []plan8Check{helmUnit, helmCore, helmConnector, helmDomain, helmHTTP, helmLive, tools, helmSources, helmSourceLive, helmSourceDomain}, nil
	case "helm-sources":
		return []plan8Check{helmSources, helmCore, helmConnector, helmDomain, helmHTTP, helmSourceLive, helmSourceDomain}, nil
	case "resources":
		return []plan8Check{resources, resourceViews, metricScan}, nil
	case "cache-web":
		return []plan8Check{cache, cacheViews}, nil
	case "brokers":
		return []plan8Check{brokers, brokerViews}, nil
	case "network-probes":
		return []plan8Check{network, networkViews}, nil
	case "kafka":
		return []plan8Check{kafka, kafkaViews}, nil
	case "databases":
		return []plan8Check{databases, databaseViews}, nil
	case "namespace":
		return []plan8Check{namespaceKeys, namespaceGrants, namespaceLifecycle, namespaceQuery, namespaceLive}, nil
	case "completion-core":
		return []plan8Check{databases, databaseViews, namespaceKeys, namespaceGrants, namespaceLifecycle, namespaceQuery, namespaceLive, apmSources, tools}, nil
	case "assignment":
		return []plan8Check{claimUnit, assignments}, nil
	case "csi":
		return []plan8Check{csi}, nil
	case "backup":
		return []plan8Check{backup}, nil
	case "operations":
		return []plan8Check{nativeOperations, operationWorkflow}, nil
	case "runtime-behavior":
		return []plan8Check{policyTraffic, hpaBehavior}, nil
	case "context":
		return []plan8Check{admission, contextWire, actionBatch}, nil
	case "reconnect":
		return []plan8Check{disconnect, registry, directExpiration, routeRecovery}, nil
	case "components":
		return []plan8Check{domain, tools, workspace, own, services, sdk, views}, nil
	case "full":
		return []plan8Check{engineering, domain, registryUpgrade, modelRecords, metricScan, runtimeRoles, helmBootstrap, tools, workspace, hostLogRules, hostLogConfig, actualRollback, nodeCoexist, nodeBinding, own, services, sdk, views, k8s, control, k8sViews, resources, resourceViews, cache, cacheViews, brokers, brokerViews, network, networkViews, kafka, kafkaViews, drainUnit, drainProtocol, drainLive, bootstrapRBAC, controlPositive, controlPositiveViews, logUnit, logProtocol, logDomain, logHTTP, logLive, streamUnit, streamClient, streamHub, streamConnector, streamDomain, streamGateway, streamLive, helmUnit, helmCore, helmConnector, helmDomain, helmHTTP, helmLive, helmSources, helmSourceLive, helmSourceDomain, databases, databaseViews, namespaceKeys, namespaceGrants, namespaceLifecycle, namespaceQuery, namespaceLive, apmSources, claimUnit, assignments, csi, backup, nativeOperations, operationWorkflow, policyTraffic, hpaBehavior, admission, contextWire, actionBatch, disconnect, registry, directExpiration, deliveryIdentity, deliveryStatus, routeRecovery, k8sState, connectorRotation}, nil
	default:
		return nil, fmt.Errorf("%w: unknown Plan8 suite %q", errUsage, suite)
	}
}
