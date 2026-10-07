import { createContext, useContext, type ReactNode } from "react";
import type { DashboardSchemas } from "@argus/api-client";
export type DashboardQueryScope = {
  time: DashboardSchemas["DashboardTimeRange"];
  resources: string[];
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
