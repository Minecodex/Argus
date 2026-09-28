import type { DashboardExecution, DashboardPanel } from "@argus/api-client";

export function hasUnverifiedCandidates(
  execution?: DashboardExecution,
): boolean {
  if (!execution) return false;
  return [
    ...Object.values(execution.variable_candidates),
    ...Object.values(execution.local_candidates).flatMap(Object.values),
  ].some((candidate) =>
    ["unavailable", "skipped_budget"].includes(candidate.status),
  );
}

// Use the execution's resolved resource set. An empty local set must never be
// sent as [] to Catalog, where it means all authorized resources.
export function panelCandidateResources(
  execution: DashboardExecution | undefined,
  panel: DashboardPanel,
): string[] {
  return (execution?.resources ?? [])
    .filter((resource) =>
      panel.applicable_resource_types.includes(resource.type),
    )
    .map((resource) => resource.id);
}
