import { useState } from "react";
import { useQueries, useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type KubernetesResourceReference,
  type DashboardSchemas,
} from "@argus/api-client";
import { ObservationPanel, Button, Alert, EmptyState } from "@argus/ui";
import { TimeRangePicker } from "../dashboards/time-range-picker";
import { queryScopeBounds } from "../dashboards/query-scope";
import { ResourceObserveLink } from "../observe/resource-link";
const definitions: Record<
  string,
  { metric: string; unit: string; title: string }[]
> = {
  pod: [
    { metric: "k8s_pod_cpu_usage", unit: "cores", title: "CPU" },
    { metric: "k8s_pod_memory_usage", unit: "bytes", title: "memory" },
  ],
  node: [
    { metric: "k8s_node_cpu_usage", unit: "cores", title: "CPU" },
    { metric: "k8s_node_memory_usage", unit: "bytes", title: "memory" },
  ],
  deployment: [
    { metric: "k8s_deployment_available", unit: "", title: "fields.ready" },
    {
      metric: "k8s_deployment_desired",
      unit: "",
      title: "fields.desired_replicas",
    },
  ],
  daemonset: [
    { metric: "k8s_daemonset_ready_nodes", unit: "", title: "fields.ready" },
  ],
};
export const supportsResourceMetrics = (kind: string) => !!definitions[kind];
export function KubernetesResourceMetrics({
  clusterId,
  reference,
}: {
  clusterId: string;
  reference: KubernetesResourceReference;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    [range, setRange] = useState<DashboardSchemas["DashboardTimeRange"]>({
      kind: "relative",
      seconds: 3600,
    }),
    [bounds, setBounds] = useState(() => queryScopeBounds(range));
  const resources = [
    {
      type: reference.namespace_id
        ? ("kubernetes_namespace" as const)
        : ("kubernetes_cluster" as const),
      id: reference.namespace_id ?? clusterId,
    },
  ];
  const metrics = definitions[reference.type] ?? [];
  const sourceType = ["pod", "node"].includes(reference.type)
    ? "kubeletstats"
    : "k8s_cluster";
  const sources = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "metric-sources",
      reference.namespace_id,
      sourceType,
      bounds,
    ],
    enabled: metrics.length > 0,
    queryFn: ({ signal }) =>
      api.telemetry.catalog(
        {
          scope: { mode: "explicit", resources, source_type: sourceType },
          signal: "metrics",
          kind: "sources",
          observed_only: false,
          ...bounds,
          limit: 100,
        },
        signal,
      ),
    retry: false,
  });
  const label =
    reference.type === "node"
      ? "resource_k8s_node_name"
      : "resource_k8s_" + reference.type + "_uid";
  const identity = reference.type === "node" ? reference.name : reference.uid;
  const queries = useQueries({
    queries: metrics.map((metric) => ({
      queryKey: [
        "kubernetes-workbench",
        clusterId,
        "metrics",
        reference.uid,
        reference.namespace_id,
        metric.metric,
        bounds,
      ],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        api.telemetry.queryMetricsRange(
          {
            scope: { mode: "explicit", resources },
            query:
              metric.metric +
              "{" +
              label +
              "=" +
              JSON.stringify(identity) +
              "}",
            time_range: bounds,
            step_seconds: Math.max(
              1,
              Math.ceil(
                (Date.parse(bounds.to) - Date.parse(bounds.from)) / 300000,
              ),
            ),
          },
          signal,
        ),
      retry: false,
    })),
  });
  return (
    <div className="argus-dashboard-form-stack">
      <div className="argus-dashboard-inline">
        <TimeRangePicker
          label={t("dashboards.time")}
          value={range}
          onChange={(value) => {
            setRange(value);
            setBounds(queryScopeBounds(value));
          }}
        />
        <Button onPress={() => setBounds(queryScopeBounds(range))}>
          {t("common.refresh")}
        </Button>
        <ResourceObserveLink type={resources[0]!.type} id={resources[0]!.id} />
      </div>
      {!metrics.length && <p>{t("kubernetesResources.unsupportedMetrics")}</p>}
      {metrics.map((metric, index) => {
        const query = queries[index]!,
          response = query.data;
        return (
          <section key={metric.metric} className="argus-k8s-resource-metric">
            <h3>
              {metric.title === "CPU"
                ? "CPU"
                : t("kubernetesResources." + metric.title)}
            </h3>
            {query.error ? (
              <Alert
                tone="warning"
                title={t("dashboards.failed")}
                description={formatApiError(
                  query.error,
                  t("kubernetesResources.notAvailable"),
                  (id) => t("common.requestReference", { requestId: id }),
                )}
              />
            ) : query.isPending ? (
              <p>{t("common.loading")}</p>
            ) : response &&
              Array.isArray(response.data.result) &&
              response.data.result.length === 0 &&
              !response.argus_meta.partial ? (
              <EmptyState
                title={t(
                  sources.data?.complete &&
                    !sources.data.has_more &&
                    !sources.data.sources.some(
                      (source) => source.source_type === sourceType,
                    )
                    ? "kubernetesResources.noSource"
                    : "kubernetesResources.noMetricSamples",
                )}
                description={t("kubernetesResources.metricScopeHint")}
              />
            ) : (
              <ObservationPanel
                title={metric.metric}
                type="timeseries"
                signal="metrics"
                unit={metric.unit}
                targets={
                  response
                    ? [
                        {
                          id: "metric",
                          status: response.argus_meta.partial
                            ? "partial"
                            : "success",
                          meta: { warnings: response.warnings },
                          result_type: response.data.resultType,
                          data: response.data.result,
                        },
                      ]
                    : []
                }
              />
            )}
          </section>
        );
      })}
    </div>
  );
}
