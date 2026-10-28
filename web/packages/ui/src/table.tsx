import { Fragment, type ReactNode } from "react";
export { Pagination } from "./pagination";

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
