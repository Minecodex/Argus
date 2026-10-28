import { useMemo, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { useEnterpriseAuthStore } from "@argus/auth";
import { Button, ComboBox, Field, Select, JsonField } from "@argus/ui";
import { Plus, X } from "lucide-react";
import type {
  DashboardPanel,
  DashboardSchemas,
  DashboardSpec,
  DashboardTarget,
} from "@argus/api-client";
import { candidateFilters } from "./candidate-filters";
import {
  queryScopeBounds,
  useDashboardQueryScope,
  useQueryCatalog,
} from "./query-scope";

type Filter = DashboardSchemas["DashboardFilter"];
type Loader = NonNullable<ComponentProps<typeof ComboBox>["fetchOptions"]>;

export function MetricLabelFilters({
  panel,
  spec,
  target,
  filters,
  onChange,
  labels,
  onInvalidInput,
  literalOnly = false,
}: {
  panel: DashboardPanel;
  spec: DashboardSpec;
  target: DashboardTarget;
  filters: Filter[];
  onChange: (filters: Filter[]) => void;
  labels: string[];
  onInvalidInput?: () => void;
  literalOnly?: boolean;
}) {
  const { t } = useTranslation(),
    scope = useDashboardQueryScope(),
    api = useQueryCatalog();
  const session = useEnterpriseAuthStore((state) => state.session?.session);
  const metric = target.source_definition.builder?.metric;
  const scopeKey = JSON.stringify([
    session,
    scope?.key,
    scope?.resources,
    scope?.time ?? spec.default_time_range,
    panel.source_binding,
    metric,
  ]);
  const { kind, from, to, seconds } = scope?.time ?? spec.default_time_range;
  const range = useMemo(
    () => queryScopeBounds({ kind, from, to, seconds }),
    [kind, from, to, seconds],
  );
  const ready =
    scope?.catalog !== false && !!metric && !!panel.source_binding.source_type;
  const variables =
    scope?.variables ??
    Object.fromEntries(spec.variables.map((v) => [v.name, v.default]));
  const locals =
    scope?.locals ??
    Object.fromEntries(panel.local_filters.map((v) => [v.id, v.default]));
  const base = {
    signal: "metrics" as const,
    source_binding: panel.source_binding,
    metric,
    ...range,
    resource_ids: scope?.resources ?? [],
    selected_values: [],
    limit: 100,
  };
  const fields: Loader = async (search, cursor, signal) => {
    const result = await api(
      { ...base, kind: "fields", filters: [], search, cursor },
      signal,
    );
    return {
      options: result.fields
        .filter((f) => f.name !== "__name__")
        .map((f) => ({ value: f.name, label: f.name })),
      next_cursor: result.next_cursor,
    };
  };
  const patch = (index: number, next: Filter) =>
    onChange(filters.map((value, i) => (i === index ? next : value)));
  return (
    <div className="argus-metric-label-filters">
      {filters.map((filter, index) => {
        const others = filters.filter((f, i) => i !== index && !!f.field);
        const valueContext = JSON.stringify([
          scopeKey,
          filter.field,
          filter.operator,
          others,
          variables,
          locals,
          target.parameter_bindings,
        ]);
        const values: Loader = async (search, cursor, signal) => {
          const predicates = candidateFilters(
            {
              signal: "metrics",
              source_binding: panel.source_binding,
              metric,
              field: filter.field,
              filters: others,
              parameter_bindings: target.parameter_bindings,
            },
            variables,
            locals,
          );
          const result = await api(
            {
              ...base,
              kind: "values",
              field: filter.field,
              filters: predicates,
              search,
              cursor,
            },
            signal,
          );
          return {
            options: result.values.map((value) => ({ value, label: value })),
            next_cursor: result.next_cursor,
          };
        };
        const binding = filter.variable
          ? `var:${filter.variable}`
          : filter.local_parameter
            ? `local:${filter.local_parameter}`
            : filter.drilldown_input
              ? `row:${filter.drilldown_input}`
              : filter.values
                ? "values"
                : "literal";
        return (
          <div className="argus-metric-label-row" key={index}>
            <Field label={t("dashboards.editor.label")} requirement="required">
              <ComboBox
                key={scopeKey}
                value={filter.field}
                disabled={!ready}
                options={labels
                  .filter((name) => name !== "__name__")
                  .map((value) => ({ value, label: value }))}
                contextKey={scopeKey}
                fetchOptions={ready ? fields : undefined}
                onValueChange={(field) =>
                  patch(index, { field, operator: filter.operator, value: "" })
                }
              />
            </Field>
            <Field label={t("dashboards.operation")} requirement="optional">
              <Select
                sizing="content"
                value={filter.operator}
                options={["=", "!=", "=~", "!~"].map((value) => ({
                  value,
                  label: value,
                }))}
                onValueChange={(operator) =>
                  patch(index, { ...filter, operator })
                }
              />
            </Field>
            {!literalOnly &&
              (spec.variables.length > 0 ||
                panel.local_filters.length > 0 ||
                binding !== "literal") && (
                <Field label={t("dashboards.variable")} requirement="optional">
                  <Select
                    sizing="content"
                    value={binding}
                    options={[
                      { value: "literal", label: t("dashboards.value") },
                      { value: "values", label: t("dashboards.valueSet") },
                      ...spec.variables.map((v) => ({
                        value: `var:${v.name}`,
                        label: `$${v.name}`,
                      })),
                      ...panel.local_filters.map((v) => ({
                        value: `local:${v.id}`,
                        label: v.label,
                      })),
                      ...(filter.drilldown_input
                        ? [{ value: binding, label: filter.drilldown_input }]
                        : []),
                    ]}
                    onValueChange={(value) =>
                      patch(index, {
                        field: filter.field,
                        operator: filter.operator,
                        ...(value.startsWith("var:")
                          ? { variable: value.slice(4) }
                          : value.startsWith("local:")
                            ? { local_parameter: value.slice(6) }
                            : value.startsWith("row:")
                              ? { drilldown_input: value.slice(4) }
                              : value === "values"
                                ? { values: [] }
                                : { value: "" }),
                      })
                    }
                  />
                </Field>
              )}
            {binding === "literal" && (
              <Field label={t("dashboards.value")} requirement="optional">
                <ComboBox
                  key={valueContext}
                  value={filter.value ?? ""}
                  allowCustom
                  options={[]}
                  disabled={!ready || !filter.field}
                  contextKey={valueContext}
                  fetchOptions={ready && filter.field ? values : undefined}
                  onValueChange={(value) => patch(index, { ...filter, value })}
                />
              </Field>
            )}
            {binding === "values" && (
              <JsonField
                label={t("dashboards.valueSet")}
                kind="strings"
                value={filter.values ?? []}
                onInvalidChange={(invalid) => {
                  if (invalid) onInvalidInput?.();
                }}
                onChange={(values) => patch(index, { ...filter, values })}
              />
            )}
            <Button
              variant="ghost"
              isIconOnly
              aria-label={t("dashboards.editor.removeFilter", {
                field: filter.field || index + 1,
              })}
              onPress={() => onChange(filters.filter((_, i) => i !== index))}
            >
              <X aria-hidden />
            </Button>
          </div>
        );
      })}
      <Button
        variant="ghost"
        size="sm"
        isDisabled={!ready}
        onPress={() =>
          onChange([...filters, { field: "", operator: "=", value: "" }])
        }
      >
        <Plus aria-hidden />
        {t("dashboards.addFilter")}
      </Button>
    </div>
  );
}
