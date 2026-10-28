import { useTranslation } from "react-i18next";
import { Alert, Button, Field, Input, Select } from "@argus/ui";
import type { DashboardExecution, DashboardSelection } from "@argus/api-client";

export function ModelGovernanceControls({
  execution,
  variables,
  busy,
  onVariable,
  onRefresh,
}: {
  execution?: DashboardExecution;
  variables: Record<string, DashboardSelection>;
  busy: boolean;
  onVariable(name: string, value: DashboardSelection): void;
  onRefresh(): void;
}) {
  const { t, i18n } = useTranslation();
  const meta = execution?.model_governance;
  return (
    <div className="argus-settings-stack">
      <div className="argus-dashboard-inline argus-dashboard-inline--fields">
        <Field requirement="required" label={t("modelGovernance.month")}>
          <Input
            type="month"
            lang={i18n.language}
            value={variables.month?.values[0] ?? ""}
            onChange={(event) => {
              if (/^\d{4}-\d{2}$/.test(event.target.value))
                onVariable("month", {
                  all: false,
                  values: [event.target.value],
                });
            }}
          />
        </Field>
        <Field requirement="optional" label={t("modelGovernance.model")}>
          <Select
            value={
              variables.model?.all ? "" : (variables.model?.values[0] ?? "")
            }
            options={[
              { value: "", label: t("aiSettings.dashboard.allModels") },
              ...(meta?.models ?? []).map((m) => ({
                value: m.id,
                label: m.name,
              })),
            ]}
            onValueChange={(value) =>
              onVariable("model", { all: !value, values: value ? [value] : [] })
            }
          />
        </Field>
        <Button isDisabled={busy} onPress={onRefresh}>
          {t("dashboards.refresh")}
        </Button>
      </div>
      <p className="argus-settings-section__hint">
        {t("modelGovernance.basis", {
          timezone: meta?.timezone ?? "—",
          cutoff: meta?.projected_at
            ? new Date(meta.projected_at).toLocaleString(i18n.language, {
                timeZone: meta.timezone,
              })
            : "—",
        })}
      </p>
      {meta && meta.pending > 0 && (
        <Alert
          tone="warning"
          title={t("modelGovernance.delayed")}
          description={t("modelGovernance.pending", { count: meta.pending })}
        />
      )}
      {meta && !meta.usage_complete && (
        <Alert
          tone="warning"
          title={t("aiSettings.dashboard.usageIncompleteTitle")}
          description={t("aiSettings.dashboard.usageIncomplete", {
            input: meta.estimated_input,
            output: meta.estimated_output,
          })}
        />
      )}
      {meta?.cached_usage_complete === false && (
        <Alert
          tone="warning"
          title={t("modelGovernance.cacheIncomplete")}
          description={t("aiSettings.dashboard.cachedPartial")}
        />
      )}
      <p className="argus-settings-section__hint">
        {t("modelGovernance.unavailableMetrics")}
      </p>
      <p className="argus-settings-section__hint">
        {t("modelGovernance.quotaBasis")}
      </p>
    </div>
  );
}
