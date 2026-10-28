import { Pagination } from "./pagination";
import { useMemo, useState } from "react";
import type { EChartsOption } from "echarts";
import { metricOption } from "./metric-options";
import { Button } from "./button";
import { Input } from "./form";
import { Select } from "./select";
import { useUiText } from "./locale";
import { ObservationChart } from "./observation-chart";
import { ObservationDetails } from "./observation-details";
import { ObservationLogs } from "./observation-logs";
import { TraceWaterfall } from "./trace-waterfall";
import { APMRedChart } from "./apm-red-chart";
import { apmColumns, type ObservationColumn } from "./apm-columns";
import {
  formatObservation,
  metricObservations,
  isMetricDimension,
  logAggregateObservations,
  observationEnvelope,
  observationEnvelopes,
  observationChartRows,
  observationRows,
  observationObject,
} from "./observation-data";
import {
  observationTone,
  reduceObservation,
  type ObservationDisplay,
  type ObservationThreshold,
} from "./observation-display";

export type ObservationTarget = {
  id: string;
  status: string;
  result_type: string;
  data: unknown;
  code?: string;
  meta?: Record<string, unknown>;
};
function ObservationStatValue({ value }: { value: string }) {
  const separator = value.indexOf(" ");
  if (separator < 0) return <>{value}</>;
  return (
    <>
      {value.slice(0, separator)} <small>{value.slice(separator + 1)}</small>
    </>
  );
}
function ObservationContent({
  type,
  signal,
  targets,
  title,
  unit = "",
  decimals = 2,
  legend = true,
  display,
  thresholds = [],
  range,
  onSelect,
}: {
  type: string;
  signal: string;
  targets: ObservationTarget[];
  title: string;
  unit?: string;
  decimals?: number;
  legend?: boolean;
  display?: ObservationDisplay;
  thresholds?: ObservationThreshold[];
  range?: { from: string; to: string };
  onSelect?: (row: Record<string, unknown>, target: string) => void;
}) {
  const text = useUiText();
  const series = useMemo(
    () =>
      targets.flatMap((target) =>
        (signal === "logs" && target.result_type !== "log_entries"
          ? logAggregateObservations(target.data)
          : metricObservations(target.data)
        ).map((s) => ({
          ...s,
          name:
            Object.keys(s.labels).every((key) => !isMetricDimension(key)) ||
            s.name === s.labels.__name__
              ? title
              : s.name,
          target: target.id,
        })),
      ),
    [targets, signal, title],
  );
  const rows = useMemo(
    () =>
      targets.flatMap((target) =>
        (type === "apm_red"
          ? observationChartRows(target.data)
          : observationRows(target.data)
        ).map((row) => ({ row, target: target.id })),
      ),
    [targets, type],
  );
  const envelope = targets
    .map((t) => observationEnvelope(t.data))
    .find(Boolean);
  const option = useMemo(
    () =>
      signal === "metrics" || signal === "logs"
        ? metricOption(
            type,
            series,
            legend,
            unit,
            decimals,
            display,
            thresholds,
            range,
          )
        : undefined,
    [signal, type, series, legend, unit, decimals, display, thresholds, range],
  );
  const negativeSlices =
    type === "pie"
      ? series.filter((s) => (reduceObservation(s, display?.reducer) ?? 0) < 0)
          .length
      : 0;
  if (!targets.length)
    return (
      <div className="argus-observation-empty">
        {text("尚未查询", "Not queried yet")}
      </div>
    );
  const error = targets.find(
    (t) => t.status === "error" || t.status === "skipped_budget",
  );
  if (
    error &&
    !targets.some((t) => t.status === "success" || t.status === "partial")
  )
    return (
      <div className="argus-observation-empty" role="status">
        {text("查询未完成", "Query incomplete")} · {error.code}
      </div>
    );
  if (
    signal === "metrics" ||
    (signal === "logs" && !["table", "logs"].includes(type))
  ) {
    const omitted = series.filter(
      (s) =>
        observationObject(s.row.argus_sample_preview)?.state ===
        "omitted_gap_budget",
    ).length;
    const curve = [
      "timeseries",
      "line",
      "area",
      "scatter",
      "state_timeline",
      "status_history",
      "heatmap",
    ].includes(type);
    if (curve && omitted > 0 && omitted === series.length)
      return (
        <div className="argus-observation-empty" role="status">
          {text(
            "曲线预览不可用，请查阅完整封存数据或缩小查询范围。",
            "Chart preview unavailable; inspect the sealed data or narrow the query.",
          )}
        </div>
      );
    if (
      !series.length ||
      series.every(
        (s) =>
          s.points.every((point) => point[1] === null) &&
          (curve ||
            (Number(observationObject(s.row.argus_statistics)?.count ?? 0) ===
              0 &&
              reduceObservation(s, display?.reducer) === null)),
      )
    )
      return (
        <div className="argus-observation-empty">
          {text("当前范围没有数据", "No data in this range")}
        </div>
      );
    if (type === "stat")
      return (
        <div
          className="argus-observation-stats"
          role="region"
          aria-label={`${title} · ${text("统计值", "Statistics")}`}
          tabIndex={0}
        >
          {series.map((s, i) => (
            <div className="argus-observation-stat" key={i}>
              {series.length > 1 && <span title={s.name}>{s.name}</span>}
              <strong
                data-tone={observationTone(
                  reduceObservation(s, display?.reducer),
                  thresholds,
                )}
              >
                <ObservationStatValue
                  value={formatObservation(
                    reduceObservation(s, display?.reducer),
                    unit,
                    decimals,
                  )}
                />
              </strong>
            </div>
          ))}
        </div>
      );
    if (type === "table")
      return (
        <ObservationTable
          rows={series.map((s) => ({
            row: {
              ...s.labels,
              value: formatObservation(
                reduceObservation(s, display?.reducer),
                unit,
                decimals,
              ),
            },
            target: s.target,
          }))}
          onSelect={onSelect}
        />
      );
    if (!option)
      return (
        <div className="argus-observation-empty">
          {text(
            "此图型需要带 le 边界且属于同一序列的有效累积直方图桶。",
            "This chart requires valid cumulative histogram buckets with le boundaries from one series.",
          )}
        </div>
      );
    return (
      <>
        {negativeSlices > 0 && (
          <p role="status" className="argus-observation-caption">
            {text(
              "饼图不支持负值，未绘制的序列数",
              "Pie charts exclude negative values; omitted series",
            )}
            : {negativeSlices}
          </p>
        )}
        <ObservationChart
          label={
            curve
              ? `${title} · ${series
                  .map((s) => {
                    const numbers = s.points.flatMap((p) =>
                      p[1] === null ? [] : [p[1]],
                    );
                    return `${s.name}: ${text("绘制样本", "Plotted samples")} ${s.points.length}, ${text("缺测标记", "Missing markers")} ${s.points.length - numbers.length}, ${text("最小 / 最大", "Min / max")} ${formatObservation(numbers.length ? numbers.reduce((a, b) => Math.min(a, b), Infinity) : null, unit, decimals)} / ${formatObservation(numbers.length ? numbers.reduce((a, b) => Math.max(a, b), -Infinity) : null, unit, decimals)}`;
                  })
                  .join("; ")}`
              : title
          }
          option={option}
          onSelect={
            onSelect
              ? (dataIndex, index) => {
                  const s = ["bar", "bar_gauge", "pie"].includes(type)
                    ? series.filter(
                        (s) =>
                          type !== "pie" ||
                          (reduceObservation(s, display?.reducer) !== null &&
                            reduceObservation(s, display?.reducer)! >= 0),
                      )[dataIndex]
                    : series[index];
                  if (s) onSelect(s.row, s.target);
                }
              : undefined
          }
        />
      </>
    );
  }
  if (type === "trace_detail" && envelope)
    return (
      <TraceWaterfall
        envelope={envelope}
        onSelect={onSelect ? (row) => onSelect(row, targets[0]!.id) : undefined}
      />
    );
  if (type === "apm_topology" && envelope) {
    const nodes = observationRows(envelope.nodes),
      edges = observationRows(envelope.edges);
    const graph: EChartsOption = {
      tooltip: { trigger: "item", renderMode: "richText" },
      series: [
        {
          type: "graph",
          layout: "circular",
          roam: true,
          symbolSize: 40,
          label: { show: true },
          data: nodes.map((n) => ({
            id: String(n.id),
            name: [n.serviceNamespace, n.serviceName]
              .filter(Boolean)
              .join(" / "),
            value: String(n.id),
          })),
          links: edges.map((e) => ({
            source: String(e.sourceNodeId),
            target: String(e.targetNodeId),
            value: Number(e.sampleCount),
          })),
          edgeSymbol: ["none", "arrow"],
        },
      ],
    };
    return (
      <ObservationChart
        label={title}
        option={graph}
        onSelect={
          onSelect
            ? (index, _series, kind) => {
                if (kind !== "edge" && nodes[index])
                  onSelect(nodes[index]!, targets[0]!.id);
              }
            : undefined
        }
      />
    );
  }
  if (type === "apm_red" && rows.length)
    return (
      <APMRedChart
        title={title}
        rows={rows}
        legend={legend}
        onSelect={onSelect}
      />
    );
  if (
    type === "apm_red" &&
    targets.some((target) =>
      observationEnvelopes(target.data).some(
        (item) =>
          Number(
            (item.chart_preview as Record<string, unknown> | undefined)
              ?.returned_rows,
          ) > 0,
      ),
    )
  )
    return (
      <p className="argus-observation-empty" role="status">
        {text(
          "曲线预览不可用；请查看完整封存表格，或缩小查询范围。",
          "Chart preview unavailable; inspect the sealed table or narrow the query.",
        )}
      </p>
    );
  if (signal === "logs" && type === "logs")
    return <ObservationLogs rows={rows} onSelect={onSelect} />;
  return (
    <ObservationTable
      rows={rows}
      onSelect={onSelect}
      searchable={signal === "logs"}
      columnDefinitions={apmColumns(type, text)}
    />
  );
}

