import { Button, Field, Input, Select, JsonField } from "@argus/ui";
import type { DashboardSchemas, DashboardVariable } from "@argus/api-client";
import { useTranslation } from "react-i18next";
export function FilterFields({
  filters,
  onChange,
  variables,
  locals = [],
  rowInputs = [],
  operators = ["=", "!=", "=~", "!~", ">", ">=", "<", "<=", ":"],
}: {
  filters: DashboardSchemas["DashboardFilter"][];
  onChange: (filters: DashboardSchemas["DashboardFilter"][]) => void;
  variables: DashboardVariable[];
  locals?: string[];
  rowInputs?: string[];
  operators?: string[];
}) {
  const { t } = useTranslation();
  return (
    <div className="argus-dashboard-form-stack">
      {filters.map((f, i) => (
        <div className="argus-dashboard-filter-row" key={i}>
          <Field label={t("dashboards.field")} requirement="required">
            <Input
              value={f.field}
              onChange={(e) =>
                onChange(
                  filters.map((v, n) =>
                    n === i ? { ...v, field: e.target.value } : v,
                  ),
                )
              }
            />
          </Field>
          <Select
            ariaLabel={t("dashboards.operation")}
            value={f.operator}
            onValueChange={(operator) =>
              onChange(
                filters.map((v, n) => (n === i ? { ...v, operator } : v)),
              )
            }
            options={operators.map((value) => ({ value, label: value }))}
          />
          <Select
            ariaLabel={t("dashboards.variable")}
            value={
              f.variable
                ? `var:${f.variable}`
                : f.local_parameter
                  ? `local:${f.local_parameter}`
                  : f.drilldown_input
                    ? `row:${f.drilldown_input}`
                    : f.values
                      ? "values"
                      : "literal"
            }
            onValueChange={(value) =>
              onChange(
                filters.map((v, n) =>
                  n === i
                    ? {
                        field: v.field,
                        operator: v.operator,
                        ...(value.startsWith("var:")
                          ? { variable: value.slice(4) }
                          : value.startsWith("local:")
                            ? { local_parameter: value.slice(6) }
                            : value.startsWith("row:")
                              ? { drilldown_input: value.slice(4) }
                              : value === "values"
                                ? { values: [] }
                                : { value: "" }),
                      }
                    : v,
                ),
              )
            }
            options={[
              { value: "literal", label: t("dashboards.value") },
              { value: "values", label: t("dashboards.valueSet") },
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
          {!f.variable &&
            !f.local_parameter &&
            !f.drilldown_input &&
            (f.values ? (
              <JsonField
                label={t("dashboards.valueSet")}
                kind="strings"
                value={f.values}
                onChange={(values) =>
                  onChange(
                    filters.map((v, n) => (n === i ? { ...v, values } : v)),
                  )
                }
              />
            ) : (
              <Input
                aria-label={t("dashboards.value")}
                value={f.value ?? ""}
                onChange={(e) =>
                  onChange(
                    filters.map((v, n) =>
                      n === i ? { ...v, value: e.target.value } : v,
                    ),
                  )
                }
              />
            ))}
          <Button
            variant="ghost"
            size="sm"
            onPress={() => onChange(filters.filter((_, n) => n !== i))}
          >
            {t("dashboards.delete")}
          </Button>
        </div>
      ))}
      <Button
        size="sm"
        onPress={() =>
          onChange([...filters, { field: "", operator: "=", value: "" }])
        }
      >
        {t("dashboards.addFilter")}
      </Button>
    </div>
  );
}
