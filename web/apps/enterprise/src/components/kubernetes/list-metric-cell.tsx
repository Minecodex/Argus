import { useTranslation } from "react-i18next";
import { MiniTrend, Tooltip, formatResourceQuantity } from "@argus/ui";
import type {
  KubernetesMetricPoint,
  KubernetesMetricResult,
  KubernetesMetricTrend,
} from "@argus/api-client";
import { apiErrorPresentation } from "@argus/api-client";
import { useInventoryError } from "./inventory-error";

type Group = {
  loading: boolean;
  error?: unknown;
  data?: KubernetesMetricResult;
};
type PlotPoint = KubernetesMetricPoint & { rawValue: number | null };
export function ListMetricCell({
  uid,
  metric,
  group,
}: {
  uid: string;
  metric: "cpu" | "memory" | "storage";
  group?: Group;
}) {
  const { t, i18n } = useTranslation(),
    formatError = useInventoryError();
  if (group?.error)
    return (
      <Tooltip content={formatError(group.error)}>
        <span
          className="argus-resource-metric-status"
          tabIndex={0}
          data-testid={`metric-${metric}-${uid}`}
        >
          {t(
            apiErrorPresentation(group.error)?.status === 403
              ? "kubernetesTrends.states.forbidden"
              : "kubernetesTrends.states.query_failed",
          )}
        </span>
      </Tooltip>
    );
  if (!group || group.loading)
    return (
      <span
        className="argus-resource-metric-loading"
        aria-busy="true"
        data-testid={`metric-${metric}-${uid}`}
        aria-label={t("kubernetesTrends.loading")}
      >
        {t("common.loading")}
      </span>
    );
  const entry = group.data?.items.find((item) => item.uid === uid);
  const format = (value: number | null | undefined, unit: string) => {
    if (value == null) return "—";
    if (unit === "cores" && Math.abs(value) > 0 && Math.abs(value) < 1)
      return (
        new Intl.NumberFormat(i18n.language, {
          maximumSignificantDigits: 3,
        }).format(value * 1000) + " mCPU"
      );
    return formatResourceQuantity(
      String(value),
      unit === "cores" ? "cpu" : "bytes",
    );
  };
  const percent = (value: number) =>
    new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 1 }).format(
      value,
    ) + "%";
  const render = (trend: KubernetesMetricTrend) => {
    const points: PlotPoint[] = trend.points.map((point) => {
      const denominator =
        trend.basis === "limit"
          ? point.limit
          : trend.basis === "request"
            ? point.request
            : trend.basis === "capacity"
              ? point.capacity
              : undefined;
      return {
        ...point,
        rawValue: point.value,
        value:
          trend.basis === "usage"
            ? point.value
            : point.value != null && denominator != null && denominator > 0
              ? (100 * point.value) / denominator
              : null,
      };
    });
    const latest = points.filter((point) => point.value != null).at(-1);
    if (!latest)
      return (
        <Tooltip
          key={trend.kind}
          content={t("kubernetesTrends.hints." + trend.state)}
        >
          <span className="argus-resource-metric-status" tabIndex={0}>
            {t("kubernetesTrends.states." + trend.state)}
          </span>
        </Tooltip>
      );
    const latestText =
      trend.basis === "usage"
        ? format(latest.rawValue, trend.unit)
        : percent(latest.value!);
    return (
      <div
        key={trend.kind}
        className="argus-resource-metric-trend"
        data-metric-state={trend.state}
      >
        <MiniTrend
          points={points}
          label={`${t("kubernetesTrends." + metric)} ${latestText}`}
          latest={
            <>
              {metric === "storage" && (
                <span className="argus-resource-metric-kind">
                  {t("kubernetesTrends.storageKinds." + trend.kind)}{" "}
                </span>
              )}
              {latestText}
              {trend.state === "stale" && (
                <span> · {t("kubernetesTrends.states.stale")}</span>
              )}
            </>
          }
          reference={trend.basis === "usage" ? undefined : 100}
          renderDetail={(point) => (
            <div className="argus-resource-metric-tooltip">
              <strong>{t("kubernetesTrends.kinds." + trend.kind)}</strong>
              <dl>
                <div>
                  <dt>{t("kubernetesTrends.time")}</dt>
                  <dd>
                    {new Date(point.time).toLocaleTimeString(i18n.language)}
                  </dd>
                </div>
                <div>
                  <dt>{t("kubernetesTrends.used")}</dt>
                  <dd>{format(point.rawValue, trend.unit)}</dd>
                </div>
                {trend.basis !== "usage" && (
                  <div>
                    <dt>{t("kubernetesTrends.basis." + trend.basis)}</dt>
                    <dd>{point.value == null ? "—" : percent(point.value)}</dd>
                  </div>
                )}
                {point.request != null && (
                  <div>
                    <dt>{t("kubernetesTrends.request")}</dt>
                    <dd>{format(point.request, trend.unit)}</dd>
                  </div>
                )}
                {point.limit != null && (
                  <div>
                    <dt>{t("kubernetesTrends.limit")}</dt>
                    <dd>{format(point.limit, trend.unit)}</dd>
                  </div>
                )}
                {point.capacity != null && (
                  <div>
                    <dt>{t("kubernetesTrends.capacity")}</dt>
                    <dd>{format(point.capacity, trend.unit)}</dd>
                  </div>
                )}
                {trend.current_request != null && (
                  <div>
                    <dt>{t("kubernetesTrends.currentRequest")}</dt>
                    <dd>{format(trend.current_request, trend.unit)}</dd>
                  </div>
                )}
                {trend.current_limit != null && (
                  <div>
                    <dt>{t("kubernetesTrends.currentLimit")}</dt>
                    <dd>{format(trend.current_limit, trend.unit)}</dd>
                  </div>
                )}
                {trend.allocatable != null && (
                  <div>
                    <dt>{t("kubernetesTrends.allocatable")}</dt>
                    <dd>{format(trend.allocatable, trend.unit)}</dd>
                  </div>
                )}
              </dl>
              {trend.partial_configuration && (
                <p>{t("kubernetesTrends.partialConfiguration")}</p>
              )}
              <p>{t("kubernetesTrends.window")}</p>
              {trend.basis === "request" && (
                <p>{t("kubernetesTrends.requestHint")}</p>
              )}
              {trend.state !== "ready" && (
                <p>{t("kubernetesTrends.hints." + trend.state)}</p>
              )}
            </div>
          )}
        />
      </div>
    );
  };
  return (
    <div
      className="argus-resource-metric-cell"
      data-testid={`metric-${metric}-${uid}`}
    >
      {entry?.trends.length ? (
        entry.trends.map(render)
      ) : (
        <span className="argus-resource-metric-status">
          {t("kubernetesTrends.states.no_data")}
        </span>
      )}
    </div>
  );
}
