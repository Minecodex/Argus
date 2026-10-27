import { useUiText } from "./locale";
import { InfoPopover } from "./info-popover";
import { ObservationFreshnessContent } from "./observation-freshness";
import { ObservationWarnings } from "./observation-warnings";
import {
  metricObservations,
  observationEnvelopes,
  observationObject,
} from "./observation-data";
import type { ObservationDisplay } from "./observation-display";
import type { ObservationTarget } from "./observation-panel";

export type ObservationDetailsProps = {
  title: string;
  targets: ObservationTarget[];
  signal: string;
  display?: ObservationDisplay;
};
export function ObservationDetails(props: ObservationDetailsProps) {
  const text = useUiText();
  const notes = props.targets.flatMap((target) => {
    return observationEnvelopes(target.data).flatMap((envelope) => {
      const coverage =
        envelope?.coverage && typeof envelope.coverage === "object"
          ? Object.entries(envelope.coverage)
          : [];
      const missing = coverage.filter(
        ([key, value]) =>
          /missing|ambiguous|unknown|cyclic|excluded/i.test(key) &&
          Number(value) > 0,
      );
      const preview = envelope.chart_preview as
        Record<string, unknown> | undefined;
      return [
        ...(["success", "no_data"].includes(target.status)
          ? []
          : [`${target.id}: ${target.status} ${target.code ?? ""}`]),
        ...(envelope?.limited
          ? [
              text(
                "查询定义显式限制了结果数量",
                "The query explicitly limits the result count",
              ),
            ]
          : []),
        ...(envelope?.status && envelope.status !== "available"
          ? [String(envelope.status)]
          : []),
        ...missing.map(([k, v]) => `${k}: ${String(v)}`),
        ...(preview?.sampled
          ? [
              `${text("已展示序列按全时间窗口采样", "Shown series sample the full time window")}: ${preview.shown_rows}/${preview.returned_rows} · ${text("序列", "Series")}: ${preview.shown_series}/${preview.returned_series}`,
            ]
          : []),
        ...(Number(preview?.unrenderable_rows) > 0
          ? [
              text(
                "部分记录缺少序列身份或时间，未绘制。",
                "Some records lack series identity or time and were not plotted.",
              ),
            ]
          : []),
        ...(Number(preview?.omitted_series) > 0
          ? [
              text(
                "部分序列未绘制，请缩小范围或查阅完整封存数据。",
                "Some series were not plotted; narrow the scope or inspect the sealed data.",
              ),
            ]
          : []),
      ];
    });
  });
  if (props.signal === "metrics") {
    const previews = props.targets
      .flatMap((target) => metricObservations(target.data))
      .map((s) => observationObject(s.row.argus_sample_preview))
      .filter((p) => p !== undefined);
    const omitted = previews.filter(
      (p) => p.state === "omitted_gap_budget",
    ).length;
    if (previews.some((p) => p.state === "sampled"))
      notes.push(
        text(
          "曲线为全窗口采样预览，保留端点、极值和缺测边界；统计值基于完整封存样本。",
          "Curves preview the full window with endpoints, extrema and missing boundaries preserved; statistics use the sealed samples.",
        ),
      );
    if (omitted > 0)
      notes.push(
        `${text(
          "缺测边界超过预览点数预算，未绘制序列；请查阅完整封存数据或缩小查询范围",
          "Missing boundaries exceed the preview point budget; inspect the sealed data or narrow the query. Omitted series",
        )}: ${omitted}`,
      );
  }
  const warnings = props.targets.flatMap((target) =>
    Array.isArray(target.meta?.warnings)
      ? target.meta.warnings.filter((v): v is string => typeof v === "string")
      : [],
  );
  return (
    <InfoPopover
      title={text("查询说明", "Query information")}
      label={`${props.title} · ${text("查询说明", "Query information")}`}
      attention={warnings.length > 0 || notes.length > 0}
    >
      {props.display?.reducer && props.display.reducer !== "last" && (
        <p className="argus-observation-caption">
          {text("返回样本的显示计算", "Display reduction of returned samples")}:{" "}
          {props.display.reducer}
          {props.display.reducer === "sum" &&
            ` · ${text("样本求和，不代表计数器增量", "Sample sum, not counter increase")}`}
          {props.display.reducer === "mean" &&
            ` · ${text("按样本平均，未按时长加权", "Sample mean, not time weighted")}`}
        </p>
      )}
      <ObservationWarnings warnings={warnings} />
      {notes.length > 0 && <p>{notes.join(" · ")}</p>}
      <ObservationFreshnessContent targets={props.targets} />
    </InfoPopover>
  );
}
