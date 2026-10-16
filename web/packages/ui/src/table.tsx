import { Fragment, type ReactNode } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "./button";
import { useUiText } from "./locale";

export type Column<T> = {
  key: keyof T | string;
  header: ReactNode;
  render?: (item: T) => ReactNode;
  align?: "left" | "right" | "center";
  selection?: boolean;
  cellClassName?: string;
  noWrap?: boolean;
};

const alignment = <T,>(column: Column<T>) =>
  column.align ??
  (["actions", "action"].includes(String(column.key)) ? "right" : "left");

export function DataTable<T extends object>({
  columns,
  data,
  getRowKey,
  className,
  tableClassName,
  density = "default",
  surface = "default",
  rowDetail,
}: {
  columns: Column<T>[];
  data: T[];
  getRowKey: (item: T) => string;
  className?: string;
  tableClassName?: string;
  density?: "default" | "compact" | "dense";
  surface?: "default" | "plain" | "outlined";
  rowDetail?: (item: T) => ReactNode;
}) {
  return (
    <div
      className={[
        "argus-table-wrap",
        surface === "plain" && "argus-table-wrap--plain",
        surface === "outlined" && "argus-table-wrap--outlined",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      tabIndex={0}
    >
      <table
        className={[
          "argus-table",
          density !== "default" && "argus-table--compact",
          density === "dense" && "argus-table--dense",
          tableClassName,
        ]
          .filter(Boolean)
          .join(" ")}
      >
        <thead>
          <tr>
            {columns.map((column) => (
              <th
                className={[
                  alignment(column) === "right"
                    ? "is-right"
                    : alignment(column) === "center"
                      ? "is-center"
                      : "",
                  column.noWrap && "argus-table__cell--nowrap",
                ]
                  .filter(Boolean)
                  .join(" ")}
                key={String(column.key)}
              >
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((item) => (
            <Fragment key={getRowKey(item)}>
              <tr>
                {columns.map((column) => (
                  <td
                    className={[
                      alignment(column) === "right" && "is-right",
                      alignment(column) === "center" && "is-center",
                      column.selection && "argus-table__selection",
                      column.noWrap && "argus-table__cell--nowrap",
                      column.cellClassName,
                    ]
                      .filter(Boolean)
                      .join(" ")}
                    key={String(column.key)}
                  >
                    {column.render
                      ? column.render(item)
                      : String(item[column.key as keyof T] ?? "")}
                  </td>
                ))}
              </tr>
              {rowDetail?.(item) && (
                <tr className="argus-table__detail">
                  <td colSpan={columns.length}>{rowDetail(item)}</td>
                </tr>
              )}
            </Fragment>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function Pagination({
  page,
  totalPages,
  onChange,
}: {
  page: number;
  totalPages: number;
  onChange: (page: number) => void;
}) {
  const text = useUiText();
  return (
    <nav aria-label={text("分页", "Pagination")} className="argus-pagination">
      <span>
        {text(`第 ${page} / ${totalPages} 页`, `Page ${page} of ${totalPages}`)}
      </span>
      <div>
        <Button
          aria-label={text("上一页", "Previous page")}
          isDisabled={page <= 1}
          onPress={() => onChange(page - 1)}
          isIconOnly
          variant="ghost"
        >
          <ChevronLeft size={16} />
        </Button>
        <Button
          aria-label={text("下一页", "Next page")}
          isDisabled={page >= totalPages}
          onPress={() => onChange(page + 1)}
          isIconOnly
          variant="ghost"
        >
          <ChevronRight size={16} />
        </Button>
      </div>
    </nav>
  );
}
