import { useId, useState, type ComponentType } from "react";
import {
  Activity,
  BarChart3,
  ChartScatter,
  CircleGauge,
  Database,
  GitBranch,
  Layers,
  LayoutList,
  ListTree,
  Logs,
  PieChart,
  SquareActivity,
  Table2,
  Timer,
  TrendingUp,
} from "lucide-react";
import { Button } from "./button";
import { useUiText } from "./locale";

export type ChartTypeOption = {
  type: string;
  label: string;
  state: "recommended" | "compatible" | "unverified" | "incompatible";
  reason: string;
};
const icons: Record<
  string,
  ComponentType<{ "aria-hidden"?: boolean; size?: number }>
> = {
  timeseries: TrendingUp,
  stat: SquareActivity,
  gauge: CircleGauge,
  bar_gauge: LayoutList,
  bar: BarChart3,
  pie: PieChart,
  histogram: BarChart3,
  heatmap: Layers,
  state_timeline: Activity,
  scatter: ChartScatter,
  table: Table2,
  logs: Logs,
  trace_list: LayoutList,
  trace_detail: ListTree,
  apm_services: Database,
  apm_instances: Layers,
  apm_endpoints: GitBranch,
  apm_red: Timer,
  apm_topology: GitBranch,
};

/** Visualization selection changes presentation only; callers own query and capability checks. */
export function ChartTypeIcon({ type }: { type: string }) {
  const Icon = icons[type] ?? Activity;
  return <Icon aria-hidden size={24} />;
}

export function ChartTypePicker({
  value,
  options,
  onChange,
}: {
  value: string;
  options: ChartTypeOption[];
  onChange: (value: string) => void;
}) {
  const text = useUiText();
  const id = useId();
  const [expanded, setExpanded] = useState(false);
  const common = ["timeseries", "stat", "gauge", "bar"];
  const preferred = options.filter(
    (o) =>
      o.type === value || o.state === "recommended" || common.includes(o.type),
  );
  const compact = options.some((o) => common.includes(o.type))
    ? preferred
    : options;
  const visible = expanded ? options : compact;
  return (
    <div className="argus-chart-type-picker">
      <div className="argus-chart-type-picker__grid">
        {visible.map((option) => {
          const Icon = icons[option.type] ?? Activity;
          return (
            <Button
              key={option.type}
              layout="content"
              variant="ghost"
              className="argus-chart-type-picker__option"
              aria-label={option.label}
              aria-pressed={value === option.type}
              aria-describedby={`${id}-${option.type}`}
              isDisabled={option.state === "incompatible"}
              title={option.reason}
              onPress={() => onChange(option.type)}
            >
              <Icon aria-hidden size={24} />
              <span>{option.label}</span>
              {option.state === "recommended" && (
                <small>{text("推荐", "Suggested")}</small>
              )}
              <span className="sr-only" id={`${id}-${option.type}`}>
                {option.reason}
              </span>
            </Button>
          );
        })}
      </div>
      {compact.length < options.length && (
        <Button
          variant="ghost"
          className="argus-chart-type-picker__more"
          aria-expanded={expanded}
          onPress={() => setExpanded(!expanded)}
        >
          {expanded
            ? text("收起其他图型", "Show fewer charts")
            : text(
                `全部 ${options.length} 类图型`,
                `All ${options.length} chart types`,
              )}
        </Button>
      )}
    </div>
  );
}
