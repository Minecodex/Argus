import { useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  DashboardPanel,
  DashboardSpec,
  DashboardSchemas,
  DashboardTarget,
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
import { isDashboardSignal, newTarget, sources } from "./model";

export function removeDrilldownGraph(panel: DashboardPanel, id: string) {
  const removed = panel.drilldowns.find((d) => d.id === id);
  if (!removed)
    return {
      drilldowns: panel.drilldowns,
      detail_query_targets: panel.detail_query_targets,
    };
  const candidates = new Set([removed.detail_query_ref]);
  for (let size = -1; size !== candidates.size;) {
    size = candidates.size;
    for (const drill of panel.drilldowns)
      if (candidates.has(drill.origin_query_ref))
        candidates.add(drill.detail_query_ref);
  }
  const drills = panel.drilldowns.filter((d) => d.id !== id),
    kept = new Set(panel.targets.map((t) => t.id));
  for (const drill of drills)
    if (!candidates.has(drill.origin_query_ref))
      kept.add(drill.origin_query_ref);
  for (let size = -1; size !== kept.size;) {
    size = kept.size;
    for (const drill of drills)
      if (kept.has(drill.origin_query_ref)) kept.add(drill.detail_query_ref);
  }
  const deleted = new Set([...candidates].filter((id) => !kept.has(id)));
  return {
    drilldowns: drills.filter((d) => !deleted.has(d.origin_query_ref)),
    detail_query_targets: panel.detail_query_targets.filter(
      (t) => !deleted.has(t.id),
    ),
  };
}
export function DrilldownsEditor({
  panel,
  spec,
  onChange,
}: {
  panel: DashboardPanel;
  spec: DashboardSpec;
  onChange: (panel: Partial<DashboardPanel>) => void;
}) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState<string>(),
    [removing, setRemoving] = useState<string>();
  const target = panel.detail_query_targets.find(
    (target) => target.id === editing,
  );
  const patch = (
    id: string,
    value: Partial<DashboardSchemas["DashboardDrilldown"]>,
  ) =>
    onChange({
      drilldowns: panel.drilldowns.map((d) =>
        d.id === id ? { ...d, ...value } : d,
      ),
    });
  const targets = [...panel.targets, ...panel.detail_query_targets];
  return (
    <details>
      <summary>
        {t("dashboards.details")} ({panel.drilldowns.length})
      </summary>
      <p className="argus-dashboard-muted">
        {t("dashboards.detailEditorHint")}
      </p>
      <div className="argus-dashboard-form-stack">
        {panel.drilldowns.map((drill) => (
          <section key={drill.id} className="argus-dashboard-editor-section">
            <div className="argus-dashboard-form-grid">
              <Field label={t("dashboards.detailTitle")} requirement="optional">
                <Input
                  value={drill.title}
                  placeholder={t(`dashboards.drillKinds.${drill.kind}`, {
                    defaultValue: drill.id,
                  })}
                  onChange={(e) => patch(drill.id, { title: e.target.value })}
                />
              </Field>
              <Field label={t("dashboards.originQuery")} requirement="required">
                <Select
                  value={drill.origin_query_ref}
                  options={targets
                    .filter((target) => target.id !== drill.detail_query_ref)
                    .map((target) => ({ value: target.id, label: target.id }))}
                  onValueChange={(origin_query_ref) =>
                    patch(drill.id, { origin_query_ref })
                  }
                />
              </Field>
              <Field label={t("dashboards.detailQuery")} requirement="required">
                <Select
                  value={drill.detail_query_ref}
                  options={panel.detail_query_targets.map((target) => ({
                    value: target.id,
                    label: target.id,
                  }))}
                  onValueChange={(detail_query_ref) =>
                    patch(drill.id, { detail_query_ref })
                  }
                />
              </Field>
              <Field label={t("dashboards.detailScope")} requirement="required">
                <Select
                  value={drill.scope_policy}
                  options={[
                    { value: "inherit", label: t("dashboards.inheritScope") },
                    {
                      value: "authorized_trace",
                      label: t("dashboards.fullTrace"),
                    },
                  ]}
                  onValueChange={(scope_policy) =>
                    patch(drill.id, { scope_policy })
                  }
                />
              </Field>
            </div>
            {drill.scope_policy === "authorized_trace" && (
              <p className="argus-dashboard-muted">
                {t("dashboards.fullScope")}
              </p>
            )}
            <div className="argus-dashboard-form-stack">
              {Object.entries(drill.inputs).map(([name, pointer], index) => (
                <div className="argus-dashboard-inline" key={index}>
                  <Input
                    aria-label={t("dashboards.rowInput")}
                    value={name}
                    pattern="[A-Za-z_][A-Za-z0-9_]*"
                    required
                    onChange={(e) =>
                      patch(drill.id, {
                        inputs: Object.fromEntries(
                          Object.entries(drill.inputs).map(
                            ([key, value], i) => [
                              i === index ? e.target.value : key,
                              value,
                            ],
                          ),
                        ),
                      })
                    }
                  />
                  <Input
                    aria-label={t("dashboards.rowPointer")}
                    value={pointer}
                    required
                    onChange={(e) =>
                      patch(drill.id, {
                        inputs: { ...drill.inputs, [name]: e.target.value },
                      })
                    }
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() =>
                      patch(drill.id, {
                        inputs: Object.fromEntries(
                          Object.entries(drill.inputs).filter(
                            ([key]) => key !== name,
                          ),
                        ),
                      })
                    }
                  >
                    {t("dashboards.delete")}
                  </Button>
                </div>
              ))}
            </div>
            <div className="argus-dashboard-inline">
              <Button
                size="sm"
                onClick={() =>
                  patch(drill.id, {
                    inputs: {
                      ...drill.inputs,
                      [`input_${Object.keys(drill.inputs).length + 1}`]: "/",
                    },
                  })
                }
              >
                {t("dashboards.addRowInput")}
              </Button>
              <Button
                size="sm"
                onClick={() => setEditing(drill.detail_query_ref)}
              >
                {t("dashboards.editDetailQuery")}
              </Button>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setRemoving(drill.id)}
              >
                {t("dashboards.delete")}
              </Button>
            </div>
            <details>
              <summary>{t("dashboards.detailWindow")}</summary>
              <label className="argus-dashboard-check">
                <input
                  type="checkbox"
                  checked={Boolean(drill.time_window)}
                  onChange={(e) =>
                    patch(drill.id, {
                      time_window: e.target.checked
                        ? { input: "timestamp", seconds: 60 }
                        : undefined,
                    })
                  }
                />
                {t("dashboards.narrowWindow")}
              </label>
              {drill.time_window && (
                <div className="argus-dashboard-form-grid">
                  <Field
                    label={t("dashboards.windowStartInput")}
                    requirement="required"
                  >
                    <Input
                      value={drill.time_window.input}
                      onChange={(e) =>
                        patch(drill.id, {
                          time_window: {
                            ...drill.time_window!,
                            input: e.target.value,
                          },
                        })
                      }
                    />
                  </Field>
                  <Field label={t("dashboards.window")} requirement="optional">
                    <Input
                      type="number"
                      min={1}
                      max={604800}
                      value={drill.time_window.seconds ?? ""}
                      onChange={(e) =>
                        patch(drill.id, {
                          time_window: {
                            ...drill.time_window!,
                            seconds: e.target.value
                              ? Number(e.target.value)
                              : undefined,
                            duration_input: undefined,
                          },
                        })
                      }
                    />
                  </Field>
                  <Field
                    label={t("dashboards.windowDurationInput")}
                    requirement="optional"
                  >
                    <Input
                      value={drill.time_window.duration_input ?? ""}
                      onChange={(e) =>
                        patch(drill.id, {
                          time_window: {
                            ...drill.time_window!,
                            duration_input: e.target.value || undefined,
                            seconds: undefined,
                          },
                        })
                      }
                    />
                  </Field>
                </div>
              )}
            </details>
          </section>
        ))}
        <Button
          size="sm"
          disabled={
            panel.detail_query_targets.length >= 16 ||
            panel.drilldowns.length >= 64
          }
          onClick={() => {
            const id = `detail_${crypto.randomUUID().replaceAll("-", "").slice(0, 12)}`;
            const target = {
              ...structuredClone(panel.targets[0]!),
              id,
              signal: panel.signal,
              source_binding: panel.source_binding,
            };
            onChange({
              detail_query_targets: [...panel.detail_query_targets, target],
              drilldowns: [
                ...panel.drilldowns,
                {
                  id: `open_${id}`,
                  title: "",
                  kind: "inspect",
                  origin_query_ref: panel.targets[0]!.id,
                  detail_query_ref: id,
                  scope_policy: "inherit",
                  inputs: {},
                },
              ],
            });
          }}
        >
          {t("dashboards.addDetail")}
        </Button>
      </div>
      {target && (
        <DetailTargetEditor
          key={target.id}
          target={target}
          panel={panel}
          spec={spec}
          onClose={() => setEditing(undefined)}
          onSave={(value) => {
            onChange({
              detail_query_targets: panel.detail_query_targets.map((t) =>
                t.id === value.id ? value : t,
              ),
            });
            setEditing(undefined);
          }}
        />
      )}
      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={(open) => !open && setRemoving(undefined)}
        title={t("dashboards.removeDetail")}
        description={t("dashboards.removeDetailHint")}
        danger
        onConfirm={() => {
          if (removing) onChange(removeDrilldownGraph(panel, removing));
          setRemoving(undefined);
        }}
      />
    </details>
  );
}
function DetailTargetEditor({
  target: initial,
  panel,
  spec,
  onClose,
  onSave,
}: {
  target: DashboardTarget;
  panel: DashboardPanel;
  spec: DashboardSpec;
  onClose: () => void;
  onSave: (target: DashboardTarget) => void;
}) {
  const { t } = useTranslation(),
    [target, setTarget] = useState(() => structuredClone(initial)),
    [signal, setSignal] = useState<string>();
  const detailSignal = target.signal ?? panel.signal;
  const context = {
    ...panel,
    signal: isDashboardSignal(detailSignal) ? detailSignal : panel.signal,
    source_binding: target.source_binding!,
  };
  const inputs = Array.from(
    new Set(
      panel.drilldowns
        .filter((d) => d.detail_query_ref === target.id)
        .flatMap((d) => Object.keys(d.inputs)),
    ),
  );
  return (
    <FormDrawer
      open
      width={900}
      title={t("dashboards.editDetailQuery")}
      onOpenChange={(open) => !open && onClose()}
      submitLabel={t("dashboards.done")}
      onSubmit={() => onSave(target)}
    >
      <div className="argus-dashboard-form-stack">
        <p className="argus-dashboard-muted">
          {target.id} ·{" "}
          {t(
            panel.authoring_mode === "builder"
              ? "dashboards.builder"
              : "dashboards.dsl",
          )}
        </p>
        <div className="argus-dashboard-form-grid">
          <Field label={t("dashboards.signal")} requirement="required">
            <Select
              value={target.signal!}
              options={["metrics", "logs", "traces"].map((value) => ({
                value,
                label: value,
              }))}
              onValueChange={setSignal}
            />
          </Field>
          <Field label={t("dashboards.source")} requirement="required">
            <Select
              value={target.source_binding!.source_type}
              options={sources[target.signal as keyof typeof sources].map(
                (value) => ({ value, label: value }),
              )}
              onValueChange={(source_type) =>
                setTarget({
                  ...target,
                  source_binding: { source_type, capability_version: "v1" },
                })
              }
            />
          </Field>
        </div>
        <QueryTargetEditor
          panel={context}
          spec={spec}
          target={target}
          onChange={setTarget}
          rowInputs={inputs}
          detail
        />
      </div>
      <ConfirmDialog
        open={Boolean(signal)}
        onOpenChange={(open) => !open && setSignal(undefined)}
        title={t("dashboards.newQuery")}
        description={t("dashboards.replaceDetailHint")}
        onConfirm={() => {
          if (signal)
            setTarget({
              ...newTarget(
                signal,
                signal === "traces" ? "trace_list" : "table",
                panel.authoring_mode as "builder" | "dsl",
              ),
              id: target.id,
              signal,
              source_binding: {
                source_type: sources[signal as keyof typeof sources][0]!,
                capability_version: "v1",
              },
            });
          setSignal(undefined);
        }}
      />
    </FormDrawer>
  );
}
