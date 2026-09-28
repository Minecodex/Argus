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
