import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Badge,
  Button,
  Checkbox,
  Field,
  Input,
  Select,
  ValueSelector,
} from "@argus/ui";
import {
  useApi,
  type DashboardSpec,
  type DashboardVariable,
  type DashboardSchemas,
} from "@argus/api-client";
import { isDashboardSignal, sources } from "./model";
import { FilterFields } from "./filter-fields";
import { ParameterBindingsEditor } from "./parameter-bindings-editor";
import { candidateRequest } from "./candidate-filters";
import { queryScopeBounds } from "./query-scope";

export function VariableQueryEditor({
  value: v,
  spec,
  onChange,
}: {
  value: DashboardVariable;
  spec: DashboardSpec;
  onChange: (patch: Partial<DashboardVariable>) => void;
}) {
  const { t } = useTranslation(),
    api = useApi();
  const [preview, setPreview] =
    useState<DashboardSchemas["DashboardCatalogResult"]>();
  const [busy, setBusy] = useState(false),
    [failed, setFailed] = useState(false);
  const active = useRef<AbortController | undefined>(undefined);
  const queryKey = JSON.stringify([
    v.query,
    spec.default_time_range,
    spec.variables.map((variable) => [variable.name, variable.default]),
  ]);
  useEffect(() => {
    active.current?.abort();
    setPreview(undefined);
    setBusy(false);
    setFailed(false);
    return () => active.current?.abort();
  }, [queryKey]);
  const fetchValues = (
    search: string,
    cursor: string | undefined,
    signal: AbortSignal,
  ) =>
    api.dashboards.catalog(
      candidateRequest(
        v.query,
        {
          ...queryScopeBounds(spec.default_time_range),
          resource_ids: [],
          variables: Object.fromEntries(
            spec.variables.map((variable) => [variable.name, variable.default]),
          ),
        },
        v.default,
        search,
        cursor,
      ),
      signal,
    );
  const load = async () => {
    active.current?.abort();
    const controller = new AbortController();
    active.current = controller;
    setBusy(true);
    setFailed(false);
    try {
      const result = await fetchValues("", undefined, controller.signal);
      if (!controller.signal.aborted) setPreview(result);
    } catch {
      if (!controller.signal.aborted) setFailed(true);
    } finally {
      if (!controller.signal.aborted) setBusy(false);
    }
  };
  return (
    <>
      <section className="argus-variable-section">
        <strong>{t("dashboardControls.querySettings")}</strong>
        <div className="argus-dashboard-form-grid">
          <Field label={t("dashboards.signal")} requirement="required">
            <Select
              value={v.query.signal}
              options={["metrics", "logs", "traces"].map((value) => ({
                value,
                label: t(`dashboards.editor.signals.${value}`),
              }))}
              onValueChange={(signal) =>
                isDashboardSignal(signal) &&
                onChange({
                  query: {
                    ...v.query,
                    signal,
                    source_binding: {
                      source_type: sources[signal][0]!,
                      capability_version: "v1",
                    },
                  },
                })
              }
            />
          </Field>
          <Field label={t("dashboards.source")} requirement="required">
            <Select
              value={v.query.source_binding.source_type}
              options={sources[v.query.signal].map((value) => ({
                value,
                label: t(`dashboards.editor.sources.${value}`),
              }))}
              onValueChange={(source_type) =>
                onChange({
                  query: {
                    ...v.query,
                    source_binding: { source_type, capability_version: "v1" },
                  },
                })
              }
            />
          </Field>
        </div>
        <Field
          label={t("dashboards.field")}
          requirement="required"
          hint={t("dashboardControls.fieldHint")}
        >
          <Input
            value={v.query.field}
            onChange={(e) =>
              onChange({ query: { ...v.query, field: e.target.value } })
            }
          />
        </Field>
        {v.query.signal === "metrics" && (
          <Field label={t("dashboards.metric")} requirement="optional">
            <Input
              value={v.query.metric ?? ""}
              onChange={(e) =>
                onChange({ query: { ...v.query, metric: e.target.value } })
              }
            />
          </Field>
        )}
        <details className="argus-variable-advanced">
          <summary>{t("dashboardControls.conditions")}</summary>
          <FilterFields
            filters={v.query.filters}
            variables={spec.variables.filter((other) => other.id !== v.id)}
            onChange={(filters) => onChange({ query: { ...v.query, filters } })}
          />
          <ParameterBindingsEditor
            value={v.query.parameter_bindings ?? []}
            variables={spec.variables.filter((other) => other.id !== v.id)}
            locals={[]}
            onChange={(parameter_bindings) =>
              onChange({ query: { ...v.query, parameter_bindings } })
            }
          />
        </details>
      </section>
      <section className="argus-variable-section">
        <strong>{t("dashboardControls.selectionSettings")}</strong>
        <Checkbox
          isSelected={v.multiple}
          onChange={(multiple) => onChange({ multiple })}
        >
          {t("dashboards.multiple")}
        </Checkbox>
        <ValueSelector
          presentation="popover"
          label={t("dashboardControls.defaultSelection")}
          multiple={v.multiple}
          value={v.default}
          candidates={preview?.values ?? []}
          fetchValues={fetchValues}
          contextKey={queryKey}
          disabled={!v.query.field.trim()}
          onChange={(value) => onChange({ default: value, include_all: true })}
        />
      </section>
      <section className="argus-variable-preview">
        <div className="argus-dashboard-inline">
          <Button
            isPending={busy}
            isDisabled={!v.query.field.trim()}
            onPress={() => void load()}
          >
            {t("dashboardControls.previewCandidates")}
          </Button>
          <span className="argus-dashboard-muted">
            {t("dashboardControls.previewHint")}
          </span>
        </div>
        {failed && <p role="alert">{t("dashboards.candidateFailed")}</p>}
        {preview && (
          <>
            <p className="argus-dashboard-muted">
              {t("dashboardControls.previewCount", {
                count: preview.values.length,
              })}
              {preview.next_cursor &&
                ` · ${t("dashboardControls.previewPartial")}`}
            </p>
            <div className="argus-variable-preview__values">
              {preview.values.map((value) => (
                <Badge key={value}>{value}</Badge>
              ))}
            </div>
          </>
        )}
      </section>
    </>
  );
}
