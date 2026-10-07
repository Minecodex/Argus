import type {
  DashboardExecution,
  DashboardSchemas,
  DashboardSpec,
  DashboardPanel,
} from "./dashboard";
import { ApiError } from "./transport/errors";
const invalid = () =>
  new ApiError(
    {
      code: "DASHBOARD_INVALID",
      message_key: "errors.codes.DASHBOARD_INVALID",
      request_id: "",
      retryable: false,
    },
    400,
  );

/** The editor and explicit mock adapter share scope/freshness semantics, not query execution. */
export function dashboardPreviewSpec(
  value: DashboardSpec,
  ids: string[] = [],
): DashboardSpec {
  const spec = structuredClone(value);
  if (!ids.length) return spec;
  if (
    new Set(ids).size !== ids.length ||
    ids.some((id) => !spec.panels.some((p) => p.id === id))
  )
    throw invalid();
  spec.panels = spec.panels.filter((p) => ids.includes(p.id));
  const variables = new Map(spec.variables.map((v) => [v.name, v])),
    used = new Set<string>();
  const visit = (name: string) => {
    if (spec.variables.filter((v) => v.name === name).length > 1)
      throw invalid();
    if (used.has(name)) return;
    const variable = variables.get(name);
    if (!variable) throw invalid();
    used.add(name);
    mark(variable.query.filters, variable.query.parameter_bindings);
  };
  const mark = (
    filters: DashboardSchemas["DashboardFilter"][] | undefined,
    bindings: DashboardSchemas["DashboardParameterBinding"][] | undefined,
  ) => {
    for (const f of filters ?? []) if (f.variable) visit(f.variable);
    for (const b of bindings ?? []) if (b.variable) visit(b.variable);
  };
  for (const p of spec.panels) {
    for (const t of [...p.targets, ...p.detail_query_targets]) {
      mark(t.source_definition.builder?.filters, t.parameter_bindings);
      mark(t.source_definition.builder?.error_filters, []);
    }
    for (const local of p.local_filters)
      if (local.query)
        mark(local.query.filters, local.query.parameter_bindings);
  }
  spec.variables = spec.variables
    .filter((v) => used.has(v.name))
    .sort((a, b) => a.name.localeCompare(b.name));
  return spec;
}
export function dashboardPreviewDefinition(
  spec: DashboardSpec,
  ids: string[] = [],
): string {
  const scoped = dashboardPreviewSpec(spec, ids);
  return JSON.stringify({
    variables: scoped.variables.map(
      ({ name, multiple, include_all, query }) => ({
        name,
        multiple,
        include_all,
        query,
      }),
    ),
    panels: scoped.panels.map((p) => ({
      id: p.id,
      signal: p.signal,
      authoring_mode: p.authoring_mode,
      applicable_resource_types: p.applicable_resource_types,
      source_binding: p.source_binding,
      targets: p.targets,
      detail_query_targets: p.detail_query_targets,
      drilldowns: p.drilldowns.map((d) => ({ ...d, title: undefined })),
      local_filters: p.local_filters.map((f) => ({
        ...f,
        label: undefined,
        default: undefined,
      })),
    })),
  });
}
export function dashboardPreviewParameters(
  spec: DashboardSpec,
  input: DashboardSchemas["DashboardExecutionInput"],
): DashboardSchemas["DashboardExecutionInput"] {
  const names = new Set(spec.variables.map((v) => v.name)),
    panels = new Set(spec.panels.map((p) => p.id));
  return {
    ...input,
    variables: Object.fromEntries(
      Object.entries(input.variables ?? {}).filter(([name]) => names.has(name)),
    ),
    local_values: Object.fromEntries(
      Object.entries(input.local_values ?? {}).filter(([id]) => panels.has(id)),
    ),
  };
}
export function panelPreviewIdentity(
  panel: DashboardPanel,
  spec: DashboardSpec,
  parameters: DashboardSchemas["DashboardExecutionInput"],
): string {
  return JSON.stringify([
    dashboardPreviewDefinition(
      {
        ...spec,
        panels: spec.panels.map((p) => (p.id === panel.id ? panel : p)),
      },
      [panel.id],
    ),
    dashboardPreviewParameters(
      dashboardPreviewSpec(spec, [panel.id]),
      parameters,
    ),
  ]);
}
export function previewPanelIds(execution: DashboardExecution) {
  return execution.panels.map((p) => p.id);
}
