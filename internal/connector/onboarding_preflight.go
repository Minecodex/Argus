package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"reflect"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (service BastionService) PlanHostOnboardingProbe(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, controlPath, sshPath string, scopeID uuid.NullUUID) (*installation.CallbackProbePlan, error) {
	switch controlPath {
	case "direct", "executor_tunnel":
		if sshPath != "direct_executor" || scopeID.Valid {
			return nil, resource.ErrInvalidConnectionMode
		}
	case "bastion_relay":
		if sshPath != "bastion_connector" || !scopeID.Valid {
			return nil, resource.ErrInvalidConnectionMode
		}
	default:
		return nil, resource.ErrInvalidConnectionMode
	}
	enrollment, err := url.Parse(service.Enrollment.EnrollmentURL)
	if err != nil || enrollment.Scheme != "https" || enrollment.Hostname() == "" || enrollment.User != nil || enrollment.RawQuery != "" || enrollment.Fragment != "" || (enrollment.Path != "" && enrollment.Path != "/") {
		return nil, resource.ErrActionUnavailable
	}
	gateway, err := url.Parse(service.Enrollment.GatewayEndpoint)
	if err != nil || gateway.Scheme != "grpcs" || gateway.Hostname() == "" || gateway.Port() == "" || gateway.User != nil || gateway.RawQuery != "" || gateway.Fragment != "" || gateway.Path != "" {
		return nil, resource.ErrActionUnavailable
	}
	bundle, err := service.Enrollment.installationTrustBundle(ctx)
	if err != nil || bundle.Epoch < 1 || len(bundle.Material.PEM) == 0 {
		return nil, resource.ErrActionUnavailable
	}
	plan := &installation.CallbackProbePlan{ControlPath: controlPath, EnrollmentEndpoint: service.Enrollment.EnrollmentURL,
		GatewayEndpoint: service.Enrollment.GatewayEndpoint, TrustBundlePEM: bytes.Clone(bundle.Material.PEM), TrustBundleEpoch: bundle.Epoch}
	if controlPath == "bastion_relay" {
		scope, err := q.GetBastionScope(ctx, db.GetBastionScopeParams{ID: scopeID.UUID, EnterpriseID: enterpriseID})
		if err != nil || scope.Status != "active" || scope.RelayStatus != "ready" || scope.RelayAddress == "" || scope.RelayPortGeneration < 1 || scope.RelayHttpsPort < 1 || scope.RelayGatewayPort < 1 {
			return nil, ErrControlTunnelUnavailable
		}
		plan.EnrollDialAddress = net.JoinHostPort(scope.RelayAddress, fmt.Sprint(scope.RelayHttpsPort))
		plan.GatewayDialAddress = net.JoinHostPort(scope.RelayAddress, fmt.Sprint(scope.RelayGatewayPort))
		plan.RelayPortGeneration = scope.RelayPortGeneration
	}
	return plan, nil
}

func (service BastionService) validateCallbackConnectionTest(ctx context.Context, q *db.Queries, enterpriseID uuid.UUID, test db.ConnectionTest, controlPath, sshPath string, scopeID uuid.NullUUID) error {
	var frozen struct {
		Onboarding *installation.CallbackProbePlan `json:"onboarding"`
	}
	var result resource.ConnectionTestResult
	if json.Unmarshal(test.RequestPlan, &frozen) != nil || frozen.Onboarding == nil || json.Unmarshal(test.Result, &result) != nil || !result.CallbackVerified || result.CallbackControlPath != controlPath {
		return resource.ErrConnectionTestNeeded
	}
	current, err := service.PlanHostOnboardingProbe(ctx, q, enterpriseID, controlPath, sshPath, scopeID)
	if err != nil || !reflect.DeepEqual(current, frozen.Onboarding) {
		return resource.ErrConnectionTestNeeded
	}
	return nil
}

func bastionInstallControlPath(installMode string) string {
	if installMode == "direct_install_tunnel" {
		return "executor_tunnel"
	}
	return "direct"
}

var _ resource.OnboardingProbePlanner = BastionService{}
