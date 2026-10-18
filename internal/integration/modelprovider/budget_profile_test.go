package modelprovider

import "testing"

func TestRevisionBudgetProfileIsProtocolBoundAndKeepsLegacySnapshots(t *testing.T) {
	for _, tc := range []struct {
		protocol       Protocol
		data, expected string
		invalid        bool
	}{
		{ProtocolChatCompletions, `{}`, BudgetMaxCompletionTokens, false},
		{ProtocolChatCompletions, `{"output_token_parameter":"max_tokens"}`, BudgetMaxTokens, false},
		{ProtocolChatCompletions, `{"output_token_parameter":"max_completion_tokens"}`, BudgetMaxCompletionTokens, false},
		{ProtocolResponses, `{}`, BudgetMaxOutputTokens, false},
		{ProtocolResponses, `{"output_token_parameter":"max_tokens"}`, "", true},
		{ProtocolChatCompletions, `{"output_token_parameter":"private-unsupported"}`, "", true},
		{ProtocolChatCompletions, `broken`, "", true},
	} {
		parameter, err := BudgetParameterForCapabilities(tc.protocol, []byte(tc.data))
		if (err != nil) != tc.invalid || parameter != tc.expected {
			t.Fatalf("profile=%s parameter=%s error=%v", tc.data, parameter, err)
		}
	}
}
