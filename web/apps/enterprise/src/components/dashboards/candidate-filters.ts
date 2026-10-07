import type { DashboardSchemas, DashboardSelection } from "@argus/api-client";

export function candidateFilters(
  query: DashboardSchemas["DashboardCandidateQuery"],
  variables: Record<string, DashboardSelection>,
  locals: Record<string, DashboardSelection> = {},
): DashboardSchemas["DashboardCatalogFilter"][] {
  return query.filters.flatMap((filter) => {
    let selected = filter.variable
      ? variables[filter.variable]
      : filter.local_parameter
        ? locals[filter.local_parameter]
        : { all: false, values: filter.values ?? [filter.value ?? ""] };
    if (!selected) throw new Error("DASHBOARD_PARAMETER_UNRESOLVED");
    if (selected.all) return [];
    const mapping = query.parameter_bindings?.find(
      (b) =>
        (filter.variable && b.variable === filter.variable) ||
        (filter.local_parameter &&
          b.local_parameter === filter.local_parameter),
    );
    if (mapping?.value_map && Object.keys(mapping.value_map).length)
      selected = {
        ...selected,
        values: selected.values.map((value) => {
          const mapped = mapping.value_map![value];
          if (mapped === undefined)
            throw new Error("DASHBOARD_PARAMETER_UNMAPPED");
          return mapped;
        }),
      };
    if (!["=", "!=", ">", ">=", "<", "<="].includes(filter.operator))
      throw new Error("DASHBOARD_CATALOG_OPERATOR_UNSUPPORTED");
    return [
      {
        field: filter.field,
        operator:
          filter.operator as DashboardSchemas["DashboardCatalogFilter"]["operator"],
        values: selected.values,
      },
    ];
  });
}

type CandidateScope = {
  time: unknown;
  resource_ids?: string[];
  variables?: Record<string, DashboardSelection>;
  locals?: Record<string, DashboardSelection>;
};

/** Only referenced values invalidate this candidate query. Its own selection does not. */
export function candidateContext(
  query: DashboardSchemas["DashboardCandidateQuery"],
  scope: CandidateScope,
) {
  const names = (kind: "variable" | "local_parameter") =>
    [
      ...new Set(query.filters.flatMap((f) => (f[kind] ? [f[kind]!] : []))),
    ].sort();
  return JSON.stringify([
    query,
    scope.time,
    scope.resource_ids ?? [],
    names("variable").map((name) => [name, scope.variables?.[name]]),
    names("local_parameter").map((name) => [name, scope.locals?.[name]]),
  ]);
}

export function candidateRequest(
  query: DashboardSchemas["DashboardCandidateQuery"],
  scope: Omit<CandidateScope, "time"> & { from: string; to: string },
  selection: DashboardSelection,
  search: string,
  cursor?: string,
): DashboardSchemas["DashboardCatalogInput"] {
  return {
    signal: query.signal,
    source_binding: query.source_binding,
    metric: query.metric,
    kind: "values",
    field: query.field,
    from: scope.from,
    to: scope.to,
    resource_ids: scope.resource_ids ?? [],
    filters: candidateFilters(query, scope.variables ?? {}, scope.locals ?? {}),
    selected_values: selection.all ? [] : selection.values,
    search,
    cursor,
    limit: 100,
  };
}
