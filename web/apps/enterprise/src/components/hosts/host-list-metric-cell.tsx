import { useTranslation } from "react-i18next";
import { MiniTrend, Tooltip, formatResourceQuantity } from "@argus/ui";
import { apiErrorPresentation, formatApiError } from "@argus/api-client";
import type { HostMetricGroup, HostMetricKind } from "./host-list-metrics";

export function HostListMetricCell({
  id,
  metric,
  group,
}: {
  id: string;
  metric: HostMetricKind;
  group: HostMetricGroup;
}) {
  const { t, i18n } = useTranslation();
  const testId = `host-metric-${metric}-${id}`;
  const status = (key: string, hint?: string) => (
    <Tooltip content={hint || t(`hostTrends.hints.${key}`)}>
      <span
        className="argus-resource-metric-status"
        tabIndex={0}
        data-testid={testId}
        data-metric-state={key}
      >
        {t(`hostTrends.states.${key}`)}
      </span>
    </Tooltip>
  );
  if (group.denied) return status("forbidden");
  if (group.error) {
    const error = apiErrorPresentation(group.error);
    if (error?.code === "QUERY_SOURCES_EMPTY") return status("not_collected");
    return status(
      error?.status === 403 ? "forbidden" : "query_failed",
      formatApiError(
        group.error,
        t("hostTrends.hints.query_failed"),
        (requestId) => t("common.requestReference", { requestId }),
      ),
    );
  }
  if (group.loading)
    return (
      <span
        className="argus-resource-metric-loading"
        aria-busy="true"
        aria-label={t("hostTrends.loading")}
        data-testid={testId}
      >
        {t("common.loading")}
      </span>
    );
  const trend = group.data?.[id];
  if (!trend || ["no_data", "partial"].includes(trend.state))
    return status(trend?.state ?? "no_data");
  const latest = trend.points.filter((point) => point.value != null).at(-1);
  if (!latest) return status("no_data");
  const format = (value: number | null) =>
    value == null
      ? "—"
      : metric === "cpu"
        ? `${new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(value * 100)}%`
        : formatResourceQuantity(String(value), "bytes");
  const percent = (value: number | null) =>
    value == null
      ? "—"
      : `${new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(value)}%`;
  const title = t(`hostTrends.${metric}`);
  const latestText =
    trend.basis === "capacity" ? percent(latest.value) : format(latest.used);
  return (
    <div
      className="argus-resource-metric-cell"
      data-testid={testId}
      data-metric-state={trend.state}
    >
      <MiniTrend
        points={trend.points}
        label={`${title} ${latestText}`}
        latest={
          <>
            {latestText}
            {trend.state === "stale" && (
              <span> · {t("hostTrends.states.stale")}</span>
            )}
          </>
        }
        reference={trend.basis === "capacity" ? 100 : undefined}
        renderDetail={(point) => (
          <div className="argus-resource-metric-tooltip">
            <strong>{title}</strong>
            <dl>
              <div>
                <dt>{t("hostTrends.time")}</dt>
                <dd>
                  {new Date(point.time).toLocaleTimeString(i18n.language)}
                </dd>
              </div>
              <div>
                <dt>{t("hostTrends.sampledAt")}</dt>
                <dd>
                  {point.sampledAt == null
                    ? "—"
                    : new Date(point.sampledAt * 1000).toLocaleTimeString(
                        i18n.language,
                      )}
                </dd>
              </div>
              <div>
                <dt>
                  {t(
                    metric === "cpu"
                      ? "hostTrends.utilization"
                      : "hostTrends.used",
                  )}
                </dt>
                <dd>{format(point.used)}</dd>
              </div>
              {trend.basis === "capacity" && metric !== "cpu" && (
                <>
                  <div>
                    <dt>{t("hostTrends.capacity")}</dt>
                    <dd>{format(point.capacity)}</dd>
                  </div>
                  <div>
                    <dt>{t("hostTrends.utilization")}</dt>
                    <dd>{percent(point.value)}</dd>
                  </div>
                </>
              )}
              {trend.filesystem && (
                <div>
                  <dt>{t("hostTrends.filesystem")}</dt>
                  <dd>{trend.filesystem}</dd>
                </div>
              )}
            </dl>
            <p>{t("hostTrends.window")}</p>
            {metric === "storage" && <p>{t("hostTrends.storageHint")}</p>}
            <p>{t("hostTrends.scopeHint")}</p>
          </div>
        )}
      />
    </div>
  );
}
