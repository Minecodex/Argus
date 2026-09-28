import { useEffect, useRef } from "react";
import {
  BarChart,
  CustomChart,
  GaugeChart,
  GraphChart,
  HeatmapChart,
  LineChart,
  PieChart,
  ScatterChart,
} from "echarts/charts";
import {
  AriaComponent,
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  MarkLineComponent,
  TooltipComponent,
  VisualMapComponent,
} from "echarts/components";
import * as echarts from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsOption } from "echarts";
import { useTheme } from "./theme";
import { themeObservation } from "./observation-theme";
let registered = false;
function registerCharts() {
  if (registered) return;
  echarts.use([
    BarChart,
    CustomChart,
    GaugeChart,
    GraphChart,
    HeatmapChart,
    LineChart,
    PieChart,
    ScatterChart,
    AriaComponent,
    DataZoomComponent,
    GridComponent,
    LegendComponent,
    MarkLineComponent,
    TooltipComponent,
    VisualMapComponent,
    CanvasRenderer,
  ]);
  registered = true;
}

export function ObservationChart({
  option,
  label,
  onSelect,
}: {
  option: EChartsOption;
  label: string;
  onSelect?: (index: number, series: number, kind?: string) => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const { resolvedTheme } = useTheme();
  useEffect(() => {
    if (!root.current) return;
    registerCharts();
    const chart = echarts.init(root.current);
    const styles = getComputedStyle(root.current);
    const color = (name: string) => styles.getPropertyValue(name).trim();
    chart.setOption({
      animation: false,
      color: ["--accent", "--info", "--success", "--warning", "--danger"].map(
        color,
      ),
      textStyle: {
        color: color("--text-secondary"),
        fontFamily: styles.fontFamily,
      },
      backgroundColor: "transparent",
      aria: { enabled: true, description: label },
      grid: { left: 48, right: 20, top: 30, bottom: 48, containLabel: true },
      ...themeObservation(option, color),
    });
    chart.on("click", (event) =>
      onSelect?.(event.dataIndex ?? 0, event.seriesIndex ?? 0, event.dataType),
    );
    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(root.current);
    return () => {
      observer.disconnect();
      chart.dispose();
    };
  }, [option, label, resolvedTheme, onSelect]);
  return (
    <div
      className="argus-observation-chart"
      ref={root}
      role="img"
      aria-label={label}
    />
  );
}
