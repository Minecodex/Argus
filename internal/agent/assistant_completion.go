package agent

import (
	"context"

	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// A completed HTTP response is not a completed task when the provider truncated
// generation. Keep its partial text and reported usage, but do not claim success
// or automatically continue remote work after an exhausted output budget.
func (loop Loop) finishAssistantTurn(ctx context.Context, run db.Run, step db.RunStep, text, scope string, usage modelprovider.TokenUsage, stopReason string) error {
	status := "succeeded"
	if stopReason != "stop" && stopReason != "completed" {
		status = "failed"
	}
	if err := loop.persistAssistantStatus(ctx, run, step, text, scope, usage, status); err != nil {
		return err
	}
	if status == "failed" {
		return loop.finishRun(ctx, run, status, "model_output_incomplete", "MODEL_OUTPUT_INCOMPLETE")
	}
	return loop.finishRun(ctx, run, status, stopReason, "")
}
