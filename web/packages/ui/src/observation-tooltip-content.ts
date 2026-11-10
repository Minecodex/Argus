import type {
  TooltipComponentFormatterCallbackParams,
  TooltipComponentOption,
} from "echarts";

type Item = Exclude<TooltipComponentFormatterCallbackParams, unknown[]>;
const datum = (value: unknown): unknown =>
  Array.isArray(value) ? value.at(-1) : value;

export function observationTooltipContent(
  params: TooltipComponentFormatterCallbackParams,
  document: Document,
  valueFormatter?: TooltipComponentOption["valueFormatter"],
): HTMLElement {
  const items: Item[] = Array.isArray(params) ? params : [params];
  const content = document.createElement("div");
  content.className = "argus-chart-tooltip__content";
  const head = document.createElement("div");
  head.className = "argus-chart-tooltip__heading";
  const first = items[0] as (Item & { axisValueLabel?: string }) | undefined;
  const time = document.createElement("strong");
  time.textContent = first?.axisValueLabel ?? "";
  const count = document.createElement("span");
  const english = document.documentElement.lang.startsWith("en");
  count.textContent = english
    ? `${items.length} series · Scroll to view`
    : `${items.length} 条序列 · 滚动查看`;
  head.append(time, count);
  const list = document.createElement("div");
  list.className = "argus-chart-tooltip__list";
  list.tabIndex = 0;
  list.setAttribute("role", "list");
  list.setAttribute(
    "aria-label",
    english ? "Values at this point" : "当前点的序列值",
  );
  for (const item of items) {
    const row = document.createElement("div");
    row.className = "argus-chart-tooltip__row";
    row.setAttribute("role", "listitem");
    const marker = document.createElement("i");
    marker.setAttribute("aria-hidden", "true");
    if (typeof item.color === "string")
      marker.style.setProperty("--argus-chart-tooltip-marker", item.color);
    const label = document.createElement("span");
    label.className = "argus-chart-tooltip__name";
    label.textContent =
      (item.seriesName && !item.seriesName.includes("\0")
        ? item.seriesName
        : item.name) || "—";
    label.title = label.textContent;
    const value = document.createElement("strong");
    value.className = "argus-chart-tooltip__value";
    const raw = datum(item.value);
    value.textContent =
      raw === null ||
      raw === undefined ||
      (typeof raw === "number" && !Number.isFinite(raw))
        ? "—"
        : valueFormatter && (typeof raw === "number" || typeof raw === "string")
          ? valueFormatter(raw, item.dataIndex)
          : typeof raw === "number"
            ? new Intl.NumberFormat(english ? "en-US" : "zh-CN", {
                maximumSignificantDigits: 6,
              }).format(raw)
            : String(raw);
    value.title = value.textContent;
    row.append(marker, label, value);
    list.append(row);
  }
  if (items.length > 1 || time.textContent) content.append(head);
  content.append(list);
  return content;
}
