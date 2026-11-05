import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ApiError,
  applicationAPMQueries,
  formatApiError,
  useApi,
} from "@argus/api-client";
import {
  Alert,
  Card,
  CardContent,
  CardHeader,
  EmptyState,
  ObservationPanel,
  ObservationWarnings,
  Pagination,
  Spinner,
} from "@argus/ui";

type View = "instances" | "endpoints" | "red" | "topology" | "traces" | "logs";
type Range = { from: string; to: string };
const tracePageSize = 50;

export function ApplicationObservationPanel({
  applicationId,
  range,
  serviceId,
  view,
  drill,
  page = 1,
  onPageChange,
  onSelect,
}: {
  applicationId: string;
  range: Range;
  serviceId?: string;
  view: View;
  drill?: { sourceId?: string; traceId?: string; operation?: string };
  page?: number;
  onPageChange?: (page: number) => void;
  onSelect?: (row: Record<string, unknown>) => void;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const query = useQuery({
    queryKey: [
      "application-observability",
      applicationId,
      view,
      range,
      serviceId,
      drill,
      page,
    ],
    retry: false,
    queryFn: async ({ signal }) => {
      const common = {
        resource_ids: [applicationId],
        time_range: range,
        budget: {
          max_rows: view === "topology" ? 50000 : 5000,
          max_samples: 100000,
          max_scan_bytes: 268435456,
          max_series: 10000,
          max_result_bytes: 8388608,
          timeout_ms: 10000,
        },
      };
      if (view === "logs") {
        const result = await api.telemetry.queryLogs(
          {
            ...common,
            query: serviceId
              ? `resource_attributes.argus.service.id = ${JSON.stringify(serviceId)}`
              : "*",
            pipeline: "limit 100",
          },
          signal,
        );
        return {
          data: result.data,
          partial: result.partial,
          meta: result.meta,
          type: "log_entries",
          warnings: result.warnings,
          total: 0,
          limited: false,
        };
      }
      const seconds = (Date.parse(range.to) - Date.parse(range.from)) / 1000;
      const result = await api.telemetry.queryTraces(
        {
          ...common,
          query: applicationAPMQueries[view],
          operation_name: "ApplicationAPM",
          variables: {
            serviceIds: serviceId ? [serviceId] : null,
            ...(view === "red"
              ? {
                  bucketSeconds: Math.max(10, Math.ceil(seconds / 60)),
                  rowLimit: 2400,
                }
              : {}),
            ...(view === "traces"
              ? {
                  pageNum: page,
                  pageSize: tracePageSize,
                  sourceId: drill?.sourceId ?? null,
                  traceId: drill?.traceId ?? null,
                  operationName: drill?.operation ?? null,
                }
              : {}),
          },
        },
        signal,
      );
      const data = result.data as {
        queryTraces?: { total?: number };
        queryAPMRED?: { limited?: boolean };
      };
      return {
        data: result.data,
        partial: result.extensions.argus.partial,
        meta: result.extensions.argus,
        type: view === "traces" ? "traces" : `apm_${view}`,
        warnings: result.errors?.map((error) => error.message) ?? [],
        total: data.queryTraces?.total ?? 0,
        limited: Boolean(data.queryAPMRED?.limited),
      };
    },
  });
  const code = query.error instanceof ApiError ? query.error.code : undefined;
  const title = t(`applicationObservability.views.${view}`);
  const chart = view === "red" || view === "topology";
  return (
    <Card className={chart ? "argus-application-chart" : undefined}>
      <CardHeader
        title={title}
        description={
          view === "red" ? t("applicationObservability.basis") : undefined
        }
      />
      <CardContent>
        {query.isFetching && <Spinner label={t("common.loading")} />}
        {code === "QUERY_SOURCES_EMPTY" ? (
          <EmptyState
            title={t("applicationObservability.noData")}
            description={t("applicationObservability.noDataHint")}
          />
        ) : (
          query.error && (
            <Alert
              tone="danger"
              title={t("telemetry.queryFailed")}
              description={
                code === "QUERY_BUDGET_EXCEEDED"
                  ? t("applicationObservability.budgetExceeded")
                  : formatApiError(
                      query.error,
                      t("monitoring.errors.unavailable"),
                      (id) => t("common.requestReference", { requestId: id }),
                    )
              }
            />
          )
        )}
        {query.data && (
          <>
            {query.data.limited && (
              <p role="status" className="argus-application-hint">
                {t("applicationObservability.trendLimited")}
              </p>
            )}
            <ObservationWarnings warnings={query.data.warnings} />
            <ObservationPanel
              title={title}
              type={
                view === "logs"
                  ? "logs"
                  : view === "traces"
                    ? "trace_list"
                    : `apm_${view}`
              }
              signal={view === "logs" ? "logs" : "traces"}
              range={range}
              onSelect={onSelect}
              targets={[
                {
                  id: "application",
                  status: query.data.partial ? "partial" : "success",
                  result_type: query.data.type,
                  data: query.data.data,
                  meta: query.data.meta,
                },
              ]}
            />
            {view === "traces" &&
              (query.data.total > tracePageSize || page > 1) && (
                <Pagination
                  page={page}
                  totalPages={Math.ceil(query.data.total / tracePageSize)}
                  isDisabled={query.isFetching}
                  onChange={onPageChange}
                />
              )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
