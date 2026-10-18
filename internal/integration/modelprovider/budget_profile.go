package modelprovider

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	OutputBudgetProbeMarker   = "argus.output_budget_probe/v1"
	BudgetMaxTokens           = "max_tokens"
	BudgetMaxCompletionTokens = "max_completion_tokens"
	BudgetMaxOutputTokens     = "max_output_tokens"
)

// HTTPStatusError preserves the safe status without retaining a response body,
// URL or credential. Compatibility may try another budget field on 400/422;
// network, authentication and rate-limit failures are never parameter fallback.
type HTTPStatusError struct{ StatusCode int }

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("model provider returned HTTP %d", e.StatusCode)
}

var ErrBudgetNotEnforced = errors.New("model output budget was not enforced")
var ErrBudgetProfileInvalid = errors.New("invalid model output budget profile")

func BudgetParameterForCapabilities(protocol Protocol, data []byte) (string, error) {
	var profile struct {
		Parameter string `json:"output_token_parameter"`
	}
	if len(data) > 0 && json.Unmarshal(data, &profile) != nil {
		return "", ErrBudgetProfileInvalid
	}
	if profile.Parameter == "" {
		if protocol == ProtocolChatCompletions {
			return BudgetMaxCompletionTokens, nil
		}
		if protocol == ProtocolResponses {
			return BudgetMaxOutputTokens, nil
		}
	}
	if protocol == ProtocolChatCompletions && (profile.Parameter == BudgetMaxTokens || profile.Parameter == BudgetMaxCompletionTokens) {
		return profile.Parameter, nil
	}
	if protocol == ProtocolResponses && profile.Parameter == BudgetMaxOutputTokens {
		return profile.Parameter, nil
	}
	return "", ErrBudgetProfileInvalid
}
