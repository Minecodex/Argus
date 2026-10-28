import { useObservationChartLink } from "./observation-chart-link";
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
import { layoutObservation } from "./observation-layout";
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
  const group = useObservationChartLink();
  const root = useRef<HTMLDivElement>(null);
  const { resolvedTheme } = useTheme();
  useEffect(() => {
    if (!root.current) return;
    registerCharts();
    const element = root.current;
    let chart: ReturnType<typeof echarts.init> | undefined;
    const styles = getComputedStyle(root.current);
    const color = (name: string) => styles.getPropertyValue(name).trim();
    const paint = () => {
      const size = (token: string) =>
        Number.parseFloat(color(token)) || Number.parseFloat(styles.fontSize);
      const arranged = layoutObservation(option, {
        width: root.current!.clientWidth,
        height: root.current!.clientHeight,
        labelWidth: size("--chart-label-max-width"),
        font: size("--text-help"),
        heading: size("--text-heading"),
        padding: size("--space-3"),
        footer: size("--chart-legend-space"),
      });
      chart!.setOption({
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
        grid: {
          left: size("--space-3"),
          right: size("--space-4"),
          top: size("--space-3"),
          bottom:
            option.legend &&
            (Array.isArray(option.legend)
              ? option.legend[0]?.show
              : option.legend.show) !== false
              ? size("--chart-legend-space")
              : size("--space-3"),
          containLabel: true,
        },
        ...themeObservation(arranged, color),
      });
    };
    const resize = () => {
      // Sandboxed cards and collapsed sections can mount while hidden. ECharts
      // graph coordinates require a real view rectangle before initialization.
      if (element.clientWidth <= 0 || element.clientHeight <= 0) {
        if (chart) {
          chart.dispatchAction({ type: "hideTip" });
          chart.group = "";
        }
        return;
      }
      if (!chart) {
        chart = echarts.init(element);
        chart.on("click", (event) =>
          onSelect?.(
            event.dataIndex ?? 0,
            event.seriesIndex ?? 0,
            event.dataType,
          ),
        );
      } else chart.resize();
      chart.group = group ?? "";
      if (group) echarts.connect(group);
      paint();
    };
    const observer = new ResizeObserver(resize);
    observer.observe(root.current);
    resize();
    return () => {
      observer.disconnect();
      chart?.dispose();
    };
  }, [option, label, resolvedTheme, onSelect, group]);
  return (
    <div
      className="argus-observation-chart"
      ref={root}
      role="img"
      aria-label={label}
    />
  );
}
