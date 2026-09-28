import { useMemo, useState } from "react";
import { Badge } from "./badge";
import { Button } from "./button";
import { Input } from "./form";
import { KeyValueGrid } from "./key-value-grid";
import { useUiText } from "./locale";

type LogResult = { row: Record<string, unknown>; target: string };
const valueText = (value: unknown) =>
  value === null || value === undefined
    ? "—"
    : typeof value === "object"
      ? JSON.stringify(value, null, 2)
      : String(value);

/** Presentation of returned log records. Search/fields never rewrite a query. */
export function ObservationLogs({
  rows,
  onSelect,
}: {
  rows: LogResult[];
  onSelect?: (row: Record<string, unknown>, target: string) => void;
}) {
  const text = useUiText();
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(0);
  const [fields, setFields] = useState(["service_name", "trace_id"]);
  const available = useMemo(
    () =>
      [...new Set(rows.flatMap(({ row }) => Object.keys(row)))]
        .filter((key) => !["body", "timestamp", "severity_text"].includes(key))
        .sort(),
    [rows],
  );
  const visible = useMemo(
    () =>
      search
        ? rows.filter(({ row }) =>
            JSON.stringify(row).toLowerCase().includes(search.toLowerCase()),
          )
        : rows,
    [rows, search],
  );
  const pages = Math.max(1, Math.ceil(visible.length / 50));
  const current = Math.min(page, pages - 1);
  return (
    <>
      <div className="argus-observation-log-controls">
        <Input
          aria-label={text("搜索当前结果", "Search current results")}
          placeholder={text("搜索当前结果", "Search current results")}
          value={search}
          onChange={(event) => {
            setSearch(event.target.value);
            setPage(0);
          }}
        />
        <details className="argus-observation-log-field-picker">
          <summary>{text("显示字段", "Visible fields")}</summary>
          <fieldset>
            <legend>
              {text(
                "选择记录摘要中的字段",
                "Choose fields for the record summary",
              )}
            </legend>
            {available.map((key) => (
              <label key={key}>
                <input
                  type="checkbox"
                  checked={fields.includes(key)}
                  onChange={(event) =>
                    setFields((previous) =>
                      event.target.checked
                        ? [...previous, key]
                        : previous.filter((value) => value !== key),
                    )
                  }
                />
                {key}
              </label>
            ))}
          </fieldset>
        </details>
      </div>
      <div
        className="argus-observation-logs"
        aria-label={text("日志结果", "Log results")}
        role="region"
      >
        {visible
          .slice(current * 50, (current + 1) * 50)
          .map(({ row, target }, index) => {
            const severity = String(row.severity_text ?? "");
            const key = JSON.stringify([
              target,
              row.resource_id,
              row.source_id,
              row.event_id,
              current,
              index,
            ]);
            return (
              <article
                key={key}
                className="argus-observation-log-entry"
                aria-label={`${text("日志记录", "Log entry")} ${current * 50 + index + 1}`}
              >
                <header>
                  <time>{valueText(row.timestamp)}</time>
                  {severity && (
                    <Badge
                      tone={
                        /ERROR|FATAL|CRITICAL/i.test(severity)
                          ? "danger"
                          : /WARN/i.test(severity)
                            ? "warning"
                            : "neutral"
                      }
                    >
                      {severity}
                    </Badge>
                  )}
                  {onSelect && (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => onSelect(row, target)}
                    >
                      {text("关联查询", "Related queries")}
                    </Button>
                  )}
                </header>
                <div className="argus-observation-log-meta">
                  {fields
                    .filter(
                      (field) =>
                        available.includes(field) &&
                        row[field] !== undefined &&
                        row[field] !== "",
                    )
                    .map((field) => (
                      <span key={field}>
                        <b>{field}</b>: <code>{valueText(row[field])}</code>
                      </span>
                    ))}
                </div>
                <pre className="argus-observation-log-body">
                  {valueText(row.body)}
                </pre>
                <details className="argus-observation-log-details">
                  <summary>
                    {text("完整记录与字段", "Full record and fields")}
                  </summary>
                  <KeyValueGrid
                    columns={1}
                    items={Object.entries(row).map(([label, value]) => ({
                      label,
                      value: <pre>{valueText(value)}</pre>,
                    }))}
                  />
                </details>
              </article>
            );
          })}
        {!visible.length && (
          <p className="argus-observation-empty">
            {text("当前范围没有数据", "No data in this range")}
          </p>
        )}
      </div>
      <div className="argus-observation-pagination">
        {pages > 1 && (
          <Button
            size="sm"
            disabled={current === 0}
            onClick={() => setPage(current - 1)}
          >
            {text("上一页", "Previous")}
          </Button>
        )}
        <span>
          {text("已返回记录", "Returned rows")}: {visible.length}
          {pages > 1 && ` · ${current + 1} / ${pages}`}
        </span>
        {pages > 1 && (
          <Button
            size="sm"
            disabled={current + 1 === pages}
            onClick={() => setPage(current + 1)}
          >
            {text("下一页", "Next")}
          </Button>
        )}
      </div>
    </>
  );
}
