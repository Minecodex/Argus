package remoteexecution

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (s Service) ModelFacts(ctx context.Context, run db.Run) (map[string]any, error) {
	activity, err := s.ActivityFacts(ctx, run)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"mode": run.ModeSnapshot, "concurrent_activity": activity, "input_directory": InputDirectory, "results_directory": "/workspace/" + ResultsDirectory, "results_metadata_path": "/workspace/" + ResultsMetadataPath, "output_rule": "Create business-relative folders and files under ARGUS_OUTPUT_DIR on the target; they are collected into /workspace/results. Use actual output_files paths after collision renaming. Read results/meta.json for provenance, then inspect current file bytes."}
	p, err := s.principal(ctx, run.EnterpriseID, run.ActorUserID, run.ConversationID, "remote_execution.read")
	if err != nil {
		return result, nil
	}
	rows, err := s.Store.Queries.ListConversationRemoteRequests(ctx, db.ListConversationRemoteRequestsParams{EnterpriseID: p.EnterpriseID, ConversationID: p.ConversationID, OwnerUserID: p.UserID})
	if err != nil {
		return nil, err
	}
	executions := []map[string]any{}
	budget := 64 << 10
	for _, r := range rows {
		if len(executions) >= 8 {
			break
		}
		v, e := s.Get(ctx, p, r.ID)
		if e != nil {
			continue
		}
		stored, e := s.Store.Queries.ListRemoteSteps(ctx, db.ListRemoteStepsParams{RequestID: r.ID, EnterpriseID: r.EnterpriseID})
		if e != nil {
			return nil, e
		}
		dispatched := map[uuid.UUID]bool{}
		for _, record := range stored {
			dispatched[record.ID] = record.CommandID != ""
		}
		steps := []map[string]any{}
		for n, step := range v.Spec.Steps {
			entry := map[string]any{"sequence": n + 1, "resource": step.Target, "session_id": step.SessionID, "generation": step.Generation, "script_hash": hashBytes([]byte(step.Script))}
			sent, observed := dispatched[step.ID]
			entry["dispatch_state"] = "unavailable"
			if observed {
				entry["dispatch_state"] = "not_sent"
				if sent {
					entry["dispatch_state"] = "sent"
				}
			}
			if n < len(v.Results) {
				fact := v.Results[n]
				entry["status"] = fact.Status
				// The default zero value is not an observed exit code for an
				// unsent command or a missing/untrusted completion receipt.
				if sent && fact.Terminated && !fact.SessionLost && fact.Status != "result_unknown" {
					entry["exit_code"] = fact.ExitCode
				}
				entry["error_code"] = fact.ErrorCode
				entry["terminated"] = fact.Terminated
				entry["outputs_collected"] = fact.OutputsCollected
				entry["output_files"] = fact.OutputFiles[:min(len(fact.OutputFiles), 16)]
				entry["stdout_preview"] = bounded(fact.Stdout, 4096)
				entry["stderr_preview"] = bounded(fact.Stderr, 2048)
			}
			session, e := s.Store.Queries.GetRemoteSession(ctx, db.GetRemoteSessionParams{ID: step.SessionID, EnterpriseID: p.EnterpriseID})
			if e == nil {
				entry["current_session_status"] = session.Status
				entry["current_generation"] = session.Generation
				entry["current_context_revision"] = session.ContextRevision
				entry["known_directory"] = session.KnownDirectory
				entry["runtime_account"] = session.RuntimeAccount
				entry["interpreter_version"] = session.InterpreterVersion
			}
			encoded, _ := json.Marshal(entry)
			if len(encoded) > budget {
				result["truncated"] = true
				break
			}
			budget -= len(encoded)
			steps = append(steps, entry)
		}
		executions = append(executions, map[string]any{"execution_id": v.ID, "action_ref": v.ActionRef, "status": v.Status, "revision": v.Revision, "kind": v.Spec.Kind, "steps": steps, "error_code": v.ErrorCode, "stop_status": v.StopStatus})
		if budget < 4096 {
			result["truncated"] = true
			break
		}
	}
	result["executions"] = executions
	return result, nil
}
func bounded(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && n < len(s) && s[n]&0xc0 == 0x80 {
		n--
	}
	return s[:n] + " [truncated; read the Workspace evidence]"
}
