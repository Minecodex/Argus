import {
  useQueryCatalog,
  useDashboardQueryScope as useCatalogScope,
} from "./query-scope";
import { useMemo, useRef, type ComponentProps } from "react";
import type { DashboardSchemas } from "@argus/api-client";
import type { ComboBox } from "@argus/ui";
import { queryScopeBounds, type DashboardQueryScope } from "./query-scope";

type Metric = DashboardSchemas["DashboardMetricDescriptor"];
type Request = DashboardSchemas["DashboardCatalogInput"];
type Catalog = import("./query-scope").QueryCatalogLoader;
type Scope = Pick<Request, "source_binding" | "resource_ids"> & {
  time: DashboardQueryScope["time"];
};

// A catalog cursor is bound to the first page's exact time/scope/search.
// Recomputing a relative range on the next page invalidates that cursor.
export function createMetricOptionsLoader(
  catalog: Catalog,
  scope: Scope,
  received?: (metrics: Metric[]) => void,
): NonNullable<ComponentProps<typeof ComboBox>["fetchOptions"]> {
  let range: ReturnType<typeof queryScopeBounds> | undefined;
  let activeSearch: string | undefined;
  return async (search, cursor, signal) => {
    if (!cursor || search !== activeSearch || !range) {
      range = queryScopeBounds(scope.time);
      activeSearch = search;
    }
    const result = await catalog(
      {
        ...range,
        source_binding: scope.source_binding,
        resource_ids: scope.resource_ids,
        signal: "metrics",
        kind: "metrics",
        selected_values: [],
        filters: [],
        limit: 100,
        search,
        cursor,
      },
      signal,
    );
    if (!signal.aborted) received?.(result.metrics);
    return {
      options: result.metrics.map((metric) => ({
        value: metric.name,
        label: metric.name,
        description: [metric.type, metric.unit].filter(Boolean).join(" · "),
      })),
      next_cursor: result.next_cursor,
    };
  };
}

export function useMetricOptions(
  source_binding: Request["source_binding"],
  scope: DashboardQueryScope,
  received?: (metrics: Metric[]) => void,
) {
  const catalogApi = useQueryCatalog(),
    context = useCatalogScope();
  const onReceived = useRef(received);
  onReceived.current = received;
  const scopeKey = JSON.stringify({
    context: context?.key,
    source_binding,
    time: scope.time,
    resource_ids: scope.resources,
  });
  return useMemo(
    () =>
      createMetricOptionsLoader(
        catalogApi,
        JSON.parse(scopeKey) as Scope,
        (metrics) => onReceived.current?.(metrics),
      ),
    [scopeKey, catalogApi],
  );
}
