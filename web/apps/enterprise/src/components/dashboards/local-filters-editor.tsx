import { Checkbox } from "@argus/ui";
import { useTranslation } from "react-i18next";
import type {
  DashboardPanel,
  DashboardSpec,
  DashboardSchemas,
} from "@argus/api-client";
import { Button, Field, Input, Select } from "@argus/ui";
import { FilterFields } from "./filter-fields";
import { ParameterBindingsEditor } from "./parameter-bindings-editor";

export function LocalFiltersEditor({
  panel,
  spec,
  onChange,
}: {
  panel: DashboardPanel;
  spec: DashboardSpec;
  onChange: (filters: DashboardPanel["local_filters"]) => void;
}) {
  const { t } = useTranslation();
  const patch = (
    id: string,
    next: Partial<DashboardSchemas["DashboardLocalFilter"]>,
  ) =>
    onChange(
      panel.local_filters.map((f) => (f.id === id ? { ...f, ...next } : f)),
    );
  const makeQuery = () => ({
    signal: panel.signal,
    source_binding: panel.source_binding,
    field: "",
    filters: [],
    parameter_bindings: [],
  });
  return (
    <details>
      <summary>{t("dashboards.localFilters")}</summary>
      <p className="argus-dashboard-muted">{t("dashboards.localHint")}</p>
      <div className="argus-dashboard-form-stack">
        {panel.local_filters.map((filter) => (
          <section className="argus-dashboard-editor-section" key={filter.id}>
            <div className="argus-dashboard-form-grid">
              <Field
                label={t("dashboards.variableName")}
                requirement="required"
              >
                <Input value={filter.id} readOnly />
              </Field>
              <Field label={t("dashboards.label")} requirement="required">
                <Input
                  value={filter.label}
                  onChange={(e) => patch(filter.id, { label: e.target.value })}
                />
              </Field>
              <Select
                ariaLabel={t("dashboards.filterKind")}
                value={filter.kind}
                options={[
                  { value: "query", label: t("dashboards.queryVariable") },
                  { value: "text", label: t("dashboards.textValue") },
                  { value: "number", label: t("dashboards.numberValue") },
                ]}
                onValueChange={(kind) =>
                  patch(filter.id, {
                    kind: kind as "query" | "text" | "number",
                    query: kind === "query" ? makeQuery() : undefined,
                  })
                }
              />
              <Button
                variant="ghost"
                onPress={() =>
                  onChange(
                    panel.local_filters.filter((f) => f.id !== filter.id),
                  )
                }
              >
                {t("dashboards.delete")}
              </Button>
            </div>
            {filter.query && (
              <>
                <Field label={t("dashboards.field")} requirement="required">
                  <Input
                    value={filter.query.field}
                    onChange={(e) =>
                      patch(filter.id, {
                        query: { ...filter.query!, field: e.target.value },
                      })
                    }
                  />
                </Field>
                {panel.signal === "metrics" && (
                  <Field label={t("dashboards.metric")} requirement="optional">
                    <Input
                      value={filter.query.metric ?? ""}
                      onChange={(e) =>
                        patch(filter.id, {
                          query: { ...filter.query!, metric: e.target.value },
                        })
                      }
                    />
                  </Field>
                )}
                <FilterFields
                  filters={filter.query.filters}
                  variables={spec.variables}
                  locals={panel.local_filters
                    .filter((f) => f.id !== filter.id)
                    .map((f) => f.id)}
                  onChange={(filters) =>
                    patch(filter.id, { query: { ...filter.query!, filters } })
                  }
                />
                <ParameterBindingsEditor
                  value={filter.query.parameter_bindings ?? []}
                  variables={spec.variables}
                  locals={panel.local_filters
                    .filter((f) => f.id !== filter.id)
                    .map((f) => f.id)}
                  onChange={(parameter_bindings) =>
                    patch(filter.id, {
                      query: { ...filter.query!, parameter_bindings },
                    })
                  }
                />
              </>
            )}
            <Checkbox
              className="argus-dashboard-check"
              isSelected={filter.multiple ?? false}
              onChange={(selected) => patch(filter.id, { multiple: selected })}
            >
              {t("dashboards.multiple")}
            </Checkbox>
            <Checkbox
              className="argus-dashboard-check"
              isSelected={filter.default.all}
              onChange={(selected) =>
                patch(filter.id, {
                  default: { all: selected, values: [] },
                })
              }
            >
              {t("dashboards.defaultAll")}
            </Checkbox>
            {!filter.default.all && (
              <Field
                label={t("dashboards.defaultValues")}
                requirement="required"
              >
                <Input
                  value={filter.default.values.join(",")}
                  onChange={(e) =>
                    patch(filter.id, {
                      default: {
                        all: false,
                        values: e.target.value
                          .split(",")
                          .map((v) => v.trim())
                          .filter(Boolean),
                      },
                    })
                  }
                />
              </Field>
            )}
          </section>
        ))}
        <Button
          size="sm"
          onPress={() => {
            const id = `local_${crypto.randomUUID().replaceAll("-", "").slice(0, 8)}`;
            onChange([
              ...panel.local_filters,
              {
                id,
                label: id,
                kind: "query",
                query: makeQuery(),
                default: { all: true, values: [] },
                multiple: false,
              },
            ]);
          }}
        >
          {t("dashboards.addLocal")}
        </Button>
      </div>
    </details>
  );
}
