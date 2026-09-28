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
  if (id.includes("/@radix-ui/")) return "vendor-radix";
  if (id.includes("/lucide-react/")) return "vendor-icons";
  if (id.includes("/@xterm/")) return "vendor-terminal";
  if (id.includes("/react-datepicker/") || id.includes("/date-fns/"))
    return "vendor-dates";
  if (
    id.includes("/react-hook-form/") ||
    id.includes("/@hookform/") ||
    id.includes("/zod/")
  )
    return "vendor-forms";
  return "vendor";
}
