import { useQuery } from "@tanstack/react-query";
import { Play, RotateCcw } from "lucide-react";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ApiError,
  emptyDashboardSpec,
  formatApiError,
  useApi,
  type DashboardPanel,
  type DashboardTarget,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Card,
  CardContent,
  CardHeader,
  EmptyState,
  Field,
  Input,
  ObservationPanel,
  ObservationWarnings,
  SegmentedControl,
  Select,
  Spinner,
} from "@argus/ui";
import { QueryTargetEditor } from "../dashboards/query-target-editor";
import { newPanel, newTarget } from "../dashboards/model";
import {
  DashboardQueryScopeProvider,
  queryScopeBounds,
} from "../dashboards/query-scope";
import { usePermission } from "../../lib/permissions";
import "../../styles/dashboards.css";
import "../../styles/resource-query.css";

type Signal = "metrics" | "logs" | "traces";
type Result = {
  result_type: string;
  data: unknown;
  warnings?: string[];
  partial?: boolean;
  meta?: { scanned_bytes?: number; elapsed_ms?: number };
};
type Submitted = {
  target: DashboardTarget;
  range: { from: string; to: string };
  request: number;
  limit: number;
};
function initialPanel(
  signal: Signal,
  resourceType: "host" | "kubernetes_cluster",
): DashboardPanel {
  const type =
    signal === "metrics"
      ? "timeseries"
      : signal === "logs"
        ? "logs"
        : "trace_list";
  const target = newTarget(signal, type, "builder");
  if (target.source_definition.builder && signal !== "metrics")
    target.source_definition.builder.limit = 200;
  return {
    ...newPanel(emptyDashboardSpec(), ""),
    signal,
    type,
    targets: [target],
    applicable_resource_types: [resourceType],
    source_binding: {
      source_type:
        signal === "metrics"
          ? resourceType === "host"
            ? "hostmetrics"
            : "kubeletstats"
          : "otlp",
      capability_version: "v1",
    },
  };
}
/** In-memory resource query; no Dashboard draft, save or publication is created. */
export function ResourceTelemetry(props: {
  resourceId: string;
  signal: Signal;
  resourceType?: "host" | "kubernetes_cluster";
}) {
  return (
    <QuickResourceQuery
      key={`${props.resourceId}:${props.signal}`}
      {...props}
    />
  );
}
function QuickResourceQuery({
  resourceId,
  signal,
  resourceType = "host",
}: {
  resourceId: string;
  signal: Signal;
  resourceType?: "host" | "kubernetes_cluster";
}) {
  const { t, i18n } = useTranslation(),
    api = useApi();
  const canDiscover = usePermission("telemetry.dashboard.read");
  const [panel, setPanel] = useState(() => initialPanel(signal, resourceType));
  const [seconds, setSeconds] = useState(3600);
  const [limit, setLimit] = useState(200);
  const [submitted, setSubmitted] = useState<Submitted | null>(null);
  const [error, setError] = useState("");
  const [converting, setConverting] = useState(false);
  const [compiled, setCompiled] = useState("");
  const [display, setDisplay] = useState("auto");
  const revision = useRef(0),
    sequence = useRef(0);
  const time = { kind: "relative" as const, seconds };
  const spec = {
    ...emptyDashboardSpec(),
    default_time_range: time,
    panels: [panel],
  };
  const update = (next: DashboardPanel) => {
    revision.current += 1;
    setPanel(next);
    setError("");
  };
  const convert = async (mode: "builder" | "dsl") => {
    const result = await api.dashboards.convertPanel({ panel, mode });
    if (!result.converted) throw new Error(t("telemetry.conversionFailed"));
    return result.panel;
  };
  const switchMode = async (mode: string) => {
    if (mode === panel.authoring_mode || converting) return;
    const version = revision.current;
    setConverting(true);
    setError("");
    try {
      const next = await convert(mode as "builder" | "dsl");
      if (version === revision.current) update(next);
    } catch (e) {
      setError(formatApiError(e, t("telemetry.conversionFailed"), (id) => id));
    } finally {
      setConverting(false);
    }
  };
  const run = async () => {
    if (converting) return;
    const version = revision.current;
    setConverting(true);
    setError("");
    try {
      const executable =
        panel.authoring_mode === "builder" ? await convert("dsl") : panel;
      if (version !== revision.current) return;
      const target = structuredClone(executable.targets[0]!);
      setCompiled(
        [
          target.source_definition.dsl?.expression,
          target.source_definition.dsl?.pipeline,
        ]
          .filter(Boolean)
          .join("\n"),
      );
      setSubmitted({
        target,
        range: queryScopeBounds(time),
        request: ++sequence.current,
        limit,
      });
    } catch (e) {
      setError(
        formatApiError(e, t("telemetry.queryFailedDescription"), (id) => id),
      );
    } finally {
      setConverting(false);
    }
  };
  const query = useQuery({
    queryKey: ["resource-quick-query", resourceId, signal, submitted],
    enabled: submitted !== null,
    queryFn: async (): Promise<Result> => {
      const { target, range } = submitted!;
      const dsl = target.source_definition.dsl!;
      const common = {
        query: dsl.expression,
        resource_ids: [resourceId],
        time_range: range,
        budget: {
          max_scan_bytes: 268435456,
          max_rows: submitted!.limit,
          max_samples: 5_000_000,
          max_series: 100_000,
          timeout_ms: 10000,
          max_result_bytes: 8 * 1024 * 1024,
        },
      };
      if (signal === "metrics") {
        const response =
          target.query_mode === "instant"
            ? await api.telemetry.queryMetrics(common)
            : await api.telemetry.queryMetricsRange({
                ...common,
                step_seconds:
                  target.range_step_policy.kind === "fixed"
                    ? (target.range_step_policy.seconds ?? 60)
                    : Math.max(
                        target.range_step_policy.min_step_seconds ?? 1,
                        Math.ceil(
                          (new Date(range.to).getTime() -
                            new Date(range.from).getTime()) /
                            1000 /
                            (target.range_step_policy.target_points ?? 300),
                        ),
                      ),
              });
        return {
          result_type: response.data.resultType,
          data: response.data.result,
          warnings: response.warnings,
          partial: response.argus_meta.partial,
          meta: response.argus_meta,
        };
      }
      if (signal === "logs")
        return api.telemetry.queryLogs({ ...common, pipeline: dsl.pipeline });
      const response = await api.telemetry.queryTraces({
        ...common,
        operation_name: dsl.operation,
        variables: dsl.variables,
      });
      return {
        result_type: "traces",
        data: response.data,
        warnings: response.errors?.map((e) => e.message),
        partial: response.extensions.argus.partial,
        meta: response.extensions.argus,
      };
    },
    retry: (count, e) => e instanceof ApiError && e.retryable && count < 2,
    refetchOnWindowFocus: false,
  });
  const result = query.data;
  const pending = converting || query.isFetching;
  const type =
    display !== "auto"
      ? display
      : signal === "metrics"
        ? submitted?.target.query_mode === "instant"
          ? "stat"
          : "timeseries"
        : signal === "logs"
          ? result?.result_type === "log_entries"
            ? "logs"
            : "stat"
          : submitted?.target.source_definition.dsl?.expression.includes(
                "queryTrace(",
              )
            ? "trace_detail"
            : "trace_list";
  return (
    <div className="argus-resource-query">
      <Card>
        <CardHeader
          title={t("telemetry.quickQuery")}
          description={t("telemetry.quickQueryHint")}
        />
        <CardContent>
          <div className="argus-resource-query__toolbar">
            <Field label={t("telemetry.timeRange")} requirement="required">
              <Select
                value={String(seconds)}
                onValueChange={(v) => setSeconds(Number(v))}
                options={[15, 60, 360, 1440].map((minutes, i) => ({
                  value: String(minutes * 60),
                  label: t(
                    [
                      "telemetry.last15Minutes",
                      "telemetry.lastHourShort",
                      "telemetry.last6Hours",
                      "telemetry.last24Hours",
                    ][i]!,
                  ),
                }))}
              />
            </Field>
            <Field label={t("telemetry.limit")} requirement="required">
              <Input
                type="number"
                min={1}
                max={50000}
                value={limit}
                onChange={(e) =>
                  setLimit(
                    Math.max(1, Math.min(50000, Number(e.target.value) || 1)),
                  )
                }
              />
            </Field>
            <div className="argus-form-actions">
              <Button
                variant="primary"
                isPending={pending}
                onPress={() => void run()}
              >
                <Play aria-hidden />
                {t("telemetry.runQuery")}
              </Button>
              <Button
                variant="ghost"
                isDisabled={pending}
                onPress={() => {
                  update(initialPanel(signal, resourceType));
                  setSeconds(3600);
                  setLimit(200);
                  setSubmitted(null);
                  setCompiled("");
                  setDisplay("auto");
                }}
              >
                <RotateCcw aria-hidden />
                {t("telemetry.reset")}
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader
          title={t("telemetry.queryConfiguration")}
          action={
            <SegmentedControl
              label={t("telemetry.queryConfiguration")}
              value={panel.authoring_mode}
              disabled={converting}
              onChange={(mode) => void switchMode(mode)}
              options={[
                { value: "builder", label: t("telemetry.builder") },
                { value: "dsl", label: t("telemetry.dslEditor") },
              ]}
            />
          }
        />
        <CardContent>
          <DashboardQueryScopeProvider
            value={{ time, resources: [resourceId], catalog: canDiscover }}
          >
            <QueryTargetEditor
              panel={panel}
              spec={spec}
              target={panel.targets[0]!}
              transient
              onChange={(target) => update({ ...panel, targets: [target] })}
            />
          </DashboardQueryScopeProvider>
          {compiled && (
            <details className="argus-resource-query__compiled">
              <summary>{t("telemetry.executedDsl")}</summary>
              <pre>{compiled}</pre>
            </details>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader
          title={t(`telemetry.${signal}`)}
          action={
            <Select
              ariaLabel={t("telemetry.resultView")}
              value={display}
              onValueChange={setDisplay}
              options={(signal === "traces"
                ? ["auto", "table"]
                : [
                    "auto",
                    "table",
                    ...(signal === "metrics" ||
                    (result && result.result_type !== "log_entries")
                      ? ["timeseries", "stat", "bar"]
                      : []),
                  ]
              ).map((value) => ({
                value,
                label: t(`telemetry.views.${value}`),
              }))}
            />
          }
          description={
            submitted
              ? `${new Date(submitted.range.from).toLocaleString(i18n.language)} – ${new Date(submitted.range.to).toLocaleString(i18n.language)}`
              : t("telemetry.awaitingQuery")
          }
        />
        <CardContent>
          {(error || query.isError) && (
            <Alert
              title={t("telemetry.queryFailed")}
              tone="danger"
              description={
                error ||
                formatApiError(
                  query.error,
                  t("telemetry.queryFailedDescription"),
                  (id) => id,
                )
              }
            />
          )}
          {result?.partial && (
            <Alert
              title={t("telemetry.partial")}
              description={t("telemetry.partialDescription")}
              tone="warning"
            />
          )}
          <ObservationWarnings warnings={result?.warnings} />
          {pending && <Spinner label={t("common.loading")} />}
          {!submitted && !pending && (
            <EmptyState
              title={t("telemetry.awaitingQuery")}
              description={t("telemetry.quickQueryHint")}
            />
          )}
          {result && (
            <>
              <p className="argus-telemetry-query-meta">
                {t("telemetry.resultType")}: {result.result_type} ·{" "}
                {t("telemetry.elapsed")}: {result.meta?.elapsed_ms ?? 0} ms
              </p>
              <ObservationPanel
                title={t(`telemetry.${signal}`)}
                type={type}
                signal={signal}
                targets={[
                  {
                    id: "main",
                    status: result.partial ? "partial" : "success",
                    result_type: result.result_type,
                    data: result.data,
                  },
                ]}
              />
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
