import { useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Field, Input } from "@argus/ui";
import type { CollectorImageValues } from "./collector-image-form";

export function CollectorImageFields({
  form,
  defaultImage,
}: {
  form: UseFormReturn<CollectorImageValues>;
  defaultImage?: string;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Field
        label={t("kubernetes.collector.wizard.imageAddress")}
        requirement="optional"
        hint={t("kubernetes.collector.wizard.imageAddressDesc")}
        error={form.formState.errors.image?.message}
      >
        <Input {...form.register("image")} placeholder={defaultImage ?? ""} />
      </Field>
      <Field
        label={t("kubernetes.collector.wizard.imagePullSecrets")}
        requirement="optional"
        hint={t("kubernetes.collector.wizard.imagePullSecretsDesc")}
        error={form.formState.errors.pullSecrets?.message}
      >
        <Input
          {...form.register("pullSecrets")}
          placeholder={t(
            "kubernetes.collector.wizard.imagePullSecretsPlaceholder",
          )}
        />
      </Field>
    </>
  );
}
