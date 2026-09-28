import { useTranslation } from "react-i18next";
import { Alert } from "@argus/ui";
import type { PendingActionPublic } from "@argus/api-client";
const object = (value: unknown): Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
const decoded = (value: unknown): Record<string, unknown> => {
  if (typeof value !== "string") return object(value);
  try {
    return object(JSON.parse(value));
  } catch {
    return {};
  }
};
export function PublicationReview({ action }: { action: PendingActionPublic }) {
  const { t } = useTranslation(),
    preview = object(action.preview),
    before = decoded(preview.before_json),
    after = decoded(preview.after_json),
    validation = object(preview.validation),
    sample = decoded(preview.sample_json),
    execution = object(sample.execution);
  const mock = preview.validation === "mock_structure_only";
  const panels = (Array.isArray(execution.panels) ? execution.panels : []).map(
    object,
  );
  const detailStates = object(sample.details_validation);
  return (
    <div className="argus-dashboard-form-stack">
      <Alert
        tone={mock || sample.status !== "success" ? "warning" : "info"}
        title={t("dashboards.sample")}
        description={
          mock
            ? t("dashboards.mock")
            : `${t("dashboards.configuration")}: ${validation.valid === true ? t("dashboards.validConfiguration") : t("dashboards.failed")} · ${t("dashboards.resultStatus")}: ${t(`dashboards.statuses.${String(sample.status ?? "unavailable")}`, { defaultValue: String(sample.status ?? "unavailable") })}. ${t("dashboards.sampleHint")}`
        }
      />
      {panels.length > 0 && (
        <details>
          <summary>{t("dashboards.resultStatus")}</summary>
          {panels.map((panel) => (
            <p key={String(panel.id)}>
              {String(panel.id)}:{" "}
              {t(`dashboards.statuses.${String(panel.status)}`, {
                defaultValue: String(panel.status),
              })}
            </p>
          ))}
        </details>
      )}
      {Object.keys(detailStates).length > 0 && (
        <p className="argus-dashboard-muted">
          {t("dashboards.detailSelectionRequired", {
            count: Object.keys(detailStates).length,
          })}
        </p>
      )}
      <div className="argus-dashboard-diff">
        {[
          { label: t("dashboards.currentPublished"), value: before },
          { label: t("dashboards.myChanges"), value: after },
        ].map(({ label, value }) => (
          <div key={label}>
            <h3>{label}</h3>
            <ConfigurationReview value={value} />
          </div>
        ))}
      </div>
    </div>
  );
}
function ConfigurationReview({ value }: { value: Record<string, unknown> }) {
  const { t } = useTranslation();
  const spec = object(value.spec ?? value);
  const panels = (Array.isArray(spec.panels) ? spec.panels : []).map(object);
  return (
    <>
      <p>{typeof value.name === "string" ? value.name : "—"}</p>
      <p>
        {t("dashboards.descriptionField")}:{" "}
        {typeof value.description === "string" && value.description
          ? value.description
          : "—"}
      </p>
      {panels.map((panel) => (
        <details key={String(panel.id)}>
          <summary>
            {String(panel.title)} ·{" "}
            {t(`dashboards.charts.${String(panel.type)}`)}
          </summary>
          <pre>{JSON.stringify(panel, null, 2)}</pre>
        </details>
      ))}
      <details>
        <summary>{t("dashboards.filtersTab")}</summary>
        <pre>
          {JSON.stringify(
            {
              default_time_range: spec.default_time_range,
              default_refresh_seconds: spec.default_refresh_seconds,
              variables: spec.variables,
              folder_id: value.folder_id,
              bindings: value.bindings,
            },
            null,
            2,
          )}
        </pre>
      </details>
    </>
  );
}
