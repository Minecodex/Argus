import type {
  DashboardSpec,
  DashboardSelection,
  DashboardTarget,
} from "@argus/api-client";

// Query syntax remains server-owned. Published structured bindings describe
// the dependencies for both builder and statement targets.
function targetVariables(target: DashboardTarget): string[] {
  const builder = target.source_definition.builder;
  return builder
    ? [...builder.filters, ...(builder.error_filters ?? [])].flatMap((f) =>
        f.variable ? [f.variable] : [],
      )
    : target.parameter_bindings.flatMap((b) =>
        b.variable ? [b.variable] : [],
      );
}

export function dependentPanels(
  spec: DashboardSpec,
  changed: string[],
): string[] {
  const affected = new Set(changed);
  let expanded = true;
  while (expanded) {
    expanded = false;
    for (const variable of spec.variables) {
      if (
        !affected.has(variable.name) &&
        variable.query.filters.some(
          (f) => f.variable && affected.has(f.variable),
        )
      ) {
        affected.add(variable.name);
        expanded = true;
      }
    }
  }
  return spec.panels
    .filter(
      (panel) =>
        [...panel.targets, ...panel.detail_query_targets].some((target) =>
          targetVariables(target).some((name) => affected.has(name)),
        ) ||
        panel.local_filters.some((filter) =>
          filter.query?.filters.some(
            (f) => f.variable && affected.has(f.variable),
          ),
        ),
    )
    .map((panel) => panel.id);
}

export function changedVariables(
  before: Record<string, DashboardSelection> = {},
  after: Record<string, DashboardSelection> = {},
): string[] {
  return [...new Set([...Object.keys(before), ...Object.keys(after)])].filter(
    (name) => {
      const a = before[name],
        b = after[name];
      return (
        !a ||
        !b ||
        a.all !== b.all ||
        (!a.all &&
          JSON.stringify([...a.values].sort()) !==
            JSON.stringify([...b.values].sort()))
      );
    },
  );
}
