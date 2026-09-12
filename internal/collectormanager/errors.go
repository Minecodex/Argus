package collectormanager

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/kakj-go/Argus/internal/telemetrybinding"
)

type stageError struct {
	stage string
	cause error
}

func (err stageError) Error() string { return err.stage + ": " + err.cause.Error() }
func (err stageError) Unwrap() error { return err.cause }

func withStage(stage string, err error) error {
	if err == nil {
		return nil
	}
	var current stageError
	if errors.As(err, &current) {
		return err
	}
	return stageError{stage: stage, cause: err}
}

func FailureStage(err error) string {
	var staged stageError
	if errors.As(err, &staged) {
		return staged.stage
	}
	return "unknown"
}

// FailureCode 把 Collector 管理执行错误映射为稳定的错误码。
// Connector 与 Direct Executor 两条执行路径共用,保证同一失败原因
// 在界面与审计里呈现一致的可诊断信息,而不是笼统的 MANAGEMENT_FAILED。
func FailureCode(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "COLLECTOR_HEALTH_CHECK_FAILED"
	case errors.Is(err, ErrTargetAuthFailed):
		return "COLLECTOR_TARGET_AUTH_FAILED"
	case errors.Is(err, ErrTargetHostKeyChanged):
		return "COLLECTOR_TARGET_HOST_KEY_CHANGED"
	case errors.Is(err, ErrInvalidCommand):
		return "COLLECTOR_COMMAND_INVALID"
	case errors.Is(err, ErrUnsupportedPlatform):
		return "COLLECTOR_DISTRIBUTION_UNSUPPORTED"
	case errors.Is(err, ErrArtifactInvalid):
		return "COLLECTOR_ARTIFACT_INVALID"
	case errors.Is(err, telemetrybinding.ErrInvalidNodeEvidence):
		return "COLLECTOR_NODE_EVIDENCE_INVALID"
	case apierrors.IsForbidden(err):
		return "COLLECTOR_KUBERNETES_FORBIDDEN"
	case apierrors.IsInvalid(err):
		return "COLLECTOR_KUBERNETES_RESOURCE_INVALID"
	case apierrors.IsNotFound(err):
		return "COLLECTOR_KUBERNETES_RESOURCE_MISSING"
	case apierrors.IsConflict(err):
		return "COLLECTOR_KUBERNETES_CONFLICT"
	default:
		return "COLLECTOR_MANAGEMENT_FAILED"
	}
}
