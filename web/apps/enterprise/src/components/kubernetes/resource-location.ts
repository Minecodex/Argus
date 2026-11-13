import {
  parseDashboardTime,
  type DashboardTimeSearch,
} from "../dashboards/url-time-range";
import { useCallback } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
export type KubernetesResourceSearch = DashboardTimeSearch & {
  tab?: "overview" | "resources" | "dashboards" | "collector" | "activity";
  kind?: string;
  namespace_id?: string;
  namespace_name?: string;
  state_filter?: string;
  name_filter?: string;
  label_filter?: string;
  page_cursor?: string;
  page_index?: number;
  resource_operation?: string;
  resource_type?: string;
  resource_name?: string;
  resource_uid?: string;
  resource_namespace?: string;
  resource_namespace_id?: string;
  full?: boolean;
};
export function parseKubernetesSearch(
  search: Record<string, unknown>,
): KubernetesResourceSearch {
  const text = (key: string, max = 1024) =>
    typeof search[key] === "string"
      ? (search[key] as string).slice(0, max)
      : undefined;
  const tab = [
    "overview",
    "resources",
    "dashboards",
    "collector",
    "activity",
  ].includes(String(search.tab))
    ? (search.tab as
        "overview" | "resources" | "dashboards" | "collector" | "activity")
    : undefined;
  return {
    ...parseDashboardTime(search),
    tab,
    kind: text("kind"),
    namespace_id: text("namespace_id", 128),
    namespace_name: text("namespace_name", 253),
    state_filter: text("state_filter", 32),
    name_filter: text("name_filter", 253),
    label_filter: text("label_filter"),
    page_cursor: text("page_cursor", 32768),
    page_index:
      Number.isInteger(Number(search.page_index)) &&
      Number(search.page_index) >= 1 &&
      Number(search.page_index) <= 100000
        ? Number(search.page_index)
        : undefined,
    resource_operation: text("resource_operation", 128),
    resource_type: text("resource_type"),
    resource_name: text("resource_name", 253),
    resource_uid: text("resource_uid", 128),
    resource_namespace: text("resource_namespace", 253),
    resource_namespace_id: text("resource_namespace_id", 128),
    full: search.full === true || search.full === "true",
  };
}
export function useResourceLocation(clusterId: string) {
  const navigate = useNavigate(),
    search = useSearch({ strict: false }) as KubernetesResourceSearch;
  const update = useCallback(
    (patch: Partial<KubernetesResourceSearch>, replace = true) =>
      void navigate({
        to: "/kubernetes/$clusterId",
        params: { clusterId },
        search: { ...search, ...patch },
        resetScroll: false,
        replace,
      }),
    [navigate, clusterId, search],
  );
  return { search, update };
}
