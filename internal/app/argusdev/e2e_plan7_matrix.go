package argusdev

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type plan7Coverage struct {
	ID     string
	Tests  []string
	Phases []string
	Files  []string
}

// Each row requires named executed checks and/or an actual resource/model/UI
// phase. A missing or skipped test cannot be converted into a passed row.
func plan7CoverageRequirements() []plan7Coverage {
	return []plan7Coverage{
		{"P7-A01", []string{"TestPathEscapeAndAmbiguousEditRejected", "TestAtomicWriteKeepsOldFileOnFailure", "TestWorkspaceLifecycleAndFileReferencesReachActualModelRequest", "TestAdmissionHardensFileRoleAndRejectsUntrustedSources"}, []string{"linux-confirmations-and-files"}, nil},
		{"P7-A02", []string{"TestInterpreterDoesNotInheritConnectorSecrets"}, []string{"workspace-offline-boundary"}, nil},
		{"P7-A03", []string{"TestCategoryListingAcceptsOnlyCategoryAndAppliesCurrentScope", "TestNativeInvocationValidatesSchemaAndTrustedIdentity", "TestCommitToolsAreHiddenAndCallerBound"}, nil, nil},
		{"P7-A04", []string{"TestLifecycleGatewayPreparesPublicTemplateAndKeepsRunForExecutor", "TestAllFormalPreviewFamiliesHaveStrictTemplatesAndHiddenCommits"}, []string{"linux-onboarding-and-collector"}, nil},
		{"P7-A05", []string{"TestRemotePreviewPersistsArrayDiff"}, []string{"linux-confirmations-and-files"}, nil},
		{"P7-A06", []string{"TestExpiredPreviewCannotCreateExecution", "TestStopBeforeDispatchAndUnknownStop", "TestCurrentAuthorityAndContextAreRevalidatedAtConfirmationAndDispatch"}, []string{"real-browser"}, nil},
		{"P7-A07", []string{"TestConnectorRemoteControlLeaseUsesRegisteredAuditActor", "TestConfirmationSerialExecutionAndNextBatchApproval"}, []string{"linux-confirmations-and-files", "windows-enrollment-and-powershell", "kubernetes-client-and-exec"}, nil},
		{"P7-A08", []string{"TestPreviewIdempotencyChecksOwnerAndContent", "TestSharedStateNativeErrorsAndUntrustedOutput", "TestOutputRetryNeverRepeatsKnownCommand"}, []string{"durable-worker-fault"}, nil},
		{"P7-A09", []string{"TestConfirmationSerialExecutionAndNextBatchApproval", "TestCrossConversationAndTenantIsolation"}, []string{"real-model-two-stage-evidence"}, nil},
		{"P7-A10", []string{"TestConfirmationSerialExecutionAndNextBatchApproval", "TestDispatchRejectsWrongConnectorAndOutOfOrderPlan"}, []string{"linux-confirmations-and-files", "kubernetes-client-and-exec"}, nil},
		{"P7-A11", []string{"TestFeedbackInvalidatesOldApprovalAndCreatesNewRevision", "TestCancellingOneRecoveredPreviewPreservesOtherActions", "TestRevisingRecoveryPlanIncreasesRevisionAndPreservesOrigin"}, []string{"real-model-diagnosis-and-repair"}, nil},
		{"P7-A12", []string{"TestCurrentAuthorityAndContextAreRevalidatedAtConfirmationAndDispatch", "TestFrozenInputTransfersAreBoundedDurableAndNotReapplied", "TestReadProjectsChangedContextAsUnconfirmableWithoutRewritingHistory"}, nil, nil},
		{"P7-A13", []string{"TestFailureAndUnknownNeverDispatchRemainingSteps"}, []string{"real-browser", "real-model-lost-context-recovery"}, nil},
		{"P7-A14", []string{"TestLateReceiptPreservesNewGenerationAndDoesNotResumeOldPlan", "TestDirectQueuePreservesGenerationAffinityAndNeverReplaysLostWork", "TestModelFactsDistinguishUnsentAndUnknownWithoutInventingExitCode", "TestDispatchedMalformedArgumentsRemainUnknownWithoutReplay"}, []string{"real-model-lost-context-recovery"}, nil},
		{"P7-A15", []string{"TestFreshWorkerResumesPersistedCollectionWithoutRepeatingEffect", "TestDirectQueuePreservesGenerationAffinityAndNeverReplaysLostWork"}, []string{"durable-worker-fault"}, []string{"plan7-worker-fault.json"}},
		{"P7-A16", []string{"TestCurrentAuthorityAndContextAreRevalidatedAtConfirmationAndDispatch"}, nil, nil},
		{"P7-A17", []string{"TestCrossConversationAndTenantIsolation", "TestNativeActionReadCannotCrossConversationOrRetainRevokedAccess", "TestRemoteInputSourceCyclesRemainBoundedAndRevocationIsRechecked", "TestConfirmationConflictIsVisibleOnlyToItsCreator"}, nil, nil},
		{"P7-A18", []string{"TestReadPermissionPreservesOwnFactsWithoutGrantingExecution"}, nil, nil},
		{"P7-A19", []string{"TestManualHostHasNoImplicitExecutionAccount"}, []string{"linux-onboarding-and-collector", "windows-enrollment-and-powershell"}, []string{"plan7-windows-identity.json"}},
		{"P7-A20", []string{"TestKubernetesProxyPinsServerAndKeepsCredentialsInConnector", "TestPinnedConfigCannotForwardCredentialsOrChangeConnection"}, []string{"kubernetes-client-and-exec", "kubernetes-direct-executor"}, []string{"plan7-kubernetes-adapter-tests.log"}},
		{"P7-A21", []string{"TestInterpreterDoesNotInheritConnectorSecrets", "TestConnectorRemoteControlLeaseUsesRegisteredAuditActor", "TestAssembleContextIsStableAndRedactsPrivateFields"}, []string{"workspace-offline-boundary"}, nil},
		{"P7-A22", []string{"TestTimeoutTerminatesInterpreterAndRejectsReuse", "TestInteractiveInputCannotConsumeNextApprovedCommand", "TestBackgroundChildCannotOutliveCompletedBatch", "TestOutputLimitAndFunctionsSurvive"}, nil, []string{"plan7-linux-runner-tests.log"}},
		{"P7-A23", []string{"TestStopBeforeDispatchAndUnknownStop", "TestUnknownExecutionCanBeStoppedWithoutReplayingCommand", "TestModeSwitchKeepsRunSnapshotAndApprovedBatch", "TestRemoteControlQueueCannotDispatchAnExecution"}, []string{"real-browser"}, nil},
		{"P7-A24", []string{"TestMCPCredentialBoundaryProtectsCatalogArtifactAndModel"}, nil, nil},
		{"P7-A25", []string{"TestSharedStateNativeErrorsAndUntrustedOutput"}, []string{"real-model-two-stage-evidence"}, []string{"plan7-model-evidence-sequence.json"}},
		{"P7-A26", []string{"TestModelCannotSupplyApprovalOrFrozenContextFields", "TestPlanRegistryPairRemainsHostOnly", "TestInputFeedbackNamesMissingFieldsWithoutEchoingValues", "TestModelReceivesSafeValidationPathsAndNotInternalErrors", "TestMalformedArgumentsArePersistedRejectedAndRecoverable", "TestMissingToolIdentityRemainsProtocolFailure"}, nil, nil},
		{"P7-A27", []string{"TestPowerShellNonTerminatingErrorRemainsFailure", "TestSharedStateNativeErrorsAndUntrustedOutput", "TestStopInvalidatesContext"}, []string{"windows-enrollment-and-powershell", "real-model-platforms"}, nil},
		{"P7-A28", []string{"TestDispatchRejectsWrongConnectorAndOutOfOrderPlan", "TestFailureAndUnknownNeverDispatchRemainingSteps", "TestStopBeforeDispatchAndUnknownStop"}, []string{"real-browser"}, nil},
		{"P7-A29", []string{"TestSharedStateNativeErrorsAndUntrustedOutput", "TestOutputLimitAndFunctionsSurvive", "TestPreviewFreezesKnownLocationVersionAndPreviousBatch"}, []string{"linux-confirmations-and-files", "windows-enrollment-and-powershell", "real-model-platforms"}, nil},
		{"P7-A30", []string{"TestDifferentChatsExecuteConcurrentlyWithPrivateActivity", "TestChangedContextAndRevocationInvalidateApproval", "TestReadProjectsChangedContextAsUnconfirmableWithoutRewritingHistory"}, nil, nil},
		{"P7-A31", []string{"TestPrivateStreamMultiplexesStopWhileBatchIsRunning", "TestFileTransferBackpressureDoesNotPreventStop", "TestSharedStateNativeErrorsAndUntrustedOutput"}, nil, []string{"plan7-linux-runtimewire-tests.log"}},
		{"P7-A32", []string{"TestFrozenInputTransfersAreBoundedDurableAndNotReapplied", "TestQueryFileImportWithRealRPCAndDurableSources", "TestFileClassificationSupportsUploadsAndRemoteOutputs", "TestWarmWorkspaceRequiresCurrentFileHelperImage"}, []string{"linux-confirmations-and-files", "windows-enrollment-and-powershell", "kubernetes-client-and-exec", "real-model-platforms"}, []string{"storage-live/plan7-storage-nine-volumes.json", "plan7-workspace-storage-tests.log", "plan7-production-workspaces.json"}},
		{"P7-A33", []string{"TestFileIntegrityAndTraversal", "TestAutomaticTransferRejectsLinks", "TestExhaustedOutputTransferPreservesSuccessfulEffect", "TestOutputRetryNeverRepeatsKnownCommand"}, nil, []string{"plan7-kubernetes-adapter-tests.log", "plan7-linux-runner-tests.log"}},
		{"P7-A34", []string{"TestDirectoryRecoveryCannotReviveLostInterpreter", "TestLateReceiptPreservesNewGenerationAndDoesNotResumeOldPlan"}, []string{"real-model-lost-context-recovery"}, nil},
		{"P7-A35", []string{"TestIdleReclamationProtectsExecutionAndUniqueOutputs", "TestIdleCandidateCannotReclaimAfterNewActivity", "TestReclaimRequiresOwnershipAndPreservesOtherDirectories", "TestFormalPausePersistsHealthAndStorageBeforeMutation", "TestFormalPauseSnapshotFailureLeavesReplicasUntouched", "TestFormalRestoreRejectsReplacedStorageAndRecordsFailure", "TestFormalRestorationDoesNotEquateReplicasWithHealth"}, nil, []string{"plan7-kubernetes-adapter-tests.log", "original-environment.json"}},
		{"P7-A36", []string{"TestDifferentChatsExecuteConcurrentlyWithPrivateActivity", "TestProcessorPoolOverlapsRemoteTasksAndKeepsControlAvailable", "TestRemoteQueuesUseIndependentCapacityWithoutParallelizingAgentTurns", "TestRemoteWorkerCapacityIsBoundedAndKeepsControlSlots"}, []string{"real-model-concurrent-context"}, nil},
		{"P7-A37", []string{"TestUnconfirmedTerminationRemainsVisibleInPrivateActivity"}, []string{"real-model-concurrent-context"}, nil},
		{"P7-A38", []string{"TestDifferentChatsExecuteConcurrentlyWithPrivateActivity", "TestPrivateRemoteActivityEvidenceMatchesActualModelProjection", "TestReadPermissionPreservesOwnFactsWithoutGrantingExecution"}, []string{"real-model-concurrent-context"}, nil},
		{"P7-M01", []string{"TestStatusPollingRecoversTransientReadAndPreservesAttempts", "TestStatusPollingDoesNotRetryMutationsDenialsOrMalformedSuccess"}, []string{"real-model-diagnosis-and-repair"}, []string{"plan7-telemetry-evidence.json"}},
		{"P7-M02", nil, []string{"real-model-diagnosis-and-repair"}, []string{"plan7-model-diagnosis.json"}},
		{"P7-M03", nil, []string{"real-model-two-stage-evidence"}, []string{"plan7-model-evidence-sequence.json"}},
		{"P7-M04", []string{"TestRunModeGuidanceUsesSnapshotInActualModelRequest", "TestStreamingResponseUsesSeparateBoundedDeadline", "TestStreamingDeadlineAndCancellationStillAbortRead", "TestEndpointPolicyRetainsBoundedConnectionHeadersAndOrdinaryRequests", "TestStreamingPrefillAndBodyShareTheBoundedDeadline", "TestTerminalModelEventDoesNotWaitForTransportEOF", "TestStreamFailureDiagnosticsUseBoundedClasses", "TestMalformedToolArgumentsRetainUsageAndCorrectionHistory"}, []string{"real-model-diagnosis-and-repair"}, []string{"plan7-model-repair-plan-original-preview.json", "plan7-model-repair-plan-revised-preview.json", "plan7-model-repair.json"}},
		{"P7-M05", nil, []string{"real-model-diagnosis-and-repair"}, []string{"plan7-model-repair.json", "plan7-service-recovery.json"}},
		{"P7-M06", []string{"TestHardCompactionMeasuresRemoteFactsAndDoesNotResumeUnchangedOversizeInput", "TestModelAccountingWaitsForBackgroundCompactionAndSettlement", "TestP5UsageGateRequiresReportedSettledSuccessfulCalls", "TestActiveTurnCompactionKeepsCompleteToolGroups", "TestCompactedActiveRunRetainsOriginalUserInstructionsAndFileState", "TestHardActiveTurnCompactionResumesWithPairedHistoryAndUserConstraints"}, []string{"real-model-lost-context-recovery", "durable-worker-fault"}, []string{"plan7-model-recovery.json", "plan7-active-compaction.log"}},
		{"P7-M07", nil, []string{"real-model-diagnosis-and-repair", "real-model-two-stage-evidence"}, []string{"plan7-telemetry-evidence.json", "plan7-model-evidence-sequence.json"}},
		{"P7-M08", []string{"TestMatrixOracleAcceptsEmptyOnlyWithNoDataAndNoInventedNumbers", "TestConclusionRejectsNonScalarLastValue", "TestMatrixOracleTiedLatestUsesLastPointInOriginalFileOrder", "TestBuiltinAvailabilityUsesActualCatalogAndExplainsSnapshotStatus", "TestMatrixRecordCountRemainsSeriesCount"}, []string{"real-model-evidence-quality"}, []string{"plan7-model-evidence-quality.json", "plan7-no-data-evidence.json"}},
		{"P7-M09", nil, []string{"real-model-platforms", "real-model-two-stage-evidence"}, []string{"plan7-windows-state-2-result.json", "plan7-kubernetes-model-3-result.json"}},
		{"P7-M10", []string{"TestModelFactsDistinguishUnsentAndUnknownWithoutInventingExitCode", "TestLostInterpreterReturnsOwnedRecoveryReferenceWithoutDispatch"}, []string{"real-model-lost-context-recovery"}, []string{"plan7-model-recovery.json"}},
		{"P7-M11", []string{"TestPrivateRemoteActivityEvidenceMatchesActualModelProjection"}, []string{"real-model-concurrent-context"}, []string{"plan7-model-activity.json"}},
		{"P7-U01", nil, []string{"real-browser", "real-model-diagnosis-and-repair"}, []string{"playwright-plan7/report.json"}},
		{"P7-U02", nil, []string{"real-browser"}, []string{"playwright-plan7/report.json"}},
		{"P7-U03", []string{"TestReadProjectsChangedContextAsUnconfirmableWithoutRewritingHistory", "TestExhaustedOutputTransferPreservesSuccessfulEffect"}, []string{"real-browser", "real-model-concurrent-context"}, []string{"playwright-plan7/report.json", "playwright-plan7-model/report.json"}},
		{"P7-U04", []string{"TestConversationSSESlowConsumersStayBoundedAndCancel"}, []string{"real-browser"}, []string{"playwright-plan7/report.json"}},
		{"P7-U05", []string{"TestPlan7ProductionWebRejectsFixtureConfiguration"}, []string{"engineering"}, []string{"plan7-web-test.log", "plan7-real-build.log", "web-build-config.json"}},
		{"P7-U06", nil, []string{"real-browser"}, []string{"playwright-plan7/report.json"}},
		{"P7-U07", nil, []string{"real-browser", "real-model-concurrent-context"}, []string{"playwright-plan7/report.json", "playwright-plan7-model/report.json"}},
		{"P7-U08", []string{"TestPrivateRemoteActivityEvidenceMatchesActualModelProjection"}, []string{"real-model-concurrent-context"}, []string{"playwright-plan7-model/report.json"}},
	}
}

