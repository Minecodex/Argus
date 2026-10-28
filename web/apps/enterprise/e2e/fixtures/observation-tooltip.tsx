import { useState } from "react";
import { createRoot } from "react-dom/client";
import { Button, Dialog, ThemeProvider, LocaleProvider } from "@argus/ui";
import { ObservationChart } from "../../../../packages/ui/src/observation-chart";
import { TelemetryTimeSeries } from "../../../../packages/ui/src/telemetry";
import type { EChartsOption } from "echarts";
import "@argus/ui/styles.css";
const theme =
  new URLSearchParams(location.search).get("theme") === "dark"
    ? "dark"
    : "light";
document.documentElement.dataset.theme = theme;
const option: EChartsOption = {
  tooltip: {
    trigger: "axis",
    renderMode: "richText",
    confine: true,
    position: [80, 250],
  },
  xAxis: { type: "category", data: ["A", "B", "C"] },
  yAxis: { type: "value", min: 0, max: 10 },
  series: [
    {
      type: "line",
      name: '<img src=x onerror="window.tooltipInjected=true">',
      data: [3, 5, 7],
    },
  ],
};
function Fixture() {
  const [open, setOpen] = useState(false);
  return (
    <LocaleProvider locale="en-US" persist={false}>
      <ThemeProvider theme={theme} persist={false}>
        <main className="argus-page">
          <Button onPress={() => setOpen(true)}>Open chart dialog</Button>
          <div
            data-testid="clipped-chart"
            style={{
              position: "relative",
              width: "var(--dialog-width-lg)",
              height: "var(--template-chart-height)",
              overflow: "hidden",
              border: "1px solid var(--border-default)",
            }}
          >
            <ObservationChart label="Shared chart" option={option} />
          </div>
          <div
            data-testid="occluder"
            style={{
              position: "absolute",
              top: "var(--space-px-100)",
              left: "var(--dialog-width-sm)",
              width: "var(--dialog-width-sm)",
              height: "var(--template-chart-height)",
              zIndex: "var(--layer-overlay)",
              background: "var(--bg-surface)",
              pointerEvents: "none",
            }}
          >
            Overlay
          </div>
          <TelemetryTimeSeries
            ariaLabel="Legacy chart"
            series={[
              {
                name: "Legacy series",
                points: [
                  { timestamp: "2026-10-28T00:00:00Z", value: 3 },
                  { timestamp: "2026-10-28T00:01:00Z", value: 5 },
                ],
              },
            ]}
          />
          <Dialog
            open={open}
            onOpenChange={setOpen}
            title="Chart dialog"
            size="lg"
          >
            <div style={{ height: "var(--template-chart-height)" }}>
              <ObservationChart label="Dialog chart" option={option} />
            </div>
          </Dialog>
        </main>
      </ThemeProvider>
    </LocaleProvider>
  );
}
createRoot(document.getElementById("root")!).render(<Fixture />);
