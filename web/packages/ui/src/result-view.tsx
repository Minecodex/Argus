import { useEffect, useId, useRef, useState } from "react";
import { DataTable, type Column } from "./table";
import { Button } from "./button";
import { FilterBar } from "./filter";
import { StatusBadge } from "./status-badge";
import { useUiText } from "./locale";
import { KeyValueGrid } from "./key-value-grid";
import { ChevronLeft, ChevronRight } from "lucide-react";

export type ResultRow = Record<string, unknown>;
export type ResultPagerProps = {
  offset: number;
  length: number;
  total: number;
  sourceTotal: number;
  onFirst?: () => void;
  onPrevious?: () => void;
  onNext?: () => void;
};
export function ResultPager({
  offset,
  length,
  total,
  sourceTotal,
  onFirst,
  onPrevious,
  onNext,
}: ResultPagerProps) {
  const text = useUiText();
  return (
    <nav
      className="argus-pagination"
      aria-label={text("结果分页", "Result pagination")}
    >
      <span>
        {length ? `${offset + 1}–${offset + length}` : "0"} ·{" "}
        {text("匹配", "Matched")} {total} / {text("已返回", "Returned")}{" "}
        {sourceTotal}
      </span>
      <div>
        {onFirst && (
          <Button size="sm" variant="ghost" onPress={onFirst}>
            {text("第一页", "First page")}
          </Button>
        )}
        {onPrevious && (
          <Button size="sm" variant="ghost" onPress={onPrevious}>
            <ChevronLeft size={14} aria-hidden />
            {text("上一页", "Previous page")}
          </Button>
        )}
        {onNext && (
          <Button size="sm" variant="ghost" onPress={onNext}>
            {text("下一页", "Next page")}
            <ChevronRight size={14} aria-hidden />
          </Button>
        )}
      </div>
    </nav>
  );
}
export type ResultColumn = {
  label: string;
  value: (row: ResultRow) => string;
  kind?: string;
};
export function ResultTable({
  rows,
  columns,
  detail,
}: {
  rows: ResultRow[];
  columns: ResultColumn[];
  detail?: (target: HTMLElement, row: ResultRow) => void;
}) {
  const text = useUiText();
  const [expanded, setExpanded] = useState<number | null>(null);
  const id = useId();
  const entries = rows.map((row, index) => ({ row, index }));
  const fields: Column<(typeof entries)[number]>[] = columns.map(
    (column, index) => ({
      key: `column-${index}`,
      header: column.label,
      render: ({ row, index: rowIndex }) => {
        const value = column.value(row);
        if (index === 0 && detail)
          return (
            <a
              href={`#${id}-${rowIndex}`}
              className="argus-result-name"
              onClick={(event) => {
                event.preventDefault();
                setExpanded(expanded === rowIndex ? null : rowIndex);
              }}
            >
              {value}
            </a>
          );
        if (column.kind === "status") {
          const tone =
            /^(active|connected|online|ready|healthy|normal|success|succeeded|resolved|converged)$/i.test(
              value,
            )
              ? "success"
              : /^(failed|error|critical|firing|disconnected|unavailable)$/i.test(
                    value,
                  )
                ? "danger"
                : "neutral";
          return <StatusBadge tone={tone}>{value}</StatusBadge>;
        }
        return (
          <span className="argus-result-cell" title={value}>
            {value}
          </span>
        );
      },
    }),
  );
  if (detail)
    fields.push({
      key: "actions",
      header: text("详情", "Details"),
      render: ({ index }) => (
        <Button
          size="sm"
          variant="ghost"
          aria-expanded={expanded === index}
          onPress={() => setExpanded(expanded === index ? null : index)}
        >
          {expanded === index
            ? text("收起", "Collapse")
            : text("展开", "Expand")}
        </Button>
      ),
    });
  return (
    <DataTable
      data={entries}
      columns={fields}
      getRowKey={({ index }) => String(index)}
      surface="plain"
      rowDetail={
        detail
          ? ({ row, index }) =>
              expanded === index ? (
                <div id={`${id}-${index}`}>
                  <NativeResultDetail row={row} render={detail} />
                </div>
              ) : null
          : undefined
      }
    />
  );
}

function NativeResultDetail({
  row,
  render,
}: {
  row: ResultRow;
  render(target: HTMLElement, row: ResultRow): void;
}) {
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (root.current) {
      root.current.replaceChildren();
      render(root.current, row);
    }
  }, [row, render]);
  return <div ref={root} className="argus-result-detail" />;
}

