/** Keep optional UI engines out of the shared React runtime and login bundle. */
export function webVendorChunk(rawId: string) {
  const id = rawId.replaceAll("\\", "/");
  // CSS is imported by the shared theme. Grouping it with an optional engine
  // would turn that engine into a static dependency of every portal.
  if (/\.(css|scss)(?:\?|$)/.test(id)) return;
  if (!id.includes("node_modules")) return;
  if (id.includes("/echarts/")) {
    if (id.endsWith("/customSeriesRegister.js")) return "vendor-echarts";
    if (
      /\/lib\/chart\/(bar|pie|gauge|scatter|heatmap|graph|custom|line)\//.test(
        id,
      )
    )
      return "vendor-echarts-charts";
    return "vendor-echarts";
  }
  if (id.includes("/zrender/")) return "vendor-zrender";
  if (/\/node_modules\/(react|react-dom|scheduler)\//.test(id))
    return "vendor-react";
  if (id.includes("/@tanstack/")) return "vendor-tanstack";

  // Split optional date/color/table engines from the primitives used at login.
  // A single accessibility chunk eagerly pulled every optional control in.
  if (/\/(?:@heroui|react-aria|react-aria-components|react-stately|@react-aria|@react-stately)\//.test(id)) {
    if (/(?:Calendar|DatePicker|DateField|DateInput|DateRange|TimeField|\/date(?:picker)?\/|\/calendar\/|\/components\/(?:calendar|date-|time-))/i.test(id)) return "vendor-calendar";
    if (/(?:Color|color-|color\/)/.test(id)) return "vendor-color";
    if (/(?:Table|Tree|GridList|\/table\/)/.test(id)) return "vendor-collections";
    return "vendor-controls";
  }

  if (id.includes("/lucide-react/")) return "vendor-icons";
  if (id.includes("/@xterm/")) return "vendor-terminal";
  if (id.includes("/@internationalized/date/"))
    return "vendor-dates";
  if (
    id.includes("/react-hook-form/") ||
    id.includes("/@hookform/") ||
    id.includes("/zod/")
  )
    return "vendor-forms";
  return;
}
