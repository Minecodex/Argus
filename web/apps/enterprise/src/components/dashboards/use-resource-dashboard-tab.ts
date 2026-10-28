import { useEffect, useState } from "react";

export function useResourceDashboardTab(
  id: string,
  requested: string | undefined,
  initial: string,
  allowed: boolean,
) {
  const [tab, setTab] = useState(() =>
    requested === "activity" ? "activity" : requested === "dashboards" && allowed ? "dashboards" : initial,
  );
  useEffect(() => {
    setTab(requested === "activity" ? "activity" : requested === "dashboards" && allowed ? "dashboards" : initial);
  }, [id, requested, initial, allowed]);
  return [tab, setTab] as const;
}
