import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { DashboardSpec } from "@argus/api-client";
import { Field, FormDialog, Input } from "@argus/ui";
import { TimeRangePicker } from "./time-range-picker";
import { useDefinitionForm } from "./use-definition-form";

export function DashboardSettingsEditor({
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
  const form = useDefinitionForm(
    value,
    (s) =>
      Number.isInteger(s.default_refresh_seconds) &&
      (s.default_refresh_seconds === 0 || s.default_refresh_seconds >= 5),
    t("dashboardControls.invalidRefresh"),
  );
  return (
    <FormDialog
      open
      title={t("dashboardControls.settings")}
      description={t("dashboardControls.defaultsHint")}
      submitLabel={t("dashboards.done")}
      onOpenChange={(open) => !open && onClose()}
      onSubmit={form.handleSubmit(onSave)}
    >
      <Field label={t("dashboards.defaultTime")} requirement="required">
        <TimeRangePicker
          label={t("dashboards.defaultTime")}
          value={value.default_time_range}
          onChange={(range) =>
            setValue({ ...value, default_time_range: range })
          }
        />
      </Field>
      <Field
        label={t("dashboards.defaultRefresh")}
        requirement="required"
        hint={t("dashboardControls.refreshHint")}
      >
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
      {form.error && (
        <p role="alert" className="argus-field__hint is-error">
          {form.error}
        </p>
      )}
      <p className="argus-dashboard-muted">
        {t("dashboardControls.defaultResourcesHint")}
      </p>
    </FormDialog>
  );
}