func plan7WritePhase(env *E2EEnvironment, name string) error {
	raw, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "source_fingerprint": env.State.Values["p6_source_fingerprint"], "phase": name, "status": "passed", "completed_at": time.Now().UTC()}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-phase-"+name+".json"), raw)
}

func plan7PassedTests(path string) (map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	passed := map[string]bool{}
	scan := bufio.NewScanner(file)
	scan.Buffer(make([]byte, 65536), 16<<20)
	for scan.Scan() {
		var event struct{ Action, Test string }
		if json.Unmarshal(scan.Bytes(), &event) != nil {
			continue
		}
		if event.Action == "fail" {
			return nil, fmt.Errorf("failed Go test %s", event.Test)
		}
		if event.Action == "pass" && event.Test != "" {
			passed[event.Test] = true
		}
	}
	return passed, scan.Err()
}

func buildPlan7Matrix(ctx context.Context, env *E2EEnvironment) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validatePlan7ProductionWeb(env); err != nil {
		return err
	}
	passed, err := plan7PassedTests(filepath.Join(env.Options.Artifacts, "plan7-go-test.log"))
	if err != nil {
		return err
	}
	active, err := plan7PassedTests(filepath.Join(env.Options.Artifacts, "plan7-active-compaction.log"))
	if err != nil {
		return err
	}
	if !active["TestHardActiveTurnCompactionResumesWithPairedHistoryAndUserConstraints"] {
		return fmt.Errorf("missing actual active-turn compaction lifecycle check")
	}
	for name := range active {
		passed[name] = true
	}
	storage, err := plan7PassedTests(filepath.Join(env.Options.Artifacts, "plan7-workspace-storage-tests.log"))
	if err != nil {
		return err
	}
	if !storage["TestPlan7LiveWorkspaceStorageBeyondEightVolumes"] {
		return fmt.Errorf("missing actual multi-volume Workspace storage check")
	}
	for _, report := range []struct {
		path  string
		count int
	}{{"playwright-plan7/report.json", 18}, {"playwright-plan7-model/report.json", 1}} {
		raw, e := os.ReadFile(filepath.Join(env.Options.Artifacts, report.path))
		if e != nil {
			return e
		}
		var body struct {
			Stats  struct{ Expected, Unexpected, Flaky, Skipped int }
			Errors []json.RawMessage
		}
		if json.Unmarshal(raw, &body) != nil || body.Stats.Expected != report.count || body.Stats.Unexpected != 0 || body.Stats.Flaky != 0 || body.Stats.Skipped != 0 || len(body.Errors) != 0 {
			return fmt.Errorf("incomplete browser evidence %s", report.path)
		}
	}
	live, err := plan7PassedTests(filepath.Join(env.Options.Artifacts, "plan7-kubernetes-adapter-tests.log"))
	if err != nil {
		return err
	}
	for _, name := range []string{"TestLivePodRuntimeStateFilesAndReplacement", "TestLiveKubectlRuntimeIdentityAndNetworkIsolation", "TestLiveIsolationRequiresPositiveControlAndRejectsConflictingAllow"} {
		if !live[name] {
			return fmt.Errorf("missing live Kubernetes check %s", name)
		}
	}
	for _, pkg := range []string{"runner", "runtimewire"} {
		raw, e := os.ReadFile(filepath.Join(env.Options.Artifacts, "plan7-linux-"+pkg+"-tests.log"))
		if e != nil {
			return e
		}
		if !strings.Contains(string(raw), "PASS\n") || strings.Contains(string(raw), "--- FAIL:") {
			return fmt.Errorf("incomplete Linux test evidence %s", pkg)
		}
	}
	type proof struct {
		Path string `json:"path"`
		Hash string `json:"hash"`
	}
	type entry struct {
		ID       string   `json:"id"`
		Status   string   `json:"status"`
		Tests    []string `json:"tests,omitempty"`
		Evidence []proof  `json:"evidence"`
	}
	entries := []entry{}
	for _, requirement := range plan7CoverageRequirements() {
		e := entry{ID: requirement.ID, Status: "passed", Tests: requirement.Tests}
		paths := append([]string{"plan7-engineering.json"}, requirement.Files...)
		if len(requirement.Tests) > 0 {
			paths = append(paths, "plan7-go-test.log")
		}
		for _, name := range requirement.Tests {
			if !passed[name] {
				return fmt.Errorf("%s missing executed test %s (skip is not pass)", requirement.ID, name)
			}
		}
		for _, phase := range requirement.Phases {
			path := "plan7-phase-" + phase + ".json"
			raw, e := os.ReadFile(filepath.Join(env.Options.Artifacts, path))
			if e != nil {
				return e
			}
			var marker map[string]string
			if json.Unmarshal(raw, &marker) != nil || marker["run_id"] != env.Options.RunID || marker["source_fingerprint"] != env.State.Values["p6_source_fingerprint"] || marker["phase"] != phase || marker["status"] != "passed" {
				return fmt.Errorf("invalid phase provenance %s", phase)
			}
			paths = append(paths, path)
		}
		for _, path := range paths {
			raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, path))
			if err != nil {
				return err
			}
			e.Evidence = append(e.Evidence, proof{path, fmt.Sprintf("%x", sha256.Sum256(raw))})
		}
		entries = append(entries, e)
	}
	raw, _ := json.MarshalIndent(map[string]any{"run_id": env.Options.RunID, "source_fingerprint": env.State.Values["p6_source_fingerprint"], "cases": entries}, "", "  ")
	return writePrivate(filepath.Join(env.Options.Artifacts, "plan7-matrix.json"), raw)
}

