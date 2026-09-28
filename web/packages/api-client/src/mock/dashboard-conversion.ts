import type { DashboardSchemas } from "../dashboard";

// Mock UI fixtures only. Native languages are parsed exclusively by the real
// Go service. More expressive inputs are kept intact instead of approximated.
export function mockConvertPanel(
  input: DashboardSchemas["DashboardConvertPanelInput"],
): DashboardSchemas["DashboardConvertedPanel"] {
  const panel = structuredClone(input.panel);
  const unsupported = (id: string) => ({
    converted: false,
    panel: structuredClone(input.panel),
    issues: [
      {
        path: id,
        code: "MOCK_CONVERSION_UNSUPPORTED",
        message:
          "Mock conversion covers only simple unparameterized metric fixtures. Use the real service for native query conversion.",
      },
    ],
  });
  for (const target of [...panel.targets, ...panel.detail_query_targets]) {
    if (
      (input.mode === "builder" && target.source_definition.builder) ||
      (input.mode === "dsl" && target.source_definition.dsl)
    )
      continue;
    if (target.language !== "promql" || target.parameter_bindings.length)
      return unsupported(target.id);
    if (input.mode === "dsl") {
      const builder = target.source_definition.builder!;
      if (
        builder.filters.length ||
        builder.group_by.length ||
        !["value", "sum", "avg", "min", "max"].includes(builder.operation) ||
        !/^\w+$/.test(builder.metric ?? "")
      )
        return unsupported(target.id);
      const selector = `${builder.metric}{}`;
      target.source_definition = {
        dsl: {
          expression:
            builder.operation === "value"
              ? selector
              : `${builder.operation} (${selector})`,
        },
      };
    } else {
      const query = target.source_definition.dsl!;
      if (
        query.pipeline ||
        query.operation ||
        Object.keys(query.variables ?? {}).length
      )
        return unsupported(target.id);
      const simple = /^\s*([A-Za-z_:][A-Za-z0-9_:]*)\s*(?:\{\s*\})?\s*$/.exec(
        query.expression,
      );
      const aggregate =
        /^\s*(sum|avg|min|max)\s*\(\s*([A-Za-z_:][A-Za-z0-9_:]*)\s*(?:\{\s*\})?\s*\)\s*$/.exec(
          query.expression,
        );
      if (!simple && !aggregate) return unsupported(target.id);
      target.source_definition = {
        builder: {
          operation: (aggregate?.[1] ?? "value") as "value",
          metric: aggregate?.[2] ?? simple![1],
          filters: [],
          group_by: [],
        },
      };
    }
  }
  panel.authoring_mode = input.mode;
  return { converted: true, panel, issues: [] };
}
