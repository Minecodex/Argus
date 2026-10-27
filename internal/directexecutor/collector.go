package directexecutor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kakj-go/Argus/internal/collectormanager"
	"github.com/kakj-go/Argus/internal/collectorstate"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetrybinding"
)

const collectorOperationTimeout = 2 * time.Minute

func (executor *Executor) executeCollectorOperation(parent context.Context, operation db.TelemetryCollectorOperation) {
	ctx, cancel := context.WithTimeout(parent, collectorOperationTimeout)
	defer cancel()
	canonicalPlan, err := resource.CanonicalJSON(operation.Plan)
	if err != nil {
		executor.finishCollectorOperation(ctx, operation, "failed", nil, "COLLECTOR_PLAN_INVALID")
		return
	}
	hash := sha256.Sum256(canonicalPlan)
	if !bytes.Equal(hash[:], operation.PlanHash) {
		executor.finishCollectorOperation(ctx, operation, "failed", nil, "COLLECTOR_PLAN_INVALID")
		return
	}
	var command connectorv1.CollectorManagementCommand
	if protojson.Unmarshal(canonicalPlan, &command) != nil || collectormanager.Validate(&command) != nil ||
		command.GetCollectorId() != operation.CollectorID.String() || command.GetOperation() != operation.Operation || len(command.GetPrivateMonitoringConfig()) != 0 {
		executor.finishCollectorOperation(ctx, operation, "failed", nil, "COLLECTOR_PLAN_INVALID")
		return
	}
	if configbundle.RequiresMonitoringCredentials(command.GetRenderedConfig()) {
		private, err := executor.Secrets.MonitoringConfigForCollector(secret.WithActorType(ctx, "direct_executor"), executor.InstanceID, operation.EnterpriseID, "direct_executor", executor.InstanceID, operation.ID.String(), int64(command.GetDesiredRevision()), command.GetRenderedConfig())
		if err != nil {
			executor.finishCollectorOperation(ctx, operation, "failed", nil, "CREDENTIAL_UNAVAILABLE")
			return
		}
		defer clear(private)
		command.PrivateMonitoringConfig = private
	}
	if command.GetTransport() == "executor_tunnel" && command.GetOperation() != "uninstall" {
		ready, failureCode := executor.tunnelGateForCollector(ctx, operation.CollectorID)
		if !ready {
			if failureCode != "" {
				// 不可恢复的隧道前件失败(回环端口冲突/转发端点未配置):
				// 直接失败 operation,而不是无限等待重排。
				executor.finishCollectorOperation(ctx, operation, "failed", nil, failureCode)
				return
			}
			// 未得到可验证结果时由租约恢复标记 unknown，等待复核或修复；
			// 不把可能已执行的操作自动重放。
			slog.Info("collector operation waiting for tunnel", "operation_id", operation.ID, "collector_id", operation.CollectorID)
			return
		}
	}
	if command.GetOperation() == "install" {
		token, tokenErr := executor.CollectorIdentity.CreateEnrollmentToken(ctx, nil, operation.CollectorID)
		if tokenErr != nil {
			executor.finishCollectorOperation(ctx, operation, "failed", nil, "TELEMETRY_ENROLLMENT_UNAVAILABLE")
			return
		}
		command.EnrollmentToken = []byte(token)
		command.EnrollmentEndpoint = executor.TelemetryEnrollmentEndpoint
		command.IngestGrpcEndpoint = executor.TelemetryIngestGRPCEndpoint
		command.IngestHttpEndpoint = executor.TelemetryIngestHTTPEndpoint
	}
	addresses, err := executor.Validator.Resolve(ctx, command.GetTargetAddress())
	if err != nil || len(addresses) == 0 {
		executor.finishCollectorOperation(ctx, operation, "failed", nil, "DIRECT_TARGET_DENIED")
		return
	}
	credentialID, err := uuid.Parse(command.GetCredentialId())
	if err != nil || command.GetCredentialVersion() < 1 {
		executor.finishCollectorOperation(ctx, operation, "failed", nil, "CREDENTIAL_UNAVAILABLE")
		return
	}
	credential, err := executor.Store.Queries.GetCredential(ctx, db.GetCredentialParams{ID: credentialID, EnterpriseID: operation.EnterpriseID})
	if err != nil || credential.Status != "active" || credential.Version != int64(command.GetCredentialVersion()) {
		executor.finishCollectorOperation(ctx, operation, "failed", nil, "CREDENTIAL_VERSION_STALE")
		return
	}
	protocol := "ssh"
	if command.GetResourceType() == "kubernetes_cluster" {
		protocol = "kubernetes"
	}
	lease, err := executor.Secrets.IssueLease(secret.WithActorType(ctx, "direct_executor"), executor.InstanceID, operation.EnterpriseID, secret.LeaseRequest{
		CredentialID: credentialID, OperationRef: operation.ID.String(), TargetResourceType: command.GetResourceType(),
		TargetResourceID: uuid.MustParse(command.GetResourceId()), RecipientType: "direct_executor", RecipientID: executor.InstanceID,
		Protocol: protocol, TTL: 5 * time.Minute,
	})
	if err != nil {
		executor.finishCollectorOperation(ctx, operation, "failed", nil, "CREDENTIAL_UNAVAILABLE")
		return
	}
	defer clear(lease.Value)
	var result collectormanager.Result
	if command.GetResourceType() == "host" {
		result, err = executor.executeSSHCollector(ctx, &command, addresses, lease.Value)
	} else if command.GetResourceType() == "kubernetes_cluster" {
		result, err = executor.executeKubernetesCollector(ctx, operation.EnterpriseID, &command, lease.Value)
	} else {
		err = collectormanager.ErrInvalidCommand
	}
	if err != nil {
		code := collectormanager.FailureCode(err)
		if errors.Is(err, resource.ErrDirectTargetDenied) {
			code = "DIRECT_TARGET_DENIED"
		}
		slog.Error("collector operation failed",
			"operation_id", operation.ID, "collector_id", operation.CollectorID,
			"operation", operation.Operation, "resource_id", command.GetResourceId(),
			"error_code", code, "error", err)
		executor.finishCollectorOperation(ctx, operation, "failed", nil, code)
		return
	}
	_ = executor.Secrets.ConsumeLease(ctx, operation.EnterpriseID, lease.Lease.ID)
	resultHash := sha256.Sum256([]byte(strings.Join([]string{result.CollectorID, result.Status, result.ConfigSHA256, result.DiagnosticHash}, "\x00")))
	executor.finishCollectorOperation(ctx, operation, "succeeded", resultHash[:], "")
}

