import { useEffect, useState } from "react";
export function useResourceDashboardTab(
  id: string,
  requested: string | undefined,
  initial: string,
  allowed: boolean,
  tabs?: string[],
) {
  const chosen = tabs
    ? requested && tabs.includes(requested)
      ? requested
      : initial
    : requested === "activity"
      ? "activity"
      : requested === "dashboards" && allowed
        ? "dashboards"
        : initial;
  const [tab, setTab] = useState(chosen);
  useEffect(() => setTab(chosen), [id, chosen]);
  return [tab, setTab] as const;
}
