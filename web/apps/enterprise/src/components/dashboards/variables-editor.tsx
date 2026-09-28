import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type DashboardSpec,
  type DashboardVariable,
} from "@argus/api-client";
import { Button, Field, FormDrawer, Input, Select } from "@argus/ui";
import { isDashboardSignal, sources } from "./model";
import { FilterFields } from "./filter-fields";
import { ParameterBindingsEditor } from "./parameter-bindings-editor";
import { TimeRangePicker } from "./time-range-picker";
import { candidateFilters } from "./candidate-filters";

export function VariablesEditor({
  spec,
  onClose,
  onSave,
}: {
  spec: DashboardSpec;
  onClose: () => void;
  onSave: (spec: DashboardSpec) => void;
}) {
  const { t } = useTranslation(),
    api = useApi();
  const [value, setValue] = useState(() => structuredClone(spec));
  const [candidates, setCandidates] = useState<Record<string, string[]>>({});
  const [errors, setErrors] = useState<Record<string, boolean>>({});
  const update = (id: string, patch: Partial<DashboardVariable>) =>
    setValue((s) => ({
      ...s,
      variables: s.variables.map((v) => (v.id === id ? { ...v, ...patch } : v)),
    }));
  return (
    <FormDrawer
      open
      width={860}
      onOpenChange={(open) => !open && onClose()}
      title={t("dashboards.filtersTab")}
      submitLabel={t("dashboards.done")}
      onSubmit={() => onSave(value)}
    >
      <div className="argus-dashboard-form-stack">
        <div className="argus-dashboard-form-grid">
          <TimeRangePicker
            label={t("dashboards.defaultTime")}
            value={value.default_time_range}
            onChange={(default_time_range) =>
              setValue({ ...value, default_time_range })
            }
          />
          <Field label={t("dashboards.defaultRefresh")} requirement="required">
            <Input
              type="number"
              min={0}
              step={5}
              value={value.default_refresh_seconds}
              onChange={(e) =>
                setValue({
                  ...value,
                  default_refresh_seconds: Number(e.target.value),
                })
              }
            />
          </Field>
        </div>
        {value.variables.map((v) => (
          <section className="argus-dashboard-editor-section" key={v.id}>
            <div className="argus-dashboard-form-grid">
              <Field
                label={t("dashboards.variableName")}
                requirement="required"
              >
                <Input
                  value={v.name}
                  pattern="[A-Za-z_][A-Za-z0-9_]{0,63}"
                  onChange={(e) => update(v.id, { name: e.target.value })}
                />
              </Field>
              <Field label={t("dashboards.label")} requirement="required">
                <Input
                  value={v.label}
                  onChange={(e) => update(v.id, { label: e.target.value })}
                />
              </Field>
              <Field label={t("dashboards.signal")} requirement="required">
                <Select
                  value={v.query.signal}
                  onValueChange={(signal) =>
                    isDashboardSignal(signal) &&
                    update(v.id, {
                      query: {
                        ...v.query,
                        signal,
                        source_binding: {
                          source_type:
                            sources[signal as keyof typeof sources][0]!,
                          capability_version: "v1",
                        },
                      },
                    })
                  }
                  options={["metrics", "logs", "traces"].map((value) => ({
                    value,
                    label: value,
                  }))}
                />
              </Field>
              <Field label={t("dashboards.source")} requirement="required">
                <Select
                  value={v.query.source_binding.source_type}
                  onValueChange={(source_type) =>
                    update(v.id, {
                      query: {
                        ...v.query,
                        source_binding: {
                          source_type,
                          capability_version: "v1",
                        },
                      },
                    })
                  }
                  options={sources[v.query.signal as keyof typeof sources].map(
                    (value) => ({ value, label: value }),
                  )}
                />
              </Field>
            </div>
            <Field
              label={t("dashboards.field")}
              requirement="required"
              hint={t("dashboards.fieldHelp")}
            >
              <Input
                value={v.query.field}
                onChange={(e) =>
                  update(v.id, { query: { ...v.query, field: e.target.value } })
                }
              />
            </Field>
            {v.query.signal === "metrics" && (
              <Field label={t("dashboards.metric")} requirement="optional">
                <Input
                  value={v.query.metric ?? ""}
                  onChange={(e) =>
                    update(v.id, {
                      query: { ...v.query, metric: e.target.value },
                    })
                  }
                />
              </Field>
            )}
            <div className="argus-dashboard-inline">
              <label className="argus-dashboard-check">
                <input
                  type="checkbox"
                  checked={v.multiple}
                  onChange={(e) => update(v.id, { multiple: e.target.checked })}
                />
                {t("dashboards.multiple")}
              </label>
              <label className="argus-dashboard-check">
                <input
                  type="checkbox"
                  checked={v.default.all}
                  onChange={(e) =>
                    update(v.id, {
                      default: { all: e.target.checked, values: [] },
                      include_all: true,
                    })
                  }
                />
                {t("dashboards.defaultAll")}
              </label>
            </div>
            {!v.default.all && (
              <Field
                label={t("dashboards.defaultValues")}
                requirement="required"
              >
                <Input
                  value={v.default.values.join(", ")}
                  onChange={(e) =>
                    update(v.id, {
                      default: {
                        all: false,
                        values: e.target.value
                          .split(",")
                          .map((s) => s.trim())
                          .filter(Boolean),
                      },
                    })
                  }
                />
              </Field>
            )}
            <FilterFields
              filters={v.query.filters}
              variables={value.variables.filter((other) => other.id !== v.id)}
              onChange={(filters) =>
                update(v.id, { query: { ...v.query, filters } })
              }
            />
            <div className="argus-dashboard-inline">
              <Button
                size="sm"
                onClick={async () => {
                  setErrors((previous) => ({ ...previous, [v.id]: false }));
                  try {
                    const to = new Date();
                    await api.dashboards
                      .catalog({
                        source_binding: v.query.source_binding,
                        signal: v.query.signal as "metrics" | "logs" | "traces",
                        kind: "values",
                        field: v.query.field,
                        metric: v.query.metric,
                        from: new Date(
                          to.getTime() -
                            (value.default_time_range.seconds ?? 3600) * 1000,
                        ).toISOString(),
                        to: to.toISOString(),
                        selected_values: v.default.values,
                        filters: candidateFilters(
                          v.query,
                          Object.fromEntries(
                            value.variables.map((variable) => [
                              variable.name,
                              variable.default,
                            ]),
                          ),
                        ),
                        resource_ids: [],
                        limit: 100,
                      })
                      .then((data) =>
                        setCandidates((previous) => ({
                          ...previous,
                          [v.id]: data.values,
                        })),
                      );
                  } catch {
                    setErrors((previous) => ({ ...previous, [v.id]: true }));
                  }
                }}
              >
                {t("dashboards.candidates")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() =>
                  setValue({
                    ...value,
                    variables: value.variables.filter(
                      (entry) => entry.id !== v.id,
                    ),
                  })
                }
              >
                {t("dashboards.delete")}
              </Button>
            </div>
            <ParameterBindingsEditor
              value={v.query.parameter_bindings ?? []}
              variables={value.variables.filter((other) => other.id !== v.id)}
              locals={[]}
              onChange={(parameter_bindings) =>
                update(v.id, { query: { ...v.query, parameter_bindings } })
              }
            />
            {errors[v.id] && (
              <p role="alert">{t("dashboards.candidateFailed")}</p>
            )}
            {candidates[v.id] && (
              <p className="argus-dashboard-muted">
                {candidates[v.id]!.join(" · ") || t("dashboards.noCandidate")}
              </p>
            )}
          </section>
        ))}
        <Button
          onClick={() =>
            setValue({
              ...value,
              variables: [
                ...value.variables,
                {
                  id: crypto.randomUUID(),
                  name: `variable_${value.variables.length + 1}`,
                  label: `variable_${value.variables.length + 1}`,
                  multiple: false,
                  include_all: true,
                  default: { all: true, values: [] },
                  query: {
                    signal: "metrics",
                    source_binding: {
                      source_type: "hostmetrics",
                      capability_version: "v1",
                    },
                    field: "host",
                    filters: [],
                  },
                },
              ],
            })
          }
        >
          {t("dashboards.addVariable")}
        </Button>
      </div>
    </FormDrawer>
  );
}
