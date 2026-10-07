import { useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useApi, type DashboardResourceType } from "@argus/api-client";
import { usePermission } from "../../lib/permissions";

/** Entry provenance is independent of the viewer's changing execution scope. */
export function DashboardEntrySource({
  id,
  type,
}: {
  id: string;
  type?: DashboardResourceType;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const canReadHost = usePermission("host.read"),
    canReadCluster = usePermission("kubernetes.read");
  const knownType = useRef(type);
  if (type) knownType.current = type;
  const host = useQuery({
    queryKey: ["hosts", "detail", id],
    queryFn: () => api.hosts.get(id),
    enabled: knownType.current === "host" && canReadHost,
    retry: false,
  });
  const cluster = useQuery({
    queryKey: ["kubernetes", "clusters", id],
    queryFn: () => api.kubernetes.getCluster(id),
    enabled: knownType.current === "kubernetes_cluster" && canReadCluster,
    retry: false,
  });
  if (canReadHost && knownType.current === "host" && !host.error && host.data)
    return (
      <span className="argus-dashboard-entry-source">
        · {t("dashboardLinks.fromHost")}{" "}
        <Link
          to="/hosts/$hostId"
          params={{ hostId: id }}
          search={{ tab: "dashboards" }}
        >
          {host.data.name}
        </Link>
      </span>
    );
  if (
    canReadCluster &&
    knownType.current === "kubernetes_cluster" &&
    !cluster.error &&
    cluster.data
  )
    return (
      <span className="argus-dashboard-entry-source">
        · {t("dashboardLinks.fromCluster")}{" "}
        <Link
          to="/kubernetes/$clusterId"
          params={{ clusterId: id }}
          search={{ tab: "dashboards" }}
        >
          {cluster.data.name}
        </Link>
      </span>
    );
  return null;
}
