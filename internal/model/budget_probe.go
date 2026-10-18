package model

import (
	"context"
	"errors"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
)

func detectBudgetParameter(ctx context.Context, provider modelprovider.Provider, input Input) (string, error) {
	if provider.Protocol == modelprovider.ProtocolResponses {
		return modelprovider.BudgetMaxOutputTokens, nil
	}
	parameters := []string{modelprovider.BudgetMaxTokens, modelprovider.BudgetMaxCompletionTokens}
	if input.OutputTokenParameter != "" {
		parameters = []string{input.OutputTokenParameter}
	}
	for _, parameter := range parameters {
		err := probeBudgetParameter(ctx, provider, input, parameter)
		if err == nil {
			return parameter, nil
		}
		var status *modelprovider.HTTPStatusError
		if errors.Is(err, modelprovider.ErrBudgetNotEnforced) || errors.As(err, &status) && (status.StatusCode == 400 || status.StatusCode == 422) {
			continue
		}
		return "", err
	}
	return "", modelprovider.ErrBudgetNotEnforced
}

func probeBudgetParameter(ctx context.Context, provider modelprovider.Provider, input Input, parameter string) error {
	limit := min(int(input.MaxOutput), 128)
	request := modelprovider.Request{Model: input.ProviderModelID, MaxTokens: limit, OutputTokenParameter: parameter, ReasoningEffort: input.ReasoningEffort, Messages: []modelprovider.Message{{Role: "user", Content: modelprovider.OutputBudgetProbeMarker + ". Count from 1 to 400 separated by commas. Do not skip numbers, abbreviate or explain."}}}
	var usage modelprovider.TokenUsage
	reason := ""
	err := provider.Stream(ctx, request, func(event modelprovider.Event) error {
		usage.Observe(event)
		if event.Type == "completed" {
			reason = event.StopReason
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !usage.Complete() || usage.Output <= 0 || usage.Output > int64(limit) || reason != "length" {
		return modelprovider.ErrBudgetNotEnforced
	}
	return nil
}
