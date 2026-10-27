import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  ApiError,
  applicationAPMQueries,
  formatApiError,
  useApi,
  type MonitoringApplication,
  type MonitoringService,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Card,
  CardContent,
  CardHeader,
  EmptyState,
  Field,
  ObservationPanel,
  ObservationWarnings,
  SegmentedControl,
  Select,
  Spinner,
} from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { queryScopeBounds } from "../dashboards/query-scope";
import { ServicePicker } from "./pickers";
import {
  ApplicationTraceDetail,
  type ApplicationTraceSelection,
} from "./application-trace-detail";

const views = [
  "services",
  "instances",
  "endpoints",
  "red",
  "topology",
  "traces",
  "logs",
] as const;
type View = (typeof views)[number];
type SelectedService = Pick<
  MonitoringService,
  "id" | "service_name" | "service_namespace"
>;

export function ApplicationObservability({
  application,
}: {
  application: MonitoringApplication;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    canQuery = usePermission("application.read");
  const [view, setView] = useState<View>("services"),
    [seconds, setSeconds] = useState(3600);
  const [service, setService] = useState<SelectedService>(),
    [choosing, setChoosing] = useState(false);
  const [sourceId, setSourceId] = useState<string>(),
    [traceFilter, setTraceFilter] = useState<string>(),
    [operation, setOperation] = useState<string>();
  const [trace, setTrace] = useState<ApplicationTraceSelection>(),
    [approximate, setApproximate] = useState(false);
  const [range, setRange] = useState(() =>
    queryScopeBounds({ kind: "relative", seconds: 3600 }),
  );
  const clearDrill = () => {
    setSourceId(undefined);
    setTraceFilter(undefined);
    setOperation(undefined);
    setApproximate(false);
  };
  const query = useQuery({
    queryKey: [
      "application-observability",
      application.id,
      view,
      range,
      service?.id,
      view === "traces" ? [sourceId, traceFilter, operation] : null,
    ],
    enabled: canQuery,
    retry: false,
    queryFn: async () => {
      const common = {
        resource_ids: [application.id],
        time_range: range,
        budget: {
          max_rows: 5000,
          max_samples: 100000,
          max_scan_bytes: 268435456,
          max_series: 10000,
          max_result_bytes: 8388608,
          timeout_ms: 10000,
        },
      };
      if (view === "logs") {
        const result = await api.telemetry.queryLogs({
          ...common,
          query: service
            ? `resource_attributes.argus.service.id = ${JSON.stringify(service.id)}`
            : "*",
          pipeline: "limit 100",
        });
        return {
          data: result.data,
          partial: result.partial,
          meta: result.meta,
          type: "log_entries",
          warnings: result.warnings,
        };
      }
      const result = await api.telemetry.queryTraces({
        ...common,
        query: applicationAPMQueries[view],
        operation_name: "ApplicationAPM",
        variables: {
          serviceIds: service ? [service.id] : null,
          ...(view === "red"
            ? { bucketSeconds: Math.max(10, Math.ceil(seconds / 120)) }
            : {}),
          ...(view === "traces"
            ? {
                sourceId: sourceId ?? null,
                traceId: traceFilter ?? null,
                operationName: operation ?? null,
              }
            : {}),
        },
      });
      return {
        data: result.data,
        partial: result.extensions.argus.partial,
        meta: result.extensions.argus,
        type: view === "traces" ? "traces" : `apm_${view}`,
        warnings: result.errors?.map((error) => error.message) ?? [],
      };
    },
  });
  const noSources =
    query.error instanceof ApiError &&
    query.error.code === "QUERY_SOURCES_EMPTY";
  const selectObservation = (row: Record<string, unknown>) => {
    if (view === "traces") {
      if (
        row.resourceId === application.id &&
        typeof row.traceId === "string" &&
        typeof row.sourceId === "string"
      )
        setTrace({ traceId: row.traceId, sourceId: row.sourceId, range });
      return;
    }
    if (view === "logs") {
      setTraceFilter(
        typeof row.trace_id === "string" && row.trace_id
          ? row.trace_id
          : undefined,
      );
      setApproximate(!row.trace_id);
      setSourceId(undefined);
      setOperation(undefined);
      setView("traces");
      return;
    }
    if (
      row.resourceId !== application.id ||
      typeof row.serviceId !== "string" ||
      !row.serviceId
    )
      return;
    setService({
      id: row.serviceId,
      service_name: String(row.serviceName ?? ""),
      service_namespace: String(row.serviceNamespace ?? ""),
    });
    setSourceId(
      Array.isArray(row.sourceIds) && row.sourceIds.length > 1
        ? undefined
        : typeof row.sourceId === "string"
          ? row.sourceId
          : undefined,
    );
    setTraceFilter(undefined);
    setOperation(
      typeof row.operationName === "string" && row.operationName
        ? row.operationName
        : undefined,
    );
    setView("traces");
  };
  return (
    <section aria-label={t("applicationObservability.title")}>
      <Card>
        <CardHeader
          title={t("applicationObservability.title")}
          description={t("applicationObservability.basis")}
        />
        <CardContent>
          {!canQuery ? (
            <EmptyState
              title={t("monitoring.denied")}
              description={t("applicationObservability.denied")}
            />
          ) : (
            <>
              <div className="argus-monitoring-toolbar">
                <SegmentedControl
                  label={t("applicationObservability.view")}
                  value={view}
                  onChange={(value) => setView(value as View)}
                  options={views.map((value) => ({
                    value,
                    label: t(`applicationObservability.views.${value}`),
                  }))}
                />
                <Field
                  label={t("applicationObservability.range")}
                  requirement="required"
                >
                  <Select
                    value={String(seconds)}
                    onValueChange={(value) => {
                      setSeconds(Number(value));
                      setRange(
                        queryScopeBounds({
                          kind: "relative",
                          seconds: Number(value),
                        }),
                      );
                    }}
                    options={[900, 3600, 21600, 86400].map((value) => ({
                      value: String(value),
                      label: t(`applicationObservability.ranges.${value}`),
                    }))}
                  />
                </Field>
                <Button onPress={() => setChoosing(true)}>
                  {service
                    ? `${service.service_namespace ? service.service_namespace + " / " : ""}${service.service_name}`
                    : t("applicationObservability.allServices")}
                </Button>
                {service && (
                  <Button
                    onPress={() => {
                      setService(undefined);
                      clearDrill();
                    }}
                  >
                    {t("applicationObservability.clearService")}
                  </Button>
                )}
                <Button
                  isDisabled={query.isFetching}
                  onPress={() =>
                    setRange(queryScopeBounds({ kind: "relative", seconds }))
                  }
                >
                  {t("applicationObservability.refresh")}
                </Button>
              </div>
              {(sourceId || traceFilter || operation) && (
                <Button onPress={clearDrill}>
                  {t("applicationObservability.clearDrill")}
                </Button>
              )}
              {approximate && (
                <Alert
                  title={t("applicationObservability.approximate")}
                  description={t("applicationObservability.approximateHint")}
                />
              )}
              {query.isFetching && <Spinner label={t("common.loading")} />}
              {query.error && !noSources && (
                <Alert
                  tone="danger"
                  title={t("telemetry.queryFailed")}
                  description={formatApiError(
                    query.error,
                    t("monitoring.errors.unavailable"),
                    (id) => t("common.requestReference", { requestId: id }),
                  )}
                />
              )}
              {noSources && (
                <EmptyState
                  title={t("applicationObservability.noData")}
                  description={t("applicationObservability.noDataHint")}
                />
              )}
              <ObservationWarnings warnings={query.data?.warnings} />
              {query.data && (
                <ObservationPanel
                  title={t(`applicationObservability.views.${view}`)}
                  type={
                    view === "logs"
                      ? "logs"
                      : view === "traces"
                        ? "trace_list"
                        : `apm_${view}`
                  }
                  signal={view === "logs" ? "logs" : "traces"}
                  range={range}
                  onSelect={selectObservation}
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
              )}
            </>
          )}
          {choosing && (
            <ServicePicker
              application={application}
              onClose={() => setChoosing(false)}
              onSelect={(value) => {
                setService(value);
                clearDrill();
                setChoosing(false);
              }}
            />
          )}
          {trace && (
            <ApplicationTraceDetail
              applicationId={application.id}
              selected={trace}
              onClose={() => setTrace(undefined)}
            />
          )}
        </CardContent>
      </Card>
    </section>
  );
}
