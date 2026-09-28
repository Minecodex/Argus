import { useMemo, useState } from "react";
import { Button } from "./button";
import { useUiText } from "./locale";
import {
  decodedObservation,
  traceObservations,
  traceQuality,
} from "./trace-observations";

export function TraceWaterfall({
  envelope,
  onSelect,
}: {
  envelope: Record<string, unknown>;
  onSelect?: (row: Record<string, unknown>) => void;
}) {
  const text = useUiText();
  const nodes = useMemo(() => traceObservations(envelope), [envelope]);
  const [selected, setSelected] = useState<string>();
  const span = nodes.find((node) => node.key === selected)?.row;
  const valid = nodes.filter(
    (n) =>
      Number.isFinite(n.start) &&
      Number.isFinite(n.duration) &&
      n.duration >= 0,
  );
  const origin = Math.min(...valid.map((n) => n.start)),
    end = Math.max(...valid.map((n) => n.start + n.duration));
  const total = Math.max(end - origin, 0.001);
  const quality = traceQuality(envelope);
  return (
    <div className="argus-trace-view">
      <p className="argus-observation-caption" role="status">
        {text("已接收链路片段", "Received trace fragments")} ·{" "}
        {String(envelope.traceId ?? "")} ·{" "}
        {Object.entries(quality)
          .map(([k, v]) => `${k}: ${String(v)}`)
          .join(" · ")}
      </p>
      <div className="argus-observation-table">
        <table>
          <thead>
            <tr>
              <th>{text("服务 / 操作", "Service / operation")}</th>
              <th>{text("耗时（ms）", "Duration (ms)")}</th>
              <th>
                {text("相对时间", "Relative time")}{" "}
                {valid.length ? `0–${total.toFixed(2)} ms` : "—"}
              </th>
            </tr>
          </thead>
          <tbody>
            {nodes.map((node) => (
              <tr
                key={node.key}
                aria-selected={selected === node.key}
                data-source-id={String(node.row.sourceId ?? "")}
                data-resource-id={String(node.row.resourceId ?? "")}
              >
                <td>
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setSelected(node.key)}
                  >
                    <span
                      style={{
                        paddingInlineStart: `calc(var(--space-3) * ${Math.min(node.depth, 20)})`,
                      }}
                    >
                      {String(node.row.serviceName ?? "—")} /{" "}
                      {String(node.row.operationName ?? "—")}
                    </span>
                  </Button>
                  <small
                    className="argus-trace-origin"
                    title={`${text("来源", "Source")}: ${String(node.row.sourceId ?? "—")} · ${text("资源", "Resource")}: ${String(node.row.resourceId ?? "—")}`}
                  >
                    {text("来源", "Source")}: {String(node.row.sourceId ?? "—")}
                  </small>
                </td>
                <td>
                  {Number.isFinite(node.duration)
                    ? node.duration.toFixed(2)
                    : "—"}
                </td>
                <td className="argus-trace-timeline">
                  <button
                    type="button"
                    className="argus-trace-bar-track"
                    aria-label={`${String(node.row.operationName)} ${node.duration} ms`}
                    onClick={() => setSelected(node.key)}
                  >
                    {Number.isFinite(node.start) &&
                      Number.isFinite(node.duration) && (
                        <span
                          className={`argus-trace-bar ${String(node.row.status).toLowerCase() === "error" ? "argus-trace-bar--error" : ""}`}
                          style={{
                            marginInlineStart: `${Math.max(0, ((node.start - origin) / total) * 100)}%`,
                            width: `${Math.max(0, (node.duration / total) * 100)}%`,
                          }}
                        />
                      )}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {span && (
        <section
          className="argus-trace-span-details"
          aria-label={text("Span 详情", "Span details")}
        >
          <div className="argus-observation-pagination">
            <strong>{String(span.operationName)}</strong>
            {onSelect && (
              <Button size="sm" onClick={() => onSelect(span)}>
                {text("关联查询", "Related queries")}
              </Button>
            )}
          </div>
          <dl>
            {[
              "spanId",
              "parentSpanId",
              "sourceId",
              "resourceId",
              "scopeName",
              "kind",
              "status",
              "statusMessage",
              "traceState",
            ]
              .filter((k) => span[k] !== undefined)
              .map((k) => (
                <div key={k}>
                  <dt>{k}</dt>
                  <dd>{String(span[k]) || "—"}</dd>
                </div>
              ))}
          </dl>
          {[
            ["attributes", text("属性", "Attributes")],
            ["resourceAttributes", text("资源属性", "Resource attributes")],
            ["events", text("事件", "Events")],
            ["links", text("关联 Span", "Span links")],
          ].map(([key, label]) => (
            <details key={key} open>
              <summary>{label}</summary>
              <pre>
                {JSON.stringify(
                  decodedObservation(span[key!]) ?? null,
                  null,
                  2,
                )}
              </pre>
            </details>
          ))}
        </section>
      )}
      {!nodes.length && (
        <p className="argus-observation-empty">
          {text("当前范围没有 Span", "No spans in this range")}
        </p>
      )}
    </div>
  );
}
