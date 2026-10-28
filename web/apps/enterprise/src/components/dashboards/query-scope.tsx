import { createContext, useContext, type ReactNode } from "react";
import {
  useApi,
  type DashboardSchemas,
  type DashboardSelection,
} from "@argus/api-client";
export type QueryCatalogLoader = (
  ...args: Parameters<ReturnType<typeof useApi>["dashboards"]["catalog"]>
) => Promise<
  Pick<
    DashboardSchemas["DashboardCatalogResult"],
    | "metrics"
    | "fields"
    | "values"
    | "has_more"
    | "next_cursor"
    | "complete"
    | "selected_exists"
  >
>;
export type DashboardQueryScope = {
  time: DashboardSchemas["DashboardTimeRange"];
  resources: string[];
  catalog?: boolean;
  key?: string;
  loadCatalog?: QueryCatalogLoader;
  variables?: Record<string, DashboardSelection>;
  locals?: Record<string, DashboardSelection>;
};
const Context = createContext<DashboardQueryScope | null>(null);
export function DashboardQueryScopeProvider({
  value,
  children,
}: {
  value: DashboardQueryScope;
  children: ReactNode;
}) {
  return <Context.Provider value={value}>{children}</Context.Provider>;
}
export function useDashboardQueryScope() {
  return useContext(Context);
}
export function queryScopeBounds(time: DashboardSchemas["DashboardTimeRange"]) {
  const to = time.kind === "absolute" ? new Date(time.to!) : new Date(),
    from =
      time.kind === "absolute"
        ? new Date(time.from!)
        : new Date(to.getTime() - (time.seconds ?? 3600) * 1000);
  return { from: from.toISOString(), to: to.toISOString() };
}

export function useQueryCatalog() {
  const scope = useDashboardQueryScope();
  const api = useApi();
  return scope?.loadCatalog ?? api.dashboards.catalog;
}
