import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { formatApiError, useApi } from "@argus/api-client";
export function DashboardBindingSummary({ id }: { id: string }) {
  const api = useApi(),
    { t } = useTranslation();
  const query = useQuery({
    queryKey: ["dashboard-bindings", "dashboard", id],
    queryFn: () => api.dashboards.bindings(id),
    refetchOnWindowFocus: true,
  });
  return (
    <details className="argus-dashboard-resource-links">
      <summary>{t("dashboards.boundResources")}</summary>
      <p className="argus-dashboard-muted">{t("dashboards.resourcesNote")}</p>
      {query.error && (
        <p role="alert">
          {formatApiError(query.error, t("dashboards.failed"), (requestId) =>
            t("common.requestReference", { requestId }),
          )}
        </p>
      )}
      <div className="argus-dashboard-inline">
        {query.data?.map((binding) =>
          binding.resource_type === "host" ? (
            <Link
              key={binding.id}
              className="argus-dashboard-shortcut"
              to="/hosts/$hostId"
              params={{ hostId: binding.resource_id }}
            >
              {binding.resource_name}
            </Link>
          ) : (
            <Link
              key={binding.id}
              className="argus-dashboard-shortcut"
              to="/kubernetes/$clusterId"
              params={{ clusterId: binding.resource_id }}
            >
              {binding.resource_name}
            </Link>
          ),
        )}
      </div>
      {query.data?.length === 0 && (
        <p className="argus-dashboard-muted">
          {t("dashboards.noBoundResources")}
        </p>
      )}
    </details>
  );
}
