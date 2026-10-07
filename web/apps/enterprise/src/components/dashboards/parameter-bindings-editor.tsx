import { useTranslation } from "react-i18next";
import type { DashboardSchemas, DashboardVariable } from "@argus/api-client";
import { Button, Field, Input, Select } from "@argus/ui";
type Binding = DashboardSchemas["DashboardParameterBinding"];
export function ParameterBindingsEditor({
  value,
  onChange,
  variables,
  locals,
  rowInputs = [],
}: {
  value: Binding[];
  onChange: (value: Binding[]) => void;
  variables: DashboardVariable[];
  locals: string[];
  rowInputs?: string[];
}) {
  const { t } = useTranslation();
  const patch = (index: number, next: Partial<Binding>) =>
    onChange(value.map((v, i) => (i === index ? { ...v, ...next } : v)));
  return (
    <details>
      <summary>{t("dashboards.parameterBindings")}</summary>
      <p className="argus-dashboard-muted">{t("dashboards.mappingHint")}</p>
      <div className="argus-dashboard-form-stack">
        {value.map((binding, index) => (
          <section key={index} className="argus-dashboard-editor-section">
            <div className="argus-dashboard-form-grid">
              <Field
                label={t("dashboards.parameterName")}
                requirement="required"
              >
                <Input
                  value={binding.parameter}
                  pattern="[A-Za-z_][A-Za-z0-9_]*"
                  onChange={(e) => patch(index, { parameter: e.target.value })}
                />
              </Field>
              <Select
                ariaLabel={t("dashboards.variable")}
                value={
                  binding.variable
                    ? `var:${binding.variable}`
                    : binding.drilldown_input
                      ? `row:${binding.drilldown_input}`
                      : `local:${binding.local_parameter ?? ""}`
                }
                onValueChange={(v) =>
                  patch(index, {
                    variable: v.startsWith("var:") ? v.slice(4) : undefined,
                    local_parameter: v.startsWith("local:")
                      ? v.slice(6)
                      : undefined,
                    drilldown_input: v.startsWith("row:")
                      ? v.slice(4)
                      : undefined,
                    ...(v.startsWith("row:")
                      ? { identity_mapping: false, value_map: undefined }
                      : {}),
                  })
                }
                options={[
                  ...rowInputs.map((value) => ({
                    value: `row:${value}`,
                    label: `${t("dashboards.rowInput")}: ${value}`,
                  })),
                  ...variables.map((v) => ({
                    value: `var:${v.name}`,
                    label: `$${v.name}`,
                  })),
                  ...locals.map((v) => ({
                    value: `local:${v}`,
                    label: `${t("dashboards.local")}: ${v}`,
                  })),
                ]}
              />
              <Select
                ariaLabel={t("dashboards.mapping")}
                disabled={Boolean(binding.drilldown_input)}
                value={
                  binding.identity_mapping
                    ? "identity"
                    : binding.value_map
                      ? "map"
                      : "same"
                }
                onValueChange={(v) =>
                  patch(index, {
                    identity_mapping: v === "identity",
                    value_map: v === "map" ? {} : undefined,
                  })
                }
                options={[
                  { value: "same", label: t("dashboards.sameSource") },
                  { value: "identity", label: t("dashboards.identityMapping") },
                  { value: "map", label: t("dashboards.valueMapping") },
                ]}
              />
              <Button
                variant="ghost"
                onPress={() => onChange(value.filter((_, i) => i !== index))}
              >
                {t("dashboards.delete")}
              </Button>
            </div>
            {binding.value_map && (
              <div className="argus-dashboard-form-stack">
                {Object.entries(binding.value_map).map(([from, to], i) => (
                  <div className="argus-dashboard-inline" key={i}>
                    <Input
                      aria-label={t("dashboards.mappingFrom")}
                      value={from}
                      onChange={(e) =>
                        patch(index, {
                          value_map: Object.fromEntries(
                            Object.entries(binding.value_map!).map(
                              ([k, v], n) => [n === i ? e.target.value : k, v],
                            ),
                          ),
                        })
                      }
                    />
                    <span>→</span>
                    <Input
                      aria-label={t("dashboards.mappingTo")}
                      value={to}
                      onChange={(e) =>
                        patch(index, {
                          value_map: {
                            ...binding.value_map,
                            [from]: e.target.value,
                          },
                        })
                      }
                    />
                    <Button
                      variant="ghost"
                      onPress={() =>
                        patch(index, {
                          value_map: Object.fromEntries(
                            Object.entries(binding.value_map!).filter(
                              ([k]) => k !== from,
                            ),
                          ),
                        })
                      }
                    >
                      {t("dashboards.delete")}
                    </Button>
                  </div>
                ))}
                <Button
                  size="sm"
                  onPress={() =>
                    patch(index, {
                      value_map: {
                        ...binding.value_map,
                        [`value_${Object.keys(binding.value_map!).length + 1}`]:
                          "",
                      },
                    })
                  }
                >
                  {t("dashboards.addMapping")}
                </Button>
              </div>
            )}
          </section>
        ))}
        <Button
          size="sm"
          isDisabled={!variables.length && !locals.length && !rowInputs.length}
          onPress={() =>
            onChange([
              ...value,
              {
                parameter: `param_${value.length + 1}`,
                ...(variables[0]
                  ? { variable: variables[0].name }
                  : locals[0]
                    ? { local_parameter: locals[0] }
                    : { drilldown_input: rowInputs[0] }),
              },
            ])
          }
        >
          {t("dashboards.addBinding")}
        </Button>
      </div>
    </details>
  );
}
