import {
  useQueryCatalog,
  useDashboardQueryScope as useCatalogScope,
} from "./query-scope";
import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { type DashboardPanel, type DashboardTarget } from "@argus/api-client";
import { knownMetricType } from "./metric-metadata";
import { queryScopeBounds, type DashboardQueryScope } from "./query-scope";

export function useMetricMetadata(
  panel: Pick<DashboardPanel, "signal" | "source_binding"> | undefined,
  target: DashboardTarget | undefined,
  scope: DashboardQueryScope,
  onChange: (target: DashboardTarget) => void,
) {
  const catalogApi = useQueryCatalog(),
    catalogScope = useCatalogScope();
  const current = useRef({ target, onChange });
  current.current = { target, onChange };
  const builder = target?.source_definition.builder;
  const metric = builder?.metric?.replace(/_bucket$/, "");
  const catalog = useQuery({
    queryKey: [
      "dashboard-metric-metadata",
      catalogScope?.key,
      panel?.source_binding,
      metric,
      scope.time,
      scope.resources,
    ],
    gcTime: 0,
    enabled:
      scope.catalog !== false &&
      panel?.signal === "metrics" &&
      !!metric &&
      !!builder,
    queryFn: ({ signal }) =>
      catalogApi(
        {
          ...queryScopeBounds(scope.time),
          resource_ids: scope.resources,
          signal: "metrics",
          source_binding: panel!.source_binding,
          kind: "metrics",
          metric,
          selected_values: [],
          filters: [],
          limit: 100,
        },
        signal,
      ),
  });
  const matches =
    catalog.data?.metrics.filter((item) => item.name === metric) ?? [];
  const conflicting =
    new Set(matches.map((item) => JSON.stringify([item.type, item.unit])))
      .size > 1;
  const metadata = conflicting
    ? { ...matches[0]!, type: "mixed", unit: "" }
    : matches[0];
  const type = knownMetricType(metadata),
    declared = builder?.metric_type;
  useEffect(() => {
    const { target: latest, onChange: change } = current.current;
    if (type && declared !== type && latest?.source_definition.builder)
      change({
        ...latest,
        source_definition: {
          builder: { ...latest.source_definition.builder, metric_type: type },
        },
      });
  }, [type, declared, target?.id, metric]);
  return { metadata, catalog };
}