export function RecordFields({ fields }: { fields: [string, unknown][] }) {
  return (
    <KeyValueGrid
      columns={fields.length > 3 ? 2 : 1}
      items={fields.map(([label, value]) => ({
        label,
        value: <RecordValue value={value} />,
      }))}
    />
  );
}
function RecordValue({ value }: { value: unknown }) {
  const text = useUiText();
  if (value === null || value === undefined || value === "") return <>—</>;
  if (typeof value !== "object")
    return <span className="argus-result-value">{String(value)}</span>;
  if (
    Array.isArray(value) &&
    value.every((item) => item === null || typeof item !== "object")
  )
    return (
      <span className="argus-result-value">
        {value.map((item) => String(item ?? "—")).join(" · ") || "—"}
      </span>
    );
  const entries = Object.entries(value);
  return (
    <details className="argus-result-value-tree">
      <summary>
        {entries.length}{" "}
        {Array.isArray(value) ? text("项", "items") : text("个字段", "fields")}
      </summary>
      <dl>
        {entries.map(([key, item]) => (
          <div key={key}>
            <dt>{key}</dt>
            <dd>
              <RecordValue value={item} />
            </dd>
          </div>
        ))}
      </dl>
    </details>
  );
}

export type ResultFilterState = {
  search: string;
  filters: Record<string, string>;
  sort: string;
  direction: "asc" | "desc";
};
export function ResultFilters({
  value,
  facets,
  onChange,
  busy,
}: {
  value: ResultFilterState;
  facets: Record<string, string[]>;
  onChange(value: ResultFilterState): void;
  busy?: boolean;
}) {
  const text = useUiText();
  const labels: Record<string, string> = {
    status: text("状态", "Status"),
    connection_status: text("连接状态", "Connection status"),
    severity: text("严重程度", "Severity"),
    level: text("级别", "Level"),
    environment: text("环境", "Environment"),
    namespace: text("命名空间", "Namespace"),
    resource_type: text("资源类型", "Resource type"),
    service_name: text("服务", "Service"),
    signal: text("信号", "Signal"),
    type: text("类型", "Type"),
  };
  return (
    <section
      className="argus-result-filters"
      aria-label={text("在本次结果中筛选", "Filter this result")}
      aria-busy={busy}
    >
      <div className="argus-result-filters__caption">
        <span>{text("在本次结果中筛选", "Filter this result")}</span>
        <Button
          variant="ghost"
          size="sm"
          onPress={() =>
            onChange({ search: "", filters: {}, sort: "", direction: "asc" })
          }
        >
          {text("清空筛选", "Clear filters")}
        </Button>
      </div>
      <FilterBar
        search={{
          value: value.search,
          onChange: (search) => onChange({ ...value, search }),
          placeholder: text("搜索本次结果…", "Search this result…"),
        }}
        filters={[
          ...Object.entries(facets)
            .sort(([a], [b]) => a.localeCompare(b))
            .map(([key, values]) => ({
              key,
              value: value.filters[key] ?? "",
              allLabel: text("全部", "All") + " · " + (labels[key] ?? key),
              ariaLabel: labels[key] ?? key,
              options: values.map((item) => ({ value: item, label: item })),
              onChange: (item: string) =>
                onChange({
                  ...value,
                  filters: { ...value.filters, [key]: item },
                }),
            })),
          {
            key: "sort",
            value: value.sort,
            allLabel: text("原始顺序", "Original order"),
            ariaLabel: text("排序", "Sort"),
            options: [
              { value: "name", label: text("名称", "Name") },
              { value: "updated_at", label: text("更新时间", "Updated") },
              { value: "status", label: text("状态", "Status") },
            ],
            onChange: (sort) => onChange({ ...value, sort }),
          },
          ...(value.sort
            ? [
                {
                  key: "direction",
                  value: value.direction,
                  ariaLabel: text("排序方向", "Sort direction"),
                  options: [
                    { value: "asc", label: text("升序", "Ascending") },
                    { value: "desc", label: text("降序", "Descending") },
                  ],
                  onChange: (direction: string) =>
                    onChange({
                      ...value,
                      direction: direction as "asc" | "desc",
                    }),
                },
              ]
            : []),
        ]}
      />
    </section>
  );
}
