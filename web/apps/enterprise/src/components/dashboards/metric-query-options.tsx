import { Field, Input, Select } from "@argus/ui";
import type { DashboardTarget } from "@argus/api-client";
import { useTranslation } from "react-i18next";

/** Execution controls stay visible; aggregation windows belong to the builder. */
export function MetricQueryOptions({
  target,
  onChange,
}: {
  target: DashboardTarget;
  onChange: (patch: Partial<DashboardTarget>) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="argus-metric-query-options"
      role="group"
      aria-label={t("telemetry.queryOptions")}
    >
      <Field label={t("dashboards.queryMode")} requirement="optional">
        <Select
          sizing="content"
          value={target.query_mode}
          options={[
            { value: "range", label: t("dashboards.range") },
            { value: "instant", label: t("dashboards.instant") },
          ]}
          onValueChange={(value) =>
            onChange({ query_mode: value as "range" | "instant" })
          }
        />
      </Field>
      {target.query_mode === "range" && (
        <>
          <Field label={t("dashboards.stepPolicy")} requirement="optional">
            <Select
              sizing="content"
              value={target.range_step_policy.kind}
              options={[
                { value: "auto", label: t("dashboards.autoStep") },
                { value: "fixed", label: t("dashboards.fixedStep") },
              ]}
              onValueChange={(kind) =>
                onChange({
                  range_step_policy:
                    kind === "fixed"
                      ? {
                          kind,
                          seconds: target.range_step_policy.seconds ?? 60,
                        }
                      : {
                          kind: "auto",
                          target_points: 300,
                          min_step_seconds: 1,
                        },
                })
              }
            />
          </Field>
          {target.range_step_policy.kind === "fixed" && (
            <Field label={t("dashboards.stepSeconds")} requirement="required">
              <Input
                type="number"
                min={1}
                required
                value={target.range_step_policy.seconds ?? 60}
                onChange={(event) =>
                  onChange({
                    range_step_policy: {
                      kind: "fixed",
                      seconds: Number(event.target.value),
                    },
                  })
                }
              />
            </Field>
          )}
        </>
      )}
      <span className="argus-dashboard-muted">
        {t(
          target.query_mode === "instant"
            ? "dashboards.editor.instantHint"
            : "dashboards.editor.resolutionHint",
        )}
      </span>
    </div>
  );
}
