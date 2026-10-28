import type { ComponentProps, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { DashboardSchemas } from "@argus/api-client";
import { ComboBox, Field, Input, Select, ValueSelector } from "@argus/ui";
import {
  metricOperationCompatible,
  requiredMetricType,
  selectMetric,
  type MetricMetadata,
} from "./metric-metadata";
import "../../styles/dashboard-query-builder.css";

type Builder = DashboardSchemas["DashboardBuilder"];

/** Shared metric authoring controls for dashboards and temporary queries. */
export function MetricQueryFields({
  builder,
  onChange,
  metadata,
  metrics,
  contextKey,
  fetchMetrics,
  disabled,
  sourceControl,
}: {
  builder: Builder;
  onChange: (patch: Partial<Builder>) => void;
  metadata?: MetricMetadata;
  metrics: MetricMetadata[];
  contextKey: string;
  fetchMetrics?: ComponentProps<typeof ComboBox>["fetchOptions"];
  disabled?: boolean;
  sourceControl?: ReactNode;
}) {
  const { t } = useTranslation();
  return (
    <>
      <div className="argus-metric-query-basics">
        {sourceControl}
        <Field
          className="argus-metric-query-name"
          label={t("dashboards.metric")}
          requirement="required"
        >
          <ComboBox
            disabled={disabled}
            value={builder.metric ?? ""}
            allowCustom
            options={metrics.map((m) => ({
              value: m.name,
              label: m.name,
              description: [m.type, m.unit].filter(Boolean).join(" · "),
            }))}
            contextKey={contextKey}
            fetchOptions={disabled ? undefined : fetchMetrics}
            onValueChange={(metric) => onChange(selectMetric(builder, metric))}
          />
        </Field>
        <Field
          label={t("dashboards.editor.calculation")}
          requirement="required"
        >
          <Select
            value={builder.operation}
            options={[
              "value",
              "sum",
              "avg",
              "min",
              "max",
              "rate",
              "error_rate",
              "topk",
              "p95",
            ].map((value) => ({
              value,
              label: t(`dashboards.operations.${value}`),
              disabled: !metricOperationCompatible(value, metadata),
              description: requiredMetricType(value)
                ? t("dashboards.editor.metricRequirement", {
                    type: requiredMetricType(value),
                  })
                : undefined,
            }))}
            onValueChange={(operation) =>
              onChange({
                operation: operation as Builder["operation"],
                window_seconds: ["rate", "p95", "error_rate"].includes(
                  operation,
                )
                  ? (builder.window_seconds ?? 300)
                  : undefined,
                top_n: operation === "topk" ? (builder.top_n ?? 10) : undefined,
                error_filters:
                  operation === "error_rate"
                    ? (builder.error_filters ?? [
                        { field: "status", operator: "=~", value: "5.." },
                      ])
                    : undefined,
              })
            }
          />
        </Field>
        <Field label={t("dashboards.editor.split")} requirement="optional">
          <ValueSelector
            label={t("dashboards.editor.split")}
            multiple
            allLabel={t("dashboards.editor.noSplit")}
            value={{
              all: !builder.group_by?.length,
              values: builder.group_by ?? [],
            }}
            candidates={metadata?.labels ?? []}
            onChange={(selection) =>
              onChange({ group_by: selection.all ? [] : selection.values })
            }
          />
        </Field>
        {["rate", "p95", "error_rate"].includes(builder.operation) && (
          <Field label={t("dashboards.editor.window")} requirement="required">
            <Input
              type="number"
              min={1}
              required
              value={builder.window_seconds ?? 300}
              onChange={(e) =>
                onChange({ window_seconds: Number(e.target.value) })
              }
            />
          </Field>
        )}
        {builder.operation === "topk" && (
          <Field label={t("dashboards.operations.topk")} requirement="required">
            <Input
              type="number"
              min={1}
              max={100}
              required
              value={builder.top_n ?? 10}
              onChange={(e) => onChange({ top_n: Number(e.target.value) })}
            />
          </Field>
        )}
      </div>
      <p className="argus-dashboard-muted">
        {metadata
          ? `${metadata.type} · ${metadata.unit || t("dashboards.editor.unknownUnit")}`
          : t("dashboards.editor.metadataUnverified")}
      </p>
      {!metricOperationCompatible(builder.operation, metadata) && (
        <p role="alert" className="argus-dashboard-muted">
          {t("dashboards.editor.metricMismatch", {
            type: metadata?.type,
            required: requiredMetricType(builder.operation),
          })}
        </p>
      )}
    </>
  );
}
