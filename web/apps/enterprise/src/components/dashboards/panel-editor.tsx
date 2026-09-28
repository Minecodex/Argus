import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type DashboardPanel,
  type DashboardSchemas,
  type DashboardSpec,
  type DashboardTarget,
} from "@argus/api-client";
import {
  Button,
  ConfirmDialog,
  Field,
  FormDrawer,
  Input,
  Select,
} from "@argus/ui";
import { QueryTargetEditor } from "./query-target-editor";
import { DrilldownsEditor } from "./drilldowns-editor";
import { LocalFiltersEditor } from "./local-filters-editor";
import { compatibleDisplay, DisplayEditor } from "./display-editor";
import {
  isDashboardSignal,
  metricCharts,
  newTarget,
  sources,
  traceCharts,
} from "./model";

export function PanelEditor({
  initial,
  spec,
  isNew,
  onClose,
  onSave,
}: {
  initial: DashboardPanel;
  spec: DashboardSpec;
  isNew: boolean;
  onClose: () => void;
  onSave: (panel: DashboardPanel) => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const [panel, setPanel] = useState(() => structuredClone(initial));
  const [replace, setReplace] = useState<{
    signal: DashboardPanel["signal"];
    mode: "builder" | "dsl";
  } | null>(null);
  const [error, setError] = useState("");
  const [targetId, setTargetId] = useState(initial.targets[0]!.id);
  const target =
    panel.targets.find((t) => t.id === targetId) ?? panel.targets[0]!;
  const patch = (value: Partial<DashboardPanel>) =>
    setPanel((p) => ({ ...p, ...value }));
  const patchTarget = (value: Partial<DashboardTarget>) =>
    patch({
      targets: panel.targets.map((t) =>
        t.id === target.id ? { ...target, ...value } : t,
      ),
    });
  const replaceQuery = (
    signal: DashboardPanel["signal"],
    mode: "builder" | "dsl",
  ) => {
    const type =
      signal === "metrics"
        ? "timeseries"
        : signal === "logs"
          ? "logs"
          : "trace_list";
    setPanel((p) => ({
      ...p,
      signal,
      type,
      authoring_mode: mode,
      source_binding: {
        source_type: sources[signal as keyof typeof sources][0]!,
        capability_version: "v1",
      },
      targets: [newTarget(signal, type, mode)],
      detail_query_targets: [],
      drilldowns: [],
      display: undefined,
      thresholds: [],
    }));
    setReplace(null);
    setTargetId("main");
  };
  const changeSource = (signal: string, mode: "builder" | "dsl") => {
    if (!isDashboardSignal(signal)) return;
    if (isNew) replaceQuery(signal, mode);
    else setReplace({ signal, mode });
  };
  const [converting, setConverting] = useState(false);
  const fields = useRef<HTMLFieldSetElement>(null);
  const convert = async (mode: "builder" | "dsl") => {
    if (converting || mode === panel.authoring_mode) return;
    if (fields.current?.form && !fields.current.form.reportValidity()) return;
    setConverting(true);
    setError("");
    try {
      const result = await api.dashboards.convertPanel({ panel, mode });
      if (result.converted) setPanel(result.panel);
      else
        setError(
          t(
            result.issues.some((i) => i.code === "MOCK_CONVERSION_UNSUPPORTED")
              ? "dashboards.mockConversion"
              : "dashboards.conversionBlocked",
          ) +
            " " +
            result.issues.map((i) => `${i.path}: ${i.message}`).join("; "),
        );
    } catch {
      setError(t("dashboards.failed"));
    } finally {
      setConverting(false);
    }
  };
  return (
    <FormDrawer
      open
      onOpenChange={(open) => !open && onClose()}
      title={t(isNew ? "dashboards.addPanel" : "dashboards.editPanel")}
      width={820}
      submitLabel={t("dashboards.done")}
      loading={converting}
      onSubmit={() => !converting && onSave(panel)}
    >
      <fieldset
        ref={fields}
        disabled={converting}
        className="argus-dashboard-form-stack argus-dashboard-fieldset"
      >
        <Field label={t("dashboards.panelTitle")} requirement="required">
          <Input
            value={panel.title}
            maxLength={240}
            onChange={(e) => patch({ title: e.target.value })}
          />
        </Field>
        <div className="argus-dashboard-form-grid">
          <Field label={t("dashboards.signal")} requirement="required">
            <Select
              value={panel.signal}
              options={["metrics", "logs", "traces"].map((value) => ({
                value,
                label: value,
              }))}
              onValueChange={(v) =>
                changeSource(v, panel.authoring_mode as "builder" | "dsl")
              }
            />
          </Field>
          <Field label={t("dashboards.source")} requirement="required">
            <Select
              value={panel.source_binding.source_type}
              options={sources[panel.signal as keyof typeof sources].map(
                (value) => ({ value, label: value }),
              )}
              onValueChange={(v) =>
                patch({
                  source_binding: { source_type: v, capability_version: "v1" },
                })
              }
            />
          </Field>
          <Field label={t("dashboards.chart")} requirement="required">
            <Select
              value={panel.type}
              options={(panel.signal === "metrics"
                ? metricCharts
                : panel.signal === "logs"
                  ? ["logs", "timeseries", "bar", "table", "stat", "pie"]
                  : traceCharts
              ).map((value) => ({
                value,
                label: t(`dashboards.charts.${value}`),
              }))}
              onValueChange={(type) => {
                const query = structuredClone(target);
                if (
                  query.source_definition.builder &&
                  type.startsWith("apm_")
                ) {
                  query.source_definition.builder.operation =
                    type as DashboardSchemas["DashboardBuilder"]["operation"];
                  query.source_definition.builder.bucket_seconds =
                    type === "apm_red" ? 60 : undefined;
                }
                if (
                  panel.signal === "traces" &&
                  !type.startsWith("apm_") &&
                  query.source_definition.builder
                )
                  query.source_definition.builder.operation =
                    type === "trace_detail" ? "detail" : "list";
                patch({
                  type,
                  ...compatibleDisplay(panel, type),
                  targets: panel.targets.map((t) =>
                    t.id === target.id ? query : t,
                  ),
                });
              }}
            />
          </Field>
          <Field label={t("dashboards.mode")} requirement="required">
            <Select
              value={panel.authoring_mode}
              options={[
                { value: "builder", label: t("dashboards.builder") },
                { value: "dsl", label: t("dashboards.dsl") },
              ]}
              onValueChange={(v) => void convert(v as "builder" | "dsl")}
            />
          </Field>
        </div>
        {!isNew && (
          <p className="argus-dashboard-muted">
            {t("dashboards.conversionHint")}
          </p>
        )}
        <div className="argus-dashboard-inline">
          <Select
            ariaLabel={t("dashboards.queryTarget")}
            value={target.id}
            options={panel.targets.map((target) => ({
              value: target.id,
              label: target.id,
            }))}
            onValueChange={setTargetId}
          />
          <Button
            disabled={panel.targets.length >= 8}
            onClick={() => {
              const value = {
                ...newTarget(
                  panel.signal,
                  panel.type,
                  panel.authoring_mode as "builder" | "dsl",
                ),
                id: `query_${crypto.randomUUID().replaceAll("-", "").slice(0, 8)}`,
              };
              patch({ targets: [...panel.targets, value] });
              setTargetId(value.id);
            }}
          >
            {t("dashboards.addQuery")}
          </Button>
          <Button
            variant="ghost"
            disabled={panel.targets.length === 1}
            onClick={() => {
              patch({
                targets: panel.targets.filter((t) => t.id !== target.id),
              });
              setTargetId(panel.targets.find((t) => t.id !== target.id)!.id);
            }}
          >
            {t("dashboards.delete")}
          </Button>
        </div>
        <QueryTargetEditor
          panel={panel}
          spec={spec}
          target={target}
          onChange={(value) => patchTarget(value)}
        />
        <LocalFiltersEditor
          panel={panel}
          spec={spec}
          onChange={(local_filters) => patch({ local_filters })}
        />
        <div className="argus-dashboard-form-grid">
          <Field label={t("dashboards.unit")} requirement="optional">
            <Select
              value={panel.unit}
              options={[
                { value: "", label: "—" },
                ...[
                  "percent",
                  "percent_ratio",
                  "bytes",
                  "s",
                  "ms",
                  "requests",
                ].map((value) => ({
                  value,
                  label: t(`dashboards.units.${value}`),
                })),
              ]}
              onValueChange={(unit) => patch({ unit })}
            />
          </Field>
          <Field label={t("dashboards.decimals")} requirement="required">
            <Input
              type="number"
              min={0}
              max={8}
              value={panel.decimals}
              onChange={(e) => patch({ decimals: Number(e.target.value) })}
            />
          </Field>
        </div>
        <label className="argus-dashboard-check">
          <input
            type="checkbox"
            checked={panel.legend}
            onChange={(e) => patch({ legend: e.target.checked })}
          />
          {t("dashboards.legend")}
        </label>
        <DrilldownsEditor panel={panel} spec={spec} onChange={patch} />
        <DisplayEditor panel={panel} onChange={patch} />
        {error && <p role="alert">{error}</p>}
      </fieldset>
      <ConfirmDialog
        open={Boolean(replace)}
        onOpenChange={(open) => !open && setReplace(null)}
        title={t("dashboards.newQuery")}
        description={t("dashboards.reconfigureHint")}
        onConfirm={() => replace && replaceQuery(replace.signal, replace.mode)}
      />
    </FormDrawer>
  );
}
