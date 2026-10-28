import type { TelemetryCompletionRequest } from "./telemetry-editor-context";
type Filters = NonNullable<TelemetryCompletionRequest["filters"]>;
// This only recognizes completed conjunctions for narrowing suggestions. It is
// not a query parser: an unrepresentable context must not broaden discovery.
export function completionFilters(
  language: string,
  prefix: string,
): Filters | undefined {
  if (language === "skywalking_graphql") {
    const args = prefix.slice(prefix.lastIndexOf("(") + 1).trim();
    if (!args) return [];
    const names: Record<string, string> = {
      serviceName: "service_name",
      operationName: "operation",
      serviceInstanceName: "resource_attributes.service.instance.id",
      traceId: "trace_id",
      sourceId: "source_id",
      resourceId: "resource_id",
      status: "status",
    };
    const pattern = /\s*([\w]+)\s*:\s*("(?:\\.|[^"\\])*"|[0-9]+)\s*,?\s*/gy;
    let position = 0;
    const filters: Filters = [];
    while (position < args.length) {
      pattern.lastIndex = position;
      const m = pattern.exec(args);
      if (!m) return undefined;
      position = pattern.lastIndex;
      if (["pageNum", "pageSize"].includes(m[1]!)) continue;
      if (!names[m[1]!]) return undefined;
      try {
        filters.push({
          field: names[m[1]!]!,
          operator: "=",
          values: [String(JSON.parse(m[2]!))],
        });
      } catch {
        return undefined;
      }
    }
    return filters;
  }
  const text = prefix
    .trim()
    .replace(/\bAND\s*$/i, "")
    .trim();
  if (!text || text === "*") return [];
  const pattern =
    /\s*([\w.-]+)\s*(:|!=|>=|<=|=|>|<)\s*("(?:\\.|[^"\\])*"|-?[0-9]+(?:\.[0-9]+)?)\s*(?:AND\b\s*|$)/giy;
  const filters: Filters = [];
  let position = 0;
  while (position < text.length) {
    pattern.lastIndex = position;
    const m = pattern.exec(text);
    if (!m || filters.length >= 32) return undefined;
    position = pattern.lastIndex;
    try {
      filters.push({
        field: m[1]!,
        operator: m[2] as Filters[number]["operator"],
        values: [String(JSON.parse(m[3]!))],
      });
    } catch {
      return undefined;
    }
  }
  return filters;
}
