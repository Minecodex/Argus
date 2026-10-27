import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type MonitoringCollection,
  type MonitoringIntegration,
  type MonitoringMutation,
} from "@argus/api-client";
import { Button, Field, FormDialog, Input } from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { useMonitoringActions } from "./actions";
import { CollectionForm } from "./collection-form";
import { LogCollectionForm } from "./log-collection-form";

export function MonitoringHostForm({
  data,
}: {
  data: Record<string, unknown>;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    canManage = usePermission("monitoring.manage");
  const actions = useMonitoringActions();
  const [opened, setOpened] = useState(false),
    [snapshot, setSnapshot] = useState<MonitoringCollection>();
  const [loading, setLoading] = useState(false),
    [error, setError] = useState("");
  const integrationKey =
    typeof data.integration === "string" ? data.integration : "";
  const collectionId =
    typeof data.collection_id === "string" ? data.collection_id : undefined;
  const catalog = useQuery({
    queryKey: ["monitoring", "integrations"],
    queryFn: () => api.monitoring.integrations(),
    enabled: canManage,
  });
  const integration = catalog.data?.find(
    (entry) =>
      entry.key === integrationKey &&
      entry.adapter_ready &&
      entry.selection === "candidate",
  );
  const open = async () => {
    if (!integration || !canManage) return;
    setLoading(true);
    setError("");
    try {
      const current = collectionId
        ? await api.monitoring.collection(collectionId)
        : undefined;
      if (
        current &&
        (current.integration !== integrationKey || current.status !== "active")
      )
        throw new Error(t("monitoring.hostForm.unavailable"));
      setSnapshot(current);
      setOpened(true);
    } catch (cause) {
      setError(
        formatApiError(
          cause,
          t("monitoring.hostForm.unavailable"),
          (requestId) => t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setLoading(false);
    }
  };
  const close = () => {
    setOpened(false);
    setSnapshot(undefined);
  };
  const submit = async (input: MonitoringMutation) => {
    const accepted = await actions.preview(input);
    if (accepted) close();
    return accepted;
  };
  const prefill =
    data.prefill &&
    typeof data.prefill === "object" &&
    !Array.isArray(data.prefill)
      ? (data.prefill as Record<string, unknown>)
      : {};
  const initial = {
    name: typeof prefill.name === "string" ? prefill.name : undefined,
    endpoint:
      typeof prefill.endpoint === "string" && prefill.endpoint
        ? prefill.endpoint
        : undefined,
    database:
      typeof prefill.database === "string" ? prefill.database : undefined,
  };
  return (
    <>
      <p>{t("monitoring.hostForm.hint")}</p>
      <Button
        isDisabled={!integration || !canManage || loading}
        onPress={() => void open()}
      >
        {t("monitoring.hostForm.open")}
      </Button>
      {(error || catalog.isError) && (
        <p role="alert">{error || t("monitoring.hostForm.unavailable")}</p>
      )}
      {opened &&
        canManage &&
        integration &&
        (snapshot ? (
          <CredentialForm
            collection={snapshot}
            integration={integration}
            busy={actions.busy}
            error={actions.error}
            onSubmit={submit}
            onClose={close}
          />
        ) : ["filelog", "journald", "eventlog", "k8slog"].includes(
            integration.key,
          ) ? (
          <LogCollectionForm
            integration={integration}
            busy={actions.busy}
            error={actions.error}
            onSubmit={submit}
            onClose={close}
          />
        ) : (
          <CollectionForm
            integration={integration}
            initial={initial}
            busy={actions.busy}
            error={actions.error}
            onSubmit={submit}
            onClose={close}
          />
        ))}
      {actions.confirmation}
    </>
  );
}

function CredentialForm({
  collection,
  integration,
  busy,
  error,
  onSubmit,
  onClose,
}: {
  collection: MonitoringCollection;
  integration: MonitoringIntegration;
  busy: boolean;
  error: string;
  onSubmit: (input: MonitoringMutation) => Promise<boolean>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [invalid, setInvalid] = useState("");
  const submit = async (remove = false) => {
    if (!remove && (!username.trim() || !password)) {
      setInvalid(t("monitoring.completeRequired"));
      return;
    }
    const input: MonitoringMutation = {
      kind: "collection",
      operation: "update",
      id: collection.id,
      expected_version: collection.version,
      collection: {
        name: collection.name,
        integration: collection.integration,
        target: collection.target,
        collector_id: collection.collector_id,
        application_id: collection.application_id,
        service_id: collection.service_id,
        config: collection.config,
        enabled: collection.desired_enabled,
        credential_update: remove ? "remove" : "replace",
      },
      ...(remove
        ? {}
        : { credentials: { username: username.trim(), password } }),
    };
    if (await onSubmit(input)) {
      setUsername("");
      setPassword("");
    }
  };
  return (
    <FormDialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("monitoring.hostForm.credentials")}
      description={collection.name}
      loading={busy}
      onSubmit={() => void submit()}
      submitLabel={t("monitoring.previewCollection")}
    >
      {(invalid || error) && <p role="alert">{invalid || error}</p>}
      <Field label={t("monitoring.username")} requirement="required">
        <Input
          autoComplete="off"
          value={username}
          onChange={(event) => setUsername(event.target.value)}
        />
      </Field>
      <Field label={t("monitoring.password")} requirement="required">
        <Input
          type="password"
          autoComplete="new-password"
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
      </Field>
      {integration.form?.authentication === "optional" && (
        <Button isDisabled={busy} onPress={() => void submit(true)}>
          {t("monitoring.hostForm.remove")}
        </Button>
      )}
    </FormDialog>
  );
}
