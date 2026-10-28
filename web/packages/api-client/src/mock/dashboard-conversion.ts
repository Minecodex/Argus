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
          "Mock conversion covers only simple unparameterized fixtures. Use the real service for native query conversion.",
      },
    ],
  });
  for (const target of [...panel.targets, ...panel.detail_query_targets]) {
    if (
      (input.mode === "builder" && target.source_definition.builder) ||
      (input.mode === "dsl" && target.source_definition.dsl)
    )
      continue;
    if (target.parameter_bindings.length) return unsupported(target.id);
    if (target.language !== "promql") {
      // Explicit empty-filter fixtures allow resource-query UI tests. Rich native
      // queries still require the Go compiler rather than a browser parser.
      const builder = target.source_definition.builder;
      if (
        input.mode === "dsl" &&
        builder &&
        !builder.filters.length &&
        !builder.group_by.length
      ) {
        const limit = builder.limit ?? 200;
        if (
          target.language === "kql" &&
          ["records", "count"].includes(builder.operation)
        ) {
          target.source_definition = {
            dsl: {
              expression: "*",
              pipeline: `${builder.operation === "count" ? "stats count() | " : ""}limit ${limit}`,
            },
          };
          continue;
        }
        if (
          target.language === "skywalking_graphql" &&
          builder.operation === "list"
        ) {
          target.source_definition = {
            dsl: {
              expression: `query { queryTraces(pageSize:${limit}) { total traces { traceId sourceId resourceId rootService rootOperation startTime duration status spanCount errorCount } } }`,
            },
          };
          continue;
        }
      }
      return unsupported(target.id);
    }
    if (input.mode === "dsl") {
      const builder = target.source_definition.builder!;
      if (
        builder.filters.some(
          (filter) =>
            filter.variable ||
            filter.local_parameter ||
            filter.drilldown_input ||
            filter.values ||
            filter.value === undefined ||
            !/^[A-Za-z_][A-Za-z0-9_]*$/.test(filter.field) ||
            !["=", "!=", "=~", "!~"].includes(filter.operator),
        ) ||
        builder.group_by.length ||
        !["value", "sum", "avg", "min", "max"].includes(builder.operation) ||
        !/^\w+$/.test(builder.metric ?? "")
      )
        return unsupported(target.id);
      // Fixed literal-label fixtures support the inline editor. This is not a
      // PromQL parser; complex and parameterized conversions still fail closed.
      const selector = `${builder.metric}{${builder.filters.map((filter) => `${filter.field}${filter.operator}${JSON.stringify(filter.value)}`).join(",")}}`;
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