func (executor *Executor) executeKubernetesCollector(ctx context.Context, enterpriseID uuid.UUID, command *connectorv1.CollectorManagementCommand, kubeconfig []byte) (collectormanager.Result, error) {
	configuration, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return collectormanager.Result{}, err
	}
	configuration.Timeout = 30 * time.Second
	client, err := kubernetes.NewForConfig(configuration)
	if err != nil {
		return collectormanager.Result{}, err
	}
	if command.GetOperation() != "uninstall" {
		values, collectErr := telemetrybinding.Collect(ctx, client)
		if collectErr != nil {
			return collectormanager.Result{}, collectErr
		}
		if err = telemetrybinding.Upsert(ctx, executor.Store.Queries, enterpriseID, uuid.MustParse(command.GetResourceId()), values); err != nil {
			return collectormanager.Result{}, err
		}
	}
	return (collectormanager.Manager{HTTPClient: executor.CollectorArtifactHTTPClient}).ApplyKubernetes(ctx, command, collectormanager.KubernetesOptions{Client: client, WaitForEnrollment: true})
}

func (executor *Executor) executeSSHCollector(ctx context.Context, command *connectorv1.CollectorManagementCommand, addresses []netip.Addr, credential []byte) (collectormanager.Result, error) {
	manager := collectormanager.Manager{HTTPClient: executor.CollectorArtifactHTTPClient}
	return manager.ApplySSH(ctx, command, collectormanager.SSHOptions{Credential: credential,
		Dial: func(ctx context.Context) (net.Conn, error) {
			return dialFixed(ctx, addresses[0], int32(command.GetTargetPort()))
		},
		Revalidate: func(ctx context.Context) error {
			return executor.Validator.Revalidate(ctx, command.GetTargetAddress(), addresses)
		}})
}

func (executor *Executor) finishCollectorOperation(ctx context.Context, operation db.TelemetryCollectorOperation, status string, resultHash []byte, errorCode string) {
	finalize, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	ctx = finalize
	var command connectorv1.CollectorManagementCommand
	_ = protojson.Unmarshal(operation.Plan, &command)
	err := executor.Store.InTx(ctx, func(q *db.Queries) error {
		if _, err := q.FinishTelemetryCollectorOperation(ctx, db.FinishTelemetryCollectorOperationParams{ID: operation.ID,
			EnterpriseID: operation.EnterpriseID, Fence: operation.Fence, Status: status, ResultHash: resultHash,
			ErrorCode: pgtype.Text{String: errorCode, Valid: errorCode != ""}}); err != nil {
			return err
		}
		if command.GetDesiredRevision() == 0 {
			return nil
		}
		current, err := collectorstate.Apply(ctx, q, operation.EnterpriseID, operation.CollectorID, int64(command.GetDesiredRevision()), operation.Operation, status == "succeeded", errorCode)
		if err != nil || !current {
			return err
		}
		if status == "succeeded" {
			if operation.Operation != "uninstall" {
				if _, err = q.MarkTelemetryRouteActive(ctx, db.MarkTelemetryRouteActiveParams{CollectorID: operation.CollectorID, EnterpriseID: operation.EnterpriseID}); err != nil {
					return err
				}
				_, err = q.FinalizeCollectorClaimMigrations(ctx, db.FinalizeCollectorClaimMigrationsParams{EnterpriseID: operation.EnterpriseID, CollectorID: operation.CollectorID})
			}
			return err
		}
		if !collectorstate.OutcomeUnknown(errorCode) {
			if _, err = q.RollbackCollectorClaimMigrations(ctx, db.RollbackCollectorClaimMigrationsParams{EnterpriseID: operation.EnterpriseID, CollectorID: operation.CollectorID}); err != nil {
				return err
			}
		}
		err = collectorstate.RouteAfterFailure(ctx, q, operation.EnterpriseID, operation.CollectorID, operation.Operation, errorCode)
		return err
	})
	if err != nil {
		slog.Error("Collector operation outcome was not persisted", "operation_id", operation.ID, "error", err)
	}
}
