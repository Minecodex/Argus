package connector

import (
	"encoding/json"
	"strings"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/installation"
	"github.com/kakj-go/Argus/internal/resource"
	"google.golang.org/protobuf/types/known/anypb"
)

func hostProbeOutcome(raw []byte, status string, typed *anypb.Any, errorCode string) (resource.ConnectionTestResult, string, string, error) {
	var plan struct {
		Onboarding *installation.CallbackProbePlan `json:"onboarding"`
	}
	if json.Unmarshal(raw, &plan) != nil {
		return resource.ConnectionTestResult{}, "", "", ErrCommandState
	}
	result := resource.ConnectionTestResult{}
	if typed != nil {
		var value connectorv1.HostConnectionProbeResult
		if typed.UnmarshalTo(&value) != nil {
			return result, "", "", ErrCommandState
		}
		// Authentication and transport failures may carry an empty typed result.
		// Only project target checks after the runtime actually observed a target.
		if status == "succeeded" || value.GetHostKeyFingerprint() != "" {
			result = hostProbeConnectionTestResult(&value)
		}
	} else if status == "succeeded" {
		return result, "", "", ErrCommandState
	}
	if plan.Onboarding != nil && status == "succeeded" && (!result.CallbackVerified || result.CallbackControlPath != plan.Onboarding.ControlPath) {
		status, errorCode = "failed", "HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID"
	}
	if strings.HasPrefix(errorCode, "HOST_ONBOARDING_CALLBACK_") {
		switch errorCode {
		case "HOST_ONBOARDING_CALLBACK_CONFIG_INVALID", "HOST_ONBOARDING_CALLBACK_DNS_FAILED", "HOST_ONBOARDING_CALLBACK_CONNECT_FAILED", "HOST_ONBOARDING_CALLBACK_TLS_FAILED", "HOST_ONBOARDING_CALLBACK_HTTP_FAILED", "HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID", "HOST_ONBOARDING_CALLBACK_TIMEOUT":
		default:
			errorCode = "HOST_ONBOARDING_CALLBACK_RESPONSE_INVALID"
		}
		if status == "failed" && plan.Onboarding != nil {
			result.CallbackVerified, result.CallbackControlPath = false, ""
			checks := result.Checks[:0]
			for _, check := range result.Checks {
				if check["name"] != "onboarding_callback" {
					checks = append(checks, check)
				}
			}
			result.Checks = append(checks, map[string]string{"name": "onboarding_callback", "status": "failed", "detail": errorCode})
		}
	}
	return result, status, errorCode, nil
}
