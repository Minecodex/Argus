import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Trash2 } from "lucide-react";
import type { DashboardSpec, DashboardVariable } from "@argus/api-client";
import { Badge, Button, Field, FormDrawer, Input } from "@argus/ui";
import { useDefinitionForm } from "./use-definition-form";
import { dependentPanels } from "./execution-dependencies";
import { VariableQueryEditor } from "./variable-query-editor";
import "../../styles/dashboard-variables.css";

export function VariablesEditor({
  spec,
  onClose,
  onSave,
}: {
  spec: DashboardSpec;
  onClose: () => void;
  onSave: (spec: DashboardSpec) => void;
}) {
  const { t } = useTranslation();
  const [value, setValue] = useState(() => structuredClone(spec));
  const [activeId, setActiveId] = useState(spec.variables[0]?.id);
  const active = value.variables.find((v) => v.id === activeId);
  const form = useDefinitionForm(
    value,
    (s) =>
      s.variables.every(
        (v) =>
          /^[A-Za-z_][A-Za-z0-9_]{0,63}$/.test(v.name) &&
          Boolean(v.label.trim()) &&
          Boolean(v.query.field.trim()) &&
          (v.default.all || v.default.values.length > 0),
      ) && new Set(s.variables.map((v) => v.name)).size === s.variables.length,
    t("dashboards.editor.invalidForm"),
  );
  const update = (patch: Partial<DashboardVariable>) =>
    setValue((s) => ({
      ...s,
      variables: s.variables.map((v) =>
        v.id === activeId ? { ...v, ...patch } : v,
      ),
    }));
  const add = () => {
    let index = value.variables.length + 1;
    while (value.variables.some((v) => v.name === `variable_${index}`)) index++;
    const variable: DashboardVariable = {
      id: crypto.randomUUID(),
      name: `variable_${index}`,
      label: `variable_${index}`,
      multiple: false,
      include_all: true,
      default: { all: true, values: [] },
      query: {
        signal: "metrics",
        source_binding: {
          source_type: "hostmetrics",
          capability_version: "v1",
        },
        field: "",
        filters: [],
      },
    };
    setValue({ ...value, variables: [...value.variables, variable] });
    setActiveId(variable.id);
  };
  const affected = active ? dependentPanels(value, [active.name]) : [];
  return (
    <FormDrawer
      open
      width={960}
      title={t("dashboardControls.filterManager")}
      description={t("dashboardControls.variablesHint")}
      submitLabel={t("dashboards.done")}
      onOpenChange={(open) => !open && onClose()}
      onSubmit={form.handleSubmit(onSave)}
    >
      {form.error && (
        <p role="alert" className="argus-field__hint is-error">
          {form.error}
        </p>
      )}
      <div className="argus-variable-workspace">
        <aside
          className="argus-variable-list"
          aria-label={t("dashboardControls.variableList")}
        >
          <div className="argus-variable-list__heading">
            <strong>{t("dashboardControls.filterManager")}</strong>
            <Badge>{value.variables.length}</Badge>
          </div>
          {value.variables.map((v) => (
            <Button
              key={v.id}
              layout="content"
              className="argus-variable-list__item"
              aria-pressed={v.id === activeId}
              onPress={() => setActiveId(v.id)}
            >
              <span>
                <strong>{v.label || v.name}</strong>
                <small>${v.name}</small>
              </span>
            </Button>
          ))}
          <Button onPress={add}>
            <Plus aria-hidden />
            {t("dashboards.addVariable")}
          </Button>
        </aside>
        <div className="argus-variable-editor">
          {active ? (
            <>
              <div className="argus-variable-editor__heading">
                <strong>{t("dashboardControls.editVariable")}</strong>
                <Button
                  isIconOnly
                  variant="ghost"
                  aria-label={t("dashboardControls.removeVariable")}
                  onPress={() => {
                    const variables = value.variables.filter(
                      (v) => v.id !== activeId,
                    );
                    setValue({ ...value, variables });
                    setActiveId(variables[0]?.id);
                  }}
                >
                  <Trash2 aria-hidden />
                </Button>
              </div>
              <div className="argus-dashboard-form-grid">
                <Field label={t("dashboards.label")} requirement="required">
                  <Input
                    value={active.label}
                    onChange={(e) => update({ label: e.target.value })}
                  />
                </Field>
                <Field
                  label={t("dashboards.variableName")}
                  requirement="required"
                >
                  <Input
                    value={active.name}
                    pattern="[A-Za-z_][A-Za-z0-9_]{0,63}"
                    onChange={(e) => update({ name: e.target.value })}
                  />
                </Field>
              </div>
              <VariableQueryEditor
                key={active.id}
                value={active}
                spec={value}
                onChange={update}
              />
              <section className="argus-variable-consumers">
                <strong>{t("dashboardControls.consumers")}</strong>
                <p className="argus-dashboard-muted">
                  {t(
                    affected.length
                      ? "dashboardControls.consumersHint"
                      : "dashboardControls.noConsumers",
                  )}
                </p>
                <div className="argus-dashboard-inline">
                  {value.panels
                    .filter((p) => affected.includes(p.id))
                    .map((p) => (
                      <Badge key={p.id}>{p.title}</Badge>
                    ))}
                </div>
              </section>
            </>
          ) : (
            <p className="argus-dashboard-muted">
              {t("dashboardControls.emptyVariables")}
            </p>
          )}
        </div>
      </div>
    </FormDrawer>
  );
}
