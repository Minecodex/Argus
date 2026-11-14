import { useEffect, useMemo } from "react";
import { useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEnterpriseAuthStore } from "@argus/auth";
import { useApi, type KubernetesInventoryInput } from "@argus/api-client";
import { useDocumentVisibility } from "../../lib/use-document-visibility";

export const supportsListMetrics = (kind: string) =>
  [
    "node",
    "namespace",
    "pod",
    "deployment",
    "statefulset",
    "daemonset",
    "replicaset",
    "pvc",
  ].includes(kind);

export function useListMetrics({
  clusterId,
  input,
  uids,
  updatedAt,
  fetching,
  scopeKey,
}: {
  clusterId: string;
  input: KubernetesInventoryInput;
  uids: string[];
  updatedAt: number;
  fetching: boolean;
  scopeKey: string;
}) {
  const api = useApi(),
    cache = useQueryClient();
  const actor = useEnterpriseAuthStore((state) => state.session?.user.id);
  const { visible, epoch: visibilityEpoch } = useDocumentVisibility();
  const signature = JSON.stringify({
    input,
    uids,
    updatedAt,
    scopeKey,
    visibilityEpoch,
  });
  const key = useMemo(
    () => ["kubernetes-list-metrics", actor, clusterId, signature],
    [actor, clusterId, signature],
  );
  const active =
    visible && !fetching && uids.length > 0 && supportsListMetrics(input.type);
  useEffect(() => {
    if (!active) void cache.cancelQueries({ queryKey: key });
    return () => {
      void cache.cancelQueries({ queryKey: key });
    };
  }, [active, cache, key]);
  const prepared = useQuery({
    queryKey: [...key, "prepare"],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.prepareMetrics(
        clusterId,
        { list: input, uids },
        signal,
      ),
    enabled: active,
    retry: false,
    staleTime: Infinity,
    gcTime: 120000,
  });
  const kinds =
    input.type === "pvc"
      ? ["storage" as const]
      : (["cpu", "memory", "storage"] as const);
  const queries = useQueries({
    queries: kinds.map((metric) => ({
      queryKey: [...key, metric, prepared.data?.token],
      queryFn: ({ signal }: { signal: AbortSignal }) =>
        api.kubernetesWorkbench.readMetrics(
          clusterId,
          { token: prepared.data!.token, metric },
          signal,
        ),
      enabled: active && !!prepared.data?.token,
      retry: false,
      staleTime: Infinity,
      gcTime: 120000,
    })),
  });
  return Object.fromEntries(
    kinds.map((metric, index) => [
      metric,
      {
        loading: !active || prepared.isPending || queries[index]!.isPending,
        error: active ? prepared.error || queries[index]!.error : undefined,
        data: active ? queries[index]!.data : undefined,
      },
    ]),
  );
}
