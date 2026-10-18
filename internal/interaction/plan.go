package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/conversation"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

func (s Service) ProposePlan(ctx context.Context, call toolruntime.Invocation, input Plan) (db.ConversationInteraction, error) {
	if err := s.authorize(ctx, call.Principal); err != nil {
		return db.ConversationInteraction{}, err
	}
	run, err := s.Store.Queries.GetRun(ctx, db.GetRunParams{ID: call.RunID, EnterpriseID: call.Principal.EnterpriseID})
	if err != nil || run.ConversationID != call.Principal.ConversationID || run.ActorUserID != call.Principal.UserID || run.AuthorizationVersion != call.Principal.AuthorizationVersion || conversation.TerminalRun(run.Status) {
		return db.ConversationInteraction{}, forbidden()
	}
	// Server-only fields never come from the model.
	input.Execution = nil
	input.RequestHash = ""
	input.DocumentMarkdown = ""
	input.Commands = nil
	if err := input.Validate(); err != nil {
		return db.ConversationInteraction{}, err
	}
	if strings.TrimSpace(input.Markdown) == "" {
		return db.ConversationInteraction{}, invalid()
	}
	encoded, _ := json.Marshal(input)
	requestHash := digest(encoded)
	if old, err := s.Store.Queries.GetToolInteraction(ctx, db.GetToolInteractionParams{ToolCallID: call.ID, EnterpriseID: call.Principal.EnterpriseID}); err == nil {
		var prior Plan
		if old.Kind != "plan" || old.OwnerUserID != call.Principal.UserID || old.ConversationID != call.Principal.ConversationID || old.RunID != call.RunID || json.Unmarshal(old.Content, &prior) != nil || prior.RequestHash != requestHash {
			return db.ConversationInteraction{}, conflict()
		}
		return old, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.ConversationInteraction{}, err
	}
	id := uuid.NewSHA1(call.ID, []byte("argus.interaction/v1"))
	remote := s.Remote
	if remote == nil {
		remote = &remoteexecution.Service{}
		if len(input.CommandBindings) > 0 {
			return db.ConversationInteraction{}, invalid()
		}
	}
	manifest, err := remote.FreezePlanCommands(ctx, call, id, input.Title, input.Markdown, input.CommandBindings)
	if err != nil {
		return db.ConversationInteraction{}, err
	}
	input.RequestHash = requestHash
	input.Execution = &manifest
	input.DocumentMarkdown = manifest.Markdown
	for i, step := range manifest.Spec.Steps {
		input.Commands = append(input.Commands, PlanCommand{ID: step.ID, BlockID: manifest.Blocks[i], Target: step.Target.Name, Identity: step.Identity.Username, Interpreter: step.Interpreter})
	}
	if len(manifest.Spec.Inputs) > 0 {
		input.DocumentMarkdown += "\n\n## 输入文件 / Input files\n\n"
		for _, f := range manifest.Spec.Inputs {
			input.DocumentMarkdown += fmt.Sprintf("- %s · %d bytes · SHA-256 %s\n", remoteexecution.EscapePlanText(f.Path), f.Size, f.Hash)
		}
	}
	return s.prepare(ctx, call, "plan", input)
}
