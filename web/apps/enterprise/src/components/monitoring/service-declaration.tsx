import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Field, FormDialog, Input } from "@argus/ui";
import type { MonitoringMutation } from "@argus/api-client";

export function ServiceDeclaration({
  applicationId,
  busy,
  onClose,
  onSave,
}: {
  applicationId: string;
  busy: boolean;
  onClose: () => void;
  onSave: (mutation: MonitoringMutation) => Promise<boolean>;
}) {
  const { t } = useTranslation(),
    [name, setName] = useState(""),
    [namespace, setNamespace] = useState("");
  return (
    <FormDialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("monitoring.createService")}
      loading={busy}
      submitLabel={t("monitoring.preview")}
      onSubmit={() => {
        void onSave({
          kind: "service",
          operation: "create",
          service: {
            application_id: applicationId,
            service_name: name.trim(),
            service_namespace: namespace.trim(),
          },
        }).then((saved) => {
          if (saved) onClose();
        });
      }}
    >
      <p>{t("monitoring.declaredServiceHint")}</p>
      <Field label={t("monitoring.serviceName")} requirement="required">
        <Input
          value={name}
          onChange={(event) => setName(event.target.value)}
          required
          maxLength={200}
        />
      </Field>
      <Field label={t("monitoring.serviceNamespace")} requirement="optional">
        <Input
          value={namespace}
          onChange={(event) => setNamespace(event.target.value)}
          maxLength={200}
        />
      </Field>
    </FormDialog>
  );
}
