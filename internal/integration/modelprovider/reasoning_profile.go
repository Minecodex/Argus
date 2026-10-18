package modelprovider

import "encoding/json"

// Reasoning strength is opt-in and frozen with the model revision. Unknown
// settings and unsupported protocols fail closed rather than changing silently.
func NormalizeReasoningEffort(protocol Protocol, effort string) (string, error) {
	if effort == "" || effort == "provider_default" {
		return "", nil
	}
	if protocol != ProtocolChatCompletions {
		return "", ErrBudgetProfileInvalid
	}
	switch effort {
	case "low", "high", "max":
		return effort, nil
	default:
		return "", ErrBudgetProfileInvalid
	}
}

func ReasoningEffortForCapabilities(protocol Protocol, capabilities []byte) (string, error) {
	if len(capabilities) == 0 {
		return "", nil
	}
	var value struct {
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if json.Unmarshal(capabilities, &value) != nil {
		return "", ErrBudgetProfileInvalid
	}
	return NormalizeReasoningEffort(protocol, value.ReasoningEffort)
}