export function ObservationTable({
  rows,
  onSelect,
  searchable,
  columnDefinitions,
}: {
  rows: Array<{ row: Record<string, unknown>; target: string }>;
  onSelect?: (row: Record<string, unknown>, target: string) => void;
  searchable?: boolean;
  columnDefinitions?: ObservationColumn[];
}) {
  const text = useUiText(),
    [search, setSearch] = useState(""),
    [page, setPage] = useState(0);
  const visible = search
    ? rows.filter(({ row }) =>
        JSON.stringify(row).toLowerCase().includes(search.toLowerCase()),
      )
    : rows;
  const preferred = [
    "timestamp",
    "severity_text",
    "body",
    "serviceName",
    "instanceId",
    "operationName",
    "sampleCount",
    "errorRate",
    "durationP95Ms",
    "traceId",
    "rootService",
    "rootOperation",
    "spanId",
    "parentSpanId",
    "startTime",
    "duration",
    "status",
    "spanCount",
    "value",
  ];
  const keys = Array.from(new Set(rows.flatMap(({ row }) => Object.keys(row))));
  const columns: ObservationColumn[] =
    columnDefinitions ??
    [
      ...preferred.filter((k) => keys.includes(k)),
      ...keys.filter((k) => !preferred.includes(k)),
    ].map((key) => ({ key, label: key }));
  const pages = Math.max(1, Math.ceil(visible.length / 50)),
    currentPage = Math.min(page, pages - 1);
  const display = (value: unknown) =>
    value === null || value === undefined
      ? "—"
      : typeof value === "object"
        ? JSON.stringify(value)
        : String(value);
  return (
    <>
      <div>
        {searchable && (
          <Input
            aria-label={text("搜索当前结果", "Search current results")}
            placeholder={text("搜索当前结果", "Search current results")}
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(0);
            }}
          />
        )}
      </div>
      <div
        className="argus-observation-table"
        tabIndex={0}
        role="region"
        aria-label={text("查询结果表格", "Query result table")}
      >
        <table>
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c.key} scope="col">
                  {c.label}
                </th>
              ))}
              {onSelect && <th scope="col">{text("详情", "Details")}</th>}
            </tr>
          </thead>
          <tbody>
            {visible
              .slice(currentPage * 50, (currentPage + 1) * 50)
              .map(({ row, target }, i) => (
                <tr key={i}>
                  {columns.map((c) => (
                    <td
                      key={c.key}
                      title={
                        c.format ? c.format(row[c.key]) : display(row[c.key])
                      }
                    >
                      {c.format ? c.format(row[c.key]) : display(row[c.key])}
                    </td>
                  ))}
                  {onSelect && (
                    <td>
                      <Button
                        size="sm"
                        variant="ghost"
                        onPress={() => onSelect(row, target)}
                      >
                        {text("打开详情", "Open details")}
                      </Button>
                    </td>
                  )}
                </tr>
              ))}
          </tbody>
        </table>
        {!visible.length && (
          <p className="argus-observation-empty">
            {text("当前范围没有数据", "No data in this range")}
          </p>
        )}
      </div>
      {visible.length > 50 && (
        <Pagination
          page={currentPage + 1}
          totalPages={pages}
          onChange={(page) => setPage(page - 1)}
        />
      )}
    </>
  );
}

export function ObservationPanel(
  props: Parameters<typeof ObservationContent>[0] & { details?: "external" },
) {
  const text = useUiText();
  const [selectedTarget, setSelectedTarget] = useState("");
  const separate =
    ["trace_detail", "apm_topology"].includes(props.type) &&
    props.targets.length > 1;
  const currentTarget =
    props.targets.find((t) => t.id === selectedTarget) ?? props.targets[0];
  return (
    <>
      {props.details !== "external" && (
        <div className="argus-observation-toolbar">
          <ObservationDetails {...props} />
        </div>
      )}
      {separate && (
        <Select
          ariaLabel={text("查询结果", "Query result")}
          value={currentTarget!.id}
          onValueChange={setSelectedTarget}
          options={props.targets.map((t) => ({ value: t.id, label: t.id }))}
        />
      )}
      <ObservationContent
        {...props}
        targets={separate ? [currentTarget!] : props.targets}
      />
    </>
  );
}
