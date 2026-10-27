import { useMemo, useState } from "react";
import { Select } from "./select";
import { useUiText } from "./locale";
import { ObservationChart } from "./observation-chart";
export function apmObservationGroups(
  rows: Array<{ row: Record<string, unknown>; target: string }>,
) {
  const groups = new Map<string, typeof rows>();
  for (const item of rows) {
    const key = JSON.stringify([
      item.target,
      item.row._presentation_root ?? "",
      ...[
        "sourceId",
        "resourceId",
        "serviceId",
        "serviceNamespace",
        "serviceName",
        "instanceId",
        "operationName",
      ].map((k) => item.row[k] ?? ""),
    ]);
    const group = groups.get(key) ?? [];
    group.push(item);
    groups.set(key, group);
  }
  return [...groups.values()].map((group) =>
    group.sort(
      (a, b) =>
        Date.parse(String(a.row.timestamp)) -
        Date.parse(String(b.row.timestamp)),
    ),
  );
}
export function APMRedChart({
  rows,
  title,
  legend,
  onSelect,
}: {
  rows: Array<{ row: Record<string, unknown>; target: string }>;
  title: string;
  legend: boolean;
  onSelect?: (row: Record<string, unknown>, target: string) => void;
}) {
  const text = useUiText(),
    [metric, setMetric] = useState(
      () =>
        ["samplesPerSecond", "errorRate", "durationP95Ms"].find((field) =>
          rows.some(
            ({ row }) =>
              row[field] != null && Number.isFinite(Number(row[field])),
          ),
        ) || "samplesPerSecond",
    );
  const groups = useMemo(() => apmObservationGroups(rows), [rows]);
  const times = rows
    .map(({ row }) => String(row.timestamp))
    .filter((value) => Number.isFinite(Date.parse(value)))
    .sort((a, b) => Date.parse(a) - Date.parse(b));
  const label =
    metric === "samplesPerSecond"
      ? text("样本速率", "Sample rate")
      : metric === "errorRate"
        ? text("样本错误率", "Sample error rate")
        : "P95";
  const unit =
    metric === "samplesPerSecond"
      ? text("样本/秒", "Samples/s")
      : metric === "errorRate"
        ? "%"
        : "ms";
  return (
    <>
      <Select
        sizing="content"
        density="compact"
        ariaLabel={text("RED 指标", "RED metric")}
        value={metric}
        onValueChange={setMetric}
        options={[
          { value: "samplesPerSecond", label: text("样本速率", "Sample rate") },
          {
            value: "errorRate",
            label: text("样本错误率", "Sample error rate"),
          },
          { value: "durationP95Ms", label: text("P95 时延", "P95 latency") },
        ]}
      />
      <ObservationChart
        label={`${title} · ${label}${times.length ? ` · ${text("曲线样本", "Chart samples")}: ${rows.length} · ${times[0]} – ${times.at(-1)}` : ""}`}
        option={{
          legend: { show: legend, type: "scroll" },
          xAxis: { type: "time" },
          yAxis: { type: "value", name: unit },
          series: groups.map((group) => ({
            type: "line",
            connectNulls: false,
            showSymbol:
              group.filter(
                ({ row }) =>
                  row[metric] != null && Number.isFinite(Number(row[metric])),
              ).length <= 1,
            name: [
              group[0]!.row._presentation_root,
              group[0]!.row.serviceNamespace,
              group[0]!.row.serviceName,
              group[0]!.row.instanceId,
              group[0]!.row.operationName,
              group[0]!.row.sourceId,
              group[0]!.row.resourceId,
            ]
              .filter(Boolean)
              .join(" · "),
            data: group.map(({ row }) => [
              String(row.timestamp),
              row[metric] == null
                ? null
                : Number(row[metric]) * (metric === "errorRate" ? 100 : 1),
            ]),
          })),
        }}
        onSelect={
          onSelect
            ? (point, series) => {
                const item = groups[series]?.[point];
                if (item) onSelect(item.row, item.target);
              }
            : undefined
        }
      />
    </>
  );
}
