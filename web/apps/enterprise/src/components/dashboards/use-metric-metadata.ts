import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  useApi,
  type DashboardPanel,
  type DashboardTarget,
} from "@argus/api-client";
import { knownMetricType } from "./metric-metadata";
import { queryScopeBounds, type DashboardQueryScope } from "./query-scope";

export function useMetricMetadata(
  panel: Pick<DashboardPanel, "signal" | "source_binding"> | undefined,
  target: DashboardTarget | undefined,
  scope: DashboardQueryScope,
  onChange: (target: DashboardTarget) => void,
) {
  const api = useApi(),
    current = useRef({ target, onChange });
  current.current = { target, onChange };
  const builder = target?.source_definition.builder;
  const metric = builder?.metric?.replace(/_bucket$/, "");
  const catalog = useQuery({
    queryKey: [
      "dashboard-metric-metadata",
      panel?.source_binding,
      metric,
      scope.time,
      scope.resources,
    ],
    enabled: panel?.signal === "metrics" && !!metric && !!builder,
    queryFn: ({ signal }) =>
      api.dashboards.catalog(
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
  const metadata = catalog.data?.metrics.find((item) => item.name === metric);
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
