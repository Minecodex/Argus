import { useEffect, useMemo, useState } from "react";
import { useQueries, useQueryClient } from "@tanstack/react-query";
import { useEnterpriseAuthStore } from "@argus/auth";
import { useApi, type Host } from "@argus/api-client";
import { usePermission } from "../../lib/permissions";
import { useDocumentVisibility } from "../../lib/use-document-visibility";
import {
  hostMetricKinds,
  hostMetricExpression,
  parseHostMetricResponse,
  HOST_TREND_SECONDS,
  HOST_TREND_STEP,
  type HostMetricGroup,
  type HostMetricKind,
} from "./host-list-metrics";

export function useHostListMetrics({
  hosts,
  ready,
  refreshKey,
  scopeKey,
}: {
  hosts: Host[];
  ready: boolean;
  refreshKey: number;
  scopeKey: string;
}): Record<HostMetricKind, HostMetricGroup> {
  const api = useApi(),
    cache = useQueryClient();
  const actor = useEnterpriseAuthStore((state) => state.session?.session.id);
  // Host queries use the resource read permission; PromQL tool IDs are not RBAC permissions.
  const canRead = usePermission("host.read");
  const { visible, epoch } = useDocumentVisibility();
  const [interval, setIntervalEpoch] = useState(0);
  useEffect(() => {
    if (!visible || !canRead || !ready || hosts.length === 0) return;
    const timer = window.setInterval(
      () => setIntervalEpoch((value) => value + 1),
      60000,
    );
    return () => window.clearInterval(timer);
  }, [visible, canRead, ready, hosts.length]);
  const signature = JSON.stringify({
    ids: [...new Set(hosts.map((host) => host.id))].sort(),
    scopeKey,
    refreshKey,
    interval,
    epoch,
  });
  const context = useMemo(() => {
    const { ids } = JSON.parse(signature) as { ids: string[] };
    const to =
      Math.floor(Date.now() / (HOST_TREND_STEP * 1000)) * HOST_TREND_STEP;
    return {
      ids,
      from: to - HOST_TREND_SECONDS,
      to,
      nonce: crypto.randomUUID(),
    };
  }, [signature]);
  const key = useMemo(
    () => ["telemetry", "host-list-trends", actor, signature, context.nonce],
    [actor, signature, context.nonce],
  );
  const active = ready && visible && canRead && context.ids.length > 0;
  useEffect(() => {
    if (!active) void cache.cancelQueries({ queryKey: key });
    return () => {
      void cache.cancelQueries({ queryKey: key });
    };
  }, [active, cache, key]);
  const queries = useQueries({
    queries: hostMetricKinds.map((metric) => ({
      queryKey: [...key, metric],
      enabled: active,
      retry: false,
      staleTime: Infinity,
      gcTime: 120000,
      queryFn: async ({ signal }: { signal: AbortSignal }) => {
        const response = await api.telemetry.queryMetricsRange(
          {
            query: hostMetricExpression(metric),
            scope: {
              mode: "explicit",
              resources: context.ids.map((id) => ({
                type: "host" as const,
                id,
              })),
              source_type: "hostmetrics",
            },
            time_range: {
              from: new Date(context.from * 1000).toISOString(),
              to: new Date(context.to * 1000).toISOString(),
            },
            step_seconds: HOST_TREND_STEP,
            budget: {
              max_scan_bytes: 268435456,
              max_rows: 20000,
              max_samples: 1000000,
              max_series: 10000,
              max_result_bytes: 8388608,
              timeout_ms: 10000,
            },
          },
          signal,
        );
        signal.throwIfAborted();
        return parseHostMetricResponse(
          response,
          context.ids,
          metric,
          context.from,
          context.to,
        );
      },
    })),
  });
  return Object.fromEntries(
    hostMetricKinds.map((metric, index) => [
      metric,
      {
        loading: canRead && (!active || queries[index]!.isPending),
        denied: !canRead,
        error: active ? queries[index]!.error : undefined,
        data: active ? queries[index]!.data : undefined,
      },
    ]),
  ) as Record<HostMetricKind, HostMetricGroup>;
}
