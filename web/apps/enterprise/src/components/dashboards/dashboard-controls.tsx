import { RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button, Select, ValueSelector } from "@argus/ui";
import {
  useApi,
  type DashboardExecution,
  type DashboardSpec,
  type DashboardSchemas,
} from "@argus/api-client";
import { TimeRangePicker } from "./time-range-picker";
import { candidateContext, candidateRequest } from "./candidate-filters";
import type { ViewInput } from "./use-dashboard-execution";

export function DashboardControls({
  spec,
  input,
  time,
  bounds,
  execution,
  resources,
  busy,
  refreshSeconds,
  onRefreshSeconds,
  onTime,
  onResources,
  onVariable,
  onRefresh,
}: {
  spec: DashboardSpec;
  input: ViewInput;
  time: DashboardSchemas["DashboardTimeRange"];
  bounds?: { from: string; to: string };
  execution?: DashboardExecution;
  resources: { id: string; label: string }[];
  busy: boolean;
  refreshSeconds: number;
  onRefreshSeconds: (seconds: number) => void;
  onTime: (time: DashboardSchemas["DashboardTimeRange"]) => void;
  onResources: (ids: string[]) => void;
  onVariable: (
    name: string,
    value: DashboardSchemas["DashboardSelection"],
  ) => void;
  onRefresh: () => void;
}) {
  const { t } = useTranslation(),
    api = useApi();
  const intervals = [...new Set([0, 5, 10, 30, 60, 300, refreshSeconds])].sort(
    (a, b) => a - b,
  );
  return (
    <div
      className="argus-dashboard-controls"
      role="group"
      aria-label={t("dashboardControls.viewConditions")}
    >
      <TimeRangePicker
        label={t("dashboards.time")}
        value={time}
        onChange={onTime}
      />
      <ValueSelector
        presentation="popover"
        label={t("dashboards.resources")}
        title={t("dashboards.chooseResources")}
        hint={t("dashboardControls.resourceHint")}
        value={{
          all: !input.resource_ids?.length,
          values: input.resource_ids ?? [],
        }}
        multiple
        candidates={resources.map((r) => r.id)}
        valueLabels={Object.fromEntries(resources.map((r) => [r.id, r.label]))}
        onChange={(value) => onResources(value.all ? [] : value.values)}
      />
      {spec.variables.map((variable) => {
        const selection = input.variables?.[variable.name] ?? variable.default;
        return (
          <ValueSelector
            key={variable.id}
            presentation="popover"
            label={variable.label}
            disabled={busy}
            contextKey={candidateContext(variable.query, {
              time: bounds,
              resource_ids: input.resource_ids,
              variables: input.variables,
            })}
            value={selection}
            multiple={variable.multiple}
            candidates={
              execution?.variable_candidates[variable.name]?.values ?? []
            }
            onChange={(value) => onVariable(variable.name, value)}
            fetchValues={async (search, cursor, signal) =>
              api.dashboards.catalog(
                candidateRequest(
                  variable.query,
                  {
                    ...bounds!,
                    resource_ids: input.resource_ids,
                    variables: input.variables,
                  },
                  selection,
                  search,
                  cursor,
                ),
                signal,
              )
            }
          />
        );
      })}
      <div className="argus-dashboard-refresh-controls">
        <Button
          aria-label={t("dashboards.refresh")}
          aria-busy={busy}
          onPress={onRefresh}
        >
          <RefreshCw aria-hidden />
          {t(busy ? "dashboards.refreshing" : "dashboards.refresh")}
        </Button>
        <Select
          ariaLabel={t("dashboardControls.refreshInterval")}
          value={String(refreshSeconds)}
          options={intervals.map((seconds) => ({
            value: String(seconds),
            label:
              seconds === 0
                ? t("dashboardControls.refreshOff")
                : t("dashboardControls.refreshEvery", { count: seconds }),
          }))}
          onValueChange={(value) => onRefreshSeconds(Number(value))}
        />
      </div>
    </div>
  );
}
