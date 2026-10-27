import { RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button, Field, Select, ValueSelector } from "@argus/ui";
import {
  useApi,
  type DashboardExecution,
  type DashboardSpec,
  type DashboardSchemas,
} from "@argus/api-client";
import { TimeRangePicker } from "./time-range-picker";
import { candidateContext, candidateRequest } from "./candidate-filters";
import type { ViewInput } from "./use-dashboard-execution";
import { DashboardScopeFilter } from "./dashboard-scope-filter";

export function DashboardControls({
  spec,
  input,
  time,
  bounds,
  execution,
  busy,
  refreshSeconds,
  onRefreshSeconds,
  onTime,
  onScope,
  onVariable,
  onRefresh,
}: {
  spec: DashboardSpec;
  input: ViewInput;
  time: DashboardSchemas["DashboardTimeRange"];
  bounds?: { from: string; to: string };
  execution?: DashboardExecution;
  busy: boolean;
  refreshSeconds: number;
  onRefreshSeconds: (seconds: number) => void;
  onTime: (time: DashboardSchemas["DashboardTimeRange"]) => void;
  onScope: (scope: Partial<ViewInput>) => void;
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
      <Field
        label={t("dashboards.time")}
        requirement="optional"
        controlMode="group"
      >
        <TimeRangePicker
          label={t("dashboards.time")}
          value={time}
          onChange={onTime}
        />
      </Field>
      <DashboardScopeFilter
        dimension="applications"
        mode={input.application_mode}
        ids={input.application_ids}
        disabled={busy}
        onChange={(mode, ids) =>
          onScope({
            application_mode: mode,
            application_ids: mode ? ids : undefined,
          })
        }
      />
      <Field label={t("dashboardControls.resourceType")} requirement="optional">
        <Select
          sizing="content"
          ariaLabel={t("dashboardControls.resourceType")}
          value={
            input.resource_types?.length === 1
              ? input.resource_types[0]!
              : "all"
          }
          options={[
            "all",
            "host",
            "kubernetes_cluster",
            "kubernetes_namespace",
            "application",
            "monitoring_target",
          ].map((value) => ({
            value,
            label: t(`dashboardControls.resourceKinds.${value}`),
          }))}
          onValueChange={(value) =>
            onScope({
              resource_types:
                value === "all"
                  ? undefined
                  : [value as NonNullable<ViewInput["resource_types"]>[number]],
              resource_mode: "all_authorized",
              resource_ids: [],
            })
          }
        />
      </Field>
      <DashboardScopeFilter
        dimension="resources"
        mode={
          input.resource_mode ??
          (input.resource_ids?.length ? "explicit" : "all_authorized")
        }
        ids={input.resource_ids}
        initialKind={
          input.resource_types?.[0] ??
          execution?.resources.find((row) =>
            input.resource_ids?.includes(row.id),
          )?.type
        }
        disabled={busy}
        onChange={(mode, ids) =>
          onScope({
            resource_mode: mode ?? "all_authorized",
            resource_ids: ids,
            ...(mode === "explicit" ? { resource_types: undefined } : {}),
          })
        }
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
              resource_mode: input.resource_mode,
              resource_types: input.resource_types,
              application_mode: input.application_mode,
              application_ids: input.application_ids,
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
                    resource_mode: input.resource_mode,
                    resource_types: input.resource_types,
                    application_mode: input.application_mode,
                    application_ids: input.application_ids,
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
        <Field
          label={t("dashboardControls.refreshInterval")}
          requirement="optional"
        >
          <Select
            sizing="content"
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
        </Field>
      </div>
    </div>
  );
}
