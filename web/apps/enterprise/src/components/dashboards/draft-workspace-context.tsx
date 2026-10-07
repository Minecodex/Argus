import { createContext, useContext } from "react";
import type { useDashboardDraft } from "./use-dashboard-draft";
export const DashboardDraftContext = createContext<ReturnType<
  typeof useDashboardDraft
> | null>(null);
export function useDashboardDraftWorkspace() {
  const value = useContext(DashboardDraftContext);
  if (!value) throw new Error("Dashboard draft workspace is unavailable");
  return value;
}
