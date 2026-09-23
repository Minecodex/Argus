// Generic DOM helpers. Business field selection and display decisions live in
// the immutable Tool template, not in this runtime or the portal.
export function text(value: unknown): string {
  if (value === undefined || value === null) return "—";
  return typeof value === "object" ? JSON.stringify(value) : String(value);
}
export function note(root: HTMLElement, value: string) {
  const p = document.createElement("p");
  p.textContent = value;
  root.append(p);
}
function empty(root: HTMLElement) {
  note(
    root,
    document.documentElement.lang === "zh-CN"
      ? "没有可显示的数据"
      : "No data to display",
  );
}
function truncated(root: HTMLElement, shown: number, total: number) {
  note(
    root,
    document.documentElement.lang === "zh-CN"
      ? `仅显示 ${total} 项中的前 ${shown} 项。请发送新消息缩小查询范围。`
      : `Showing the first ${shown} of ${total} items. Send another message to narrow the query.`,
  );
}
export function details(root: HTMLElement, value: unknown) {
  if (!value || typeof value !== "object") {
    note(root, text(value));
    return;
  }
  const entries = Object.entries(value);
  if (!entries.length) {
    empty(root);
    return;
  }
  const dl = document.createElement("dl");
  for (const [key, item] of entries.slice(0, 100)) {
    const term = document.createElement("dt"),
      description = document.createElement("dd");
    term.textContent = key;
    description.textContent = text(item);
    dl.append(term, description);
  }
  root.append(dl);
  if (entries.length > 100) truncated(root, 100, entries.length);
}
export function table(
  root: HTMLElement,
  rows: Record<string, unknown>[],
  fields: string[],
) {
  const valid = rows.filter((row) => row && typeof row === "object");
  if (!valid.length) {
    empty(root);
    return;
  }
  const values = valid.slice(0, 200);
  let columns = fields.filter((key) => values.some((row) => key in row));
  if (!columns.length)
    columns = Array.from(
      new Set(values.flatMap((row) => Object.keys(row))),
    ).slice(0, 8);
  const table = document.createElement("table"),
    head = document.createElement("thead"),
    row = document.createElement("tr"),
    body = document.createElement("tbody");
  for (const key of columns) {
    const cell = document.createElement("th");
    cell.scope = "col";
    cell.textContent = key;
    row.append(cell);
  }
  head.append(row);
  table.append(head, body);
  for (const item of values) {
    const row = document.createElement("tr");
    for (const key of columns) {
      const cell = document.createElement("td");
      cell.textContent = text(item[key]);
      row.append(cell);
    }
    body.append(row);
  }
  root.append(table);
  if (valid.length > values.length)
    truncated(root, values.length, valid.length);
}
export function timeseries(
  root: HTMLElement,
  series: Record<string, unknown>[],
  label: string,
) {
  const lines = series
    .slice(0, 20)
    .map((item) => {
      const values = Array.isArray(item.values)
        ? item.values
        : Array.isArray(item.value)
          ? [item.value]
          : [];
      return values
        .filter(Array.isArray)
        .map((value) => [Number(value[0]), Number(value[1])] as const)
        .filter(([x, y]) => Number.isFinite(x) && Number.isFinite(y));
    })
    .filter((line) => line.length > 0);
  if (!lines.length) {
    empty(root);
    return;
  }
  // A valid presentation can contain more points than the JavaScript argument
  // limit. Iterate instead of spreading the complete result into Math.min/max.
  let minX = Infinity,
    maxX = -Infinity,
    minY = Infinity,
    maxY = -Infinity;
  for (const line of lines)
    for (const [x, y] of line) {
      minX = Math.min(minX, x);
      maxX = Math.max(maxX, x);
      minY = Math.min(minY, y);
      maxY = Math.max(maxY, y);
    }
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 800 220");
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", label);
  for (const [index, line] of lines.entries()) {
    const polyline = document.createElementNS(svg.namespaceURI, "polyline");
    polyline.setAttribute(
      "points",
      line
        .map(
          ([x, y]) =>
            `${20 + ((x - minX) / (maxX - minX || 1)) * 760},${200 - ((y - minY) / (maxY - minY || 1)) * 180}`,
        )
        .join(" "),
    );
    polyline.setAttribute("fill", "none");
    polyline.setAttribute(
      "stroke",
      `var(${["--accent", "--info", "--success", "--warning"][index % 4]})`,
    );
    polyline.setAttribute("stroke-width", "2");
    svg.append(polyline);
  }
  root.append(svg);
  if (series.length > 20) truncated(root, 20, series.length);
  note(
    root,
    `${minY} – ${maxY} · ${new Date(minX * 1000).toISOString()} – ${new Date(maxX * 1000).toISOString()}`,
  );
}
