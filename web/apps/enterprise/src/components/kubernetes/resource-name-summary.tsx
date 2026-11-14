import { useTranslation } from "react-i18next";
import type { KubernetesInventoryObject } from "@argus/api-client";
import { HoverSummaryButton } from "@argus/ui";
import {
  objectRecord,
  resourceHoverFields,
  formatResourceField,
} from "./resource-model";

export function ResourceNameSummary({
  object,
  onPress,
  isSummaryDisabled,
}: {
  object: KubernetesInventoryObject;
  onPress: () => void;
  isSummaryDisabled?: boolean;
}) {
  const { t, i18n } = useTranslation();
  const fields = (resourceHoverFields[object.definition.key] ?? [])
    .filter((key) => key !== "images")
    .flatMap((key) => {
      const value = object.summary?.fields[key];
      return value
        ? [
            {
              label: t("kubernetesResources.fields." + key, {
                defaultValue: key,
              }),
              value: formatResourceField(key, value),
              copyable: false,
            },
          ]
        : [];
    });
  fields.push({
    label: t("kubernetesResources.age"),
    value: new Date(object.created_at).toLocaleString(i18n.language),
    copyable: false,
  });
  let spec = objectRecord(object.data.spec);
  if (object.definition.key === "cronjob")
    spec = objectRecord(objectRecord(spec.jobTemplate).spec);
  if (object.definition.key !== "pod")
    spec = objectRecord(objectRecord(spec.template).spec);
  for (const key of ["containers", "initContainers", "ephemeralContainers"]) {
    const containers = spec[key];
    if (Array.isArray(containers))
      for (const value of containers) {
        const container = objectRecord(value);
        if (typeof container.image === "string")
          fields.push({
            label:
              t("kubernetesResources.fields.images") +
              (typeof container.name === "string"
                ? ` · ${container.name}`
                : ""),
            value: container.image,
            copyable: true,
          });
      }
  }
  return (
    <HoverSummaryButton
      label={object.name}
      fields={fields}
      onPress={onPress}
      isSummaryDisabled={isSummaryDisabled}
      className="argus-k8s-resource-name-link"
    />
  );
}
