import { suggestWorkloadSelector } from "./resource-create";
import { Field, Input, StructuredFieldsEditor, Button } from "@argus/ui";
import { useTranslation } from "react-i18next";
import type { KubernetesInventoryObject } from "@argus/api-client";
import { resourceFormSchema } from "./resource-form-schema";
import { asRecord, type JsonRecord } from "./resource-patch";
export function ResourceActionFields({
  kind,
  intent,
  object,
  draft,
  onChange,
  capacity,
  onCapacity,
  disabled,
  onAdvanced,
}: {
  kind: string;
  intent: string;
  object?: KubernetesInventoryObject;
  draft: JsonRecord;
  onChange: (value: JsonRecord) => void;
  capacity: string;
  onCapacity: (value: string) => void;
  disabled: boolean;
  onAdvanced: () => void;
}) {
  const { t } = useTranslation();
  if (intent === "suspend" || intent === "resume")
    return (
      <p>
        {t(
          kind === "cronjob"
            ? "kubernetesResources.scheduleHint"
            : "kubernetesResources.preflight",
        )}
      </p>
    );
  if (intent === "expand")
    return (
      <>
        <p>{t("kubernetesResources.volumeHint")}</p>
        <Field
          label={t("kubernetesResources.targetCapacity")}
          requirement="required"
        >
          <Input
            value={capacity}
            disabled={disabled}
            onChange={(event) => onCapacity(event.target.value)}
            required
          />
        </Field>
      </>
    );
  let fields = resourceFormSchema(kind, t);
  if (object)
    fields = fields.map((field) =>
      field.key === "metadata"
        ? {
            ...field,
            fields: field.fields?.filter((child) => child.key !== "name"),
          }
        : field,
    );
  if (intent === "image") {
    const containers = asRecord(
      asRecord(asRecord(draft.spec).template).spec,
    ).containers;
    return (
      <div className="argus-dashboard-form-stack">
        {(Array.isArray(containers) ? containers : []).map((row, index) => {
          const container = asRecord(row);
          return (
            <Field
              key={String(container.name)}
              label={String(container.name)}
              requirement="required"
            >
              <Input
                required
                value={String(container.image ?? "")}
                disabled={disabled}
                onChange={(event) => {
                  const copy = structuredClone(draft);
                  const items = asRecord(
                    asRecord(asRecord(copy.spec).template).spec,
                  ).containers as Record<string, unknown>[];
                  items[index] = { ...items[index], image: event.target.value };
                  onChange(copy);
                }}
              />
            </Field>
          );
        })}
      </div>
    );
  }
  if (intent === "schedule")
    fields = fields
      .filter((field) => field.key === "spec")
      .map((field) => ({
        ...field,
        fields: field.fields?.filter((child) =>
          [
            "schedule",
            "timeZone",
            "concurrencyPolicy",
            "successfulJobsHistoryLimit",
            "failedJobsHistoryLimit",
          ].includes(child.key),
        ),
      }));
  return (
    <>
      <StructuredFieldsEditor
        fields={fields}
        value={draft}
        onChange={(value) =>
          onChange(object ? value : suggestWorkloadSelector(draft, value, kind))
        }
        disabled={disabled}
      />
      <Button variant="ghost" onPress={onAdvanced}>
        {t("kubernetesResources.useAdvanced")}
      </Button>
    </>
  );
}