func validatePlan7ProductionWeb(env *E2EEnvironment) error {
	raw, err := os.ReadFile(filepath.Join(env.Options.Artifacts, "web-build-config.json"))
	if err != nil {
		return err
	}
	var receipt struct {
		RunID   string  `json:"run_id"`
		Source  string  `json:"source_fingerprint"`
		Profile string  `json:"profile"`
		Mode    string  `json:"api_mode"`
		BaseURL string  `json:"api_base_url"`
		Egress  *string `json:"direct_egress_addresses"`
		Digest  string  `json:"image_digest"`
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.RunID != env.Options.RunID || receipt.Source != env.State.Values["p6_source_fingerprint"] || receipt.Profile != "production" || receipt.Mode != "real" || receipt.BaseURL != "/" || receipt.Egress == nil || *receipt.Egress != "" || !validPlan6Digest(receipt.Digest) {
		return fmt.Errorf("Plan7 Web build is not the frozen production configuration")
	}
	raw, err = os.ReadFile(filepath.Join(env.Options.Artifacts, "image-provenance.json"))
	if err != nil {
		return err
	}
	var provenance struct {
		Images []plan6Image `json:"core_images"`
	}
	if json.Unmarshal(raw, &provenance) != nil {
		return fmt.Errorf("invalid Web image provenance")
	}
	for _, image := range provenance.Images {
		if image.Name == "web" && image.ImageID == receipt.Digest {
			return nil
		}
	}
	return fmt.Errorf("accepted Web image differs from production build receipt")
}
