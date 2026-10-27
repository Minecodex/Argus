import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type MonitoringSchemas,
  type PendingActionPublic,
} from "@argus/api-client";
import { Dialog, Field, Input, Textarea, Button } from "@argus/ui";

const schema = z.object({
  serverName: z
    .string()
    .max(253, { message: "builtinCollections.trust.invalidServerName" }),
  caPem: z
    .string()
    .max(262144, { message: "builtinCollections.trust.invalidCA" }),
});

export function ComponentTrustDialog({
  row,
  kind,
  id,
  onClose,
  onPreview,
}: {
  row: MonitoringSchemas["MonitoringBuiltin"];
  kind: "host" | "kubernetes_cluster";
  id: string;
  onClose: () => void;
  onPreview: (action: PendingActionPublic) => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const config = (row.collection?.config ?? {}) as {
    interval?: string;
    component_tls?: { server_name?: string; ca_pem?: string };
  };
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      serverName: config.component_tls?.server_name ?? "",
      caPem: config.component_tls?.ca_pem ?? "",
    },
  });
  const [error, setError] = useState("");
  const submit = form.handleSubmit(async (values) => {
    setError("");
    try {
      const action = await api.monitoring.previewBuiltin({
        target: { kind, id },
        key: row.definition.key,
        expected_version: row.collection?.version ?? 0,
        enabled: row.collection?.desired_enabled ?? false,
        config: {
          interval: config.interval ?? "30s",
          component_tls: {
            server_name: values.serverName.trim(),
            ca_pem: values.caPem,
          },
        },
      });
      onPreview(action);
    } catch (cause) {
      setError(
        formatApiError(cause, t("monitoring.errors.invalid"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    }
  });
  return (
    <Dialog
      open
      size="lg"
      title={t("builtinCollections.trust.title")}
      onOpenChange={(open) => {
        if (!open && !form.formState.isSubmitting) onClose();
      }}
    >
      <form className="argus-component-trust-form" onSubmit={submit}>
        <p>{t("builtinCollections.trust.hint")}</p>
        <Field
          label={t("builtinCollections.trust.serverName")}
          requirement="optional"
          hint={t("builtinCollections.trust.serverNameHint")}
          error={
            form.formState.errors.serverName?.message
              ? t(form.formState.errors.serverName.message)
              : undefined
          }
        >
          <Input {...form.register("serverName")} />
        </Field>
        <Field
          label={t("builtinCollections.trust.ca")}
          requirement="optional"
          hint={t("builtinCollections.trust.caHint")}
          error={
            form.formState.errors.caPem?.message
              ? t(form.formState.errors.caPem.message)
              : undefined
          }
        >
          <Textarea rows={8} {...form.register("caPem")} />
        </Field>
        {error && <p role="alert">{error}</p>}
        <div className="argus-form-actions">
          <Button
            type="button"
            onPress={onClose}
            isDisabled={form.formState.isSubmitting}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            variant="primary"
            isDisabled={form.formState.isSubmitting}
          >
            {t("builtinCollections.trust.preview")}
          </Button>
        </div>
      </form>
    </Dialog>
  );
}
