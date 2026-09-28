import type { DashboardExecution, DashboardSchemas } from "../dashboard";
import type { MockDashboards } from "./dashboard-state";
import {
  copyDashboard as copy,
  dashboardFailure as fail,
} from "./dashboard-state";

type Job = NonNullable<MockDashboards["queryJobs"]>[number];
export function mockFileDrilldown(
  parent: Job,
  input: DashboardSchemas["DashboardQueryJobInput"],
): { execution: DashboardExecution; context: Record<string, unknown> } {
  const request = input.drilldown!;
  if (!parent.value.manifest) fail("DASHBOARD_SELECTION_STALE", 409);
  if (parent.value.dashboard_id !== input.dashboard_id)
    fail("DASHBOARD_DENIED", 403);
  const parameters = input.parameters;
  if (
    parameters.from ||
    parameters.to ||
    parameters.resource_ids?.length ||
    parameters.panel_ids?.length ||
    Object.keys(parameters.variables ?? {}).length ||
    Object.keys(parameters.local_values ?? {}).length
  )
    fail();
  const panel =
    parent.spec.panels.find((p) => p.id === request.panel_id) ?? fail();
  const drill =
    panel.drilldowns.find((d) => d.id === request.drilldown_id) ?? fail();
  if (
    (drill.scope_policy === "authorized_trace") !==
    request.expand_authorized_resources
  )
    fail();
  const target =
    panel.detail_query_targets.find((t) => t.id === drill.detail_query_ref) ??
    fail();
  const origin =
    [...panel.targets, ...panel.detail_query_targets].find(
      (t) => t.id === drill.origin_query_ref,
    ) ?? fail();
  const depth = Number(parent.drilldown?.depth ?? 0);
  if (!Number.isInteger(depth) || depth >= 16)
    fail("DASHBOARD_CONTEXT_EXPIRED", 409);
  if (
    parent.drilldown &&
    (parent.drilldown.panel_id !== panel.id ||
      parent.drilldown.target_id !== origin.id)
  )
    fail();
  if (!parent.drilldown && !panel.targets.some((t) => t.id === origin.id))
    fail();
  const previous =
    parent.execution.panels.find((p) => p.id === panel.id) ?? fail();
  const result = previous.targets.find((t) => t.id === origin.id) ?? fail();
  // Mock only proves exact inspect copies. Other published native definitions
  // require the real parser/runtime; never silently query the whole dashboard.
  if (
    drill.scope_policy !== "inherit" ||
    request.expand_authorized_resources ||
    Object.keys(drill.inputs).length ||
    Object.keys(request.values).length ||
    drill.time_window ||
    target.signal !== (origin.signal ?? panel.signal) ||
    JSON.stringify(target.source_binding) !==
      JSON.stringify(origin.source_binding ?? panel.source_binding) ||
    JSON.stringify([
      target.language,
      target.source_definition,
      target.parameter_bindings,
      target.query_mode,
      target.range_step_policy,
    ]) !==
      JSON.stringify([
        origin.language,
        origin.source_definition,
        origin.parameter_bindings,
        origin.query_mode,
        origin.range_step_policy,
      ])
  )
    fail("DASHBOARD_UNAVAILABLE", 503);
  const execution = copy(parent.execution);
  execution.execution_id = crypto.randomUUID();
  execution.execution_hash = crypto.randomUUID();
  delete execution.context_token;
  delete execution.context_expires_at;
  execution.panels = [
    { ...copy(previous), targets: [{ ...copy(result), id: target.id }] },
  ];
  execution.partial = ["error", "partial", "skipped_budget"].includes(
    result.status,
  );
  const attempt = parent.value.manifest.attempt_id;
  if (typeof attempt !== "string") fail();
  return {
    execution,
    context: {
      parent_job_id: parent.value.id,
      parent_attempt_id: attempt,
      parent_execution_id: parent.execution.execution_id,
      panel_id: panel.id,
      drilldown_id: drill.id,
      origin_query_ref: origin.id,
      target_id: target.id,
      scope_policy: "inherit",
      inputs: {},
      depth: depth + 1,
    },
  };
}
