package argusdev

import (
	"context"
	"path/filepath"
	"strings"
)

// Only operational state is exported. Credentials, tool inputs, result bodies,
// private previews and callback trust material are deliberately excluded.
func (a *App) collectScenarioState(ctx context.Context, env *E2EEnvironment) {
	if env.State == nil || !env.installed {
		return
	}
	queries := map[string]string{
		"pending-actions":       `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,action_ref,enterprise_id,creator_subject_id,action_type,status,created_at,updated_at,expires_at FROM pending_actions ORDER BY created_at DESC LIMIT 200) t`,
		"pending-action-tokens": `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,pending_action_id,enterprise_id,status,consumed_at,expires_at,created_at,updated_at FROM pending_action_tokens ORDER BY created_at DESC LIMIT 200) t`,
		"executions":            `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,pending_action_id,status,error_code,created_at,updated_at FROM executions ORDER BY created_at DESC LIMIT 200) t`,
		"connection-tests":      `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,status,error_code,result->>'architecture' AS architecture,result->>'callback_verified' AS callback_verified,result->>'callback_control_path' AS callback_control_path,created_at,updated_at FROM connection_tests ORDER BY created_at DESC LIMIT 100) t`,
		"runs":                  `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,conversation_id,status,stop_reason,error_code,created_at,updated_at FROM runs ORDER BY created_at DESC LIMIT 100) t`,
		"model-calls":           `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,run_id,step_id,call_kind,status,error_code,stop_reason,input_tokens,output_tokens,input_usage_source,output_usage_source,cached_input_tokens,cached_input_usage_source,latency_ms,created_at,completed_at FROM model_calls ORDER BY created_at DESC LIMIT 200) t`,
		"runtime-tasks":         `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,run_id,queue,status,attempt,max_attempts,last_error_code,lease_until,fence_token,created_at,updated_at FROM runtime_tasks ORDER BY created_at DESC LIMIT 200) t`,
		"tool-calls":            `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,run_id,tool_id,source,status,error_code,created_at FROM tool_calls ORDER BY created_at DESC LIMIT 100) t`,
		"workspaces":            `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,conversation_id,status,capacity_bytes,fence_token,lease_owner,lease_until,last_used_at,active_pod_name,active_sandbox_id,created_at,updated_at FROM workspaces ORDER BY created_at DESC LIMIT 100) t`,
		"alert-configurations":  `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,kind,name,status,version,generation,evaluation_health,evaluation_error,last_evaluated_at,last_successful_at FROM alert_configurations ORDER BY updated_at DESC LIMIT 100) t`,
		"alert-instances":       `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,policy_id,policy_version,resource_ids,source_ids,lifecycle,health,severity,acknowledged,handling_version,version,started_at,resolved_at,last_evaluated_at FROM alert_instances ORDER BY updated_at DESC LIMIT 200) t`,
		"alert-evaluations":     `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,policy_id,policy_version,slot,evaluator_id,authorization_version,scope,health,error_code,fence FROM alert_evaluations ORDER BY slot DESC LIMIT 200) t`,
		"alert-deliveries":      `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,instance_id,channel_id,channel_version,batch_id,resend_of_id,status,attempt,last_error,available_at,retry_until FROM alert_deliveries ORDER BY updated_at DESC LIMIT 200) t`,
		"alert-batches":         `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,channel_id,channel_version,owner_task_id,fence,status,send_started_at,finished_at FROM alert_delivery_batches ORDER BY created_at DESC LIMIT 200) t`,
		"alert-attempts":        `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,delivery_id,attempt,status,result_code,error_code,started_at,finished_at FROM alert_delivery_attempts ORDER BY started_at DESC LIMIT 200) t`,
	}
	if env.Options.Suite == "plan7" {
		queries["run-tools"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,mode_snapshot,tool_snapshot->>'sandbox_status' AS workspace_status,jsonb_path_query_array(tool_snapshot,'$.tools[*].model.name') AS tools FROM runs ORDER BY created_at DESC LIMIT 100) t`
		queries["sandbox-capacity"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT enterprise_id,max_concurrent_sessions,monthly_session_seconds,version FROM sandbox_quotas) t`
		queries["sandbox-sessions"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,enterprise_id,workspace_id,status,started_at,expires_at,updated_at FROM sandbox_sessions ORDER BY created_at DESC LIMIT 100) t`
		queries["remote-requests"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,conversation_id,owner_user_id,run_id,action_ref,revision,status,stop_requested,stop_status,error_code,content_hash,created_at,updated_at FROM remote_execution_requests ORDER BY created_at DESC LIMIT 200) t`
		queries["remote-steps"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,request_id,session_id,generation,sequence,status,command_id,result->>'error_code' AS error_code,result->>'exit_code' AS exit_code,result->>'terminated' AS terminated,result->>'outputs_collected' AS outputs_collected,started_at,completed_at,updated_at FROM remote_execution_steps ORDER BY updated_at DESC LIMIT 200) t`
		queries["remote-sessions"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,conversation_id,resource_type,resource_id,identity_id,generation,status,context_revision,active_request_id,runtime_account,interpreter_version,last_active_at FROM remote_execution_sessions ORDER BY last_active_at DESC LIMIT 200) t`
		queries["remote-connector-commands"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,command_id,connector_id,connection_epoch,command_type,status,error_code,dispatch_rejected,credential_lease_id,created_at,updated_at FROM connector_commands WHERE command_type LIKE 'remote_execution_%' ORDER BY created_at DESC LIMIT 200) t`
		queries["remote-direct-commands"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,command_id,session_id,generation,owner_instance,command_type,status,error_code,dispatch_rejected,created_at,updated_at FROM remote_execution_direct_commands ORDER BY created_at DESC LIMIT 200) t`
		queries["remote-transfers"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT id,request_id,session_id,direction,path,byte_size,content_hash,status,transferred_bytes,created_at FROM remote_execution_files ORDER BY created_at DESC LIMIT 200) t`
		queries["remote-model-activity"] = `SELECT coalesce(json_agg(t),'[]') FROM (SELECT model_call_id,run_id,authorization_version,encode(projection_hash,'hex') AS projection_hash,activity,created_at FROM remote_execution_context_observations ORDER BY created_at DESC LIMIT 200) t`
	}
	for name, query := range queries {
		value, err := a.postgresQuery(ctx, env, query)
		if err == nil {
			_ = writePrivate(filepath.Join(env.Options.Artifacts, "state-"+name+".json"), []byte(strings.TrimSpace(value)+"\n"))
		}
	}
}
