import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  formatApiError,
  useApi,
  type BastionScope,
  type Environment,
  type Host,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Alert,
  Button,
  Field,
  FormDrawer,
  Input,
  Select,
  Textarea,
} from "@argus/ui";

import { labelsToText, parseLabels } from "./host-utils";
import { PendingActionConfirm } from "./pending-action-confirm";

const ENVIRONMENTS: Environment[] = ["development", "staging", "production"];

type MetadataForm = {
  name: string;
  hostname: string;
  environment: Environment;
  labelsText: string;
};

function MetadataFields({
  value,
  onChange,
  includeHostname = false,
}: {
  value: MetadataForm;
  onChange: (value: MetadataForm) => void;
  includeHostname?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Field label={t("hosts.wizard.name")} requirement="required">
        <Input
          autoFocus
          value={value.name}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
        />
      </Field>
      {includeHostname && (
        <Field label={t("hosts.overview.kv.hostname")} requirement="optional">
          <Input
            value={value.hostname}
            onChange={(event) =>
              onChange({ ...value, hostname: event.target.value })
            }
          />
        </Field>
      )}
      <Field label={t("hosts.wizard.environment")} requirement="required">
        <Select
          value={value.environment}
          onValueChange={(environment) =>
            onChange({ ...value, environment: environment as Environment })
          }
          options={ENVIRONMENTS.map((environment) => ({
            value: environment,
            label: t(`hosts.env.${environment}`),
          }))}
        />
      </Field>
      <Field label={t("hosts.wizard.labels")} requirement="optional">
        <Textarea
          rows={4}
          value={value.labelsText}
          onChange={(event) =>
            onChange({ ...value, labelsText: event.target.value })
          }
        />
      </Field>
    </>
  );
}

export function EditBastionDrawer({
  scope,
  onOpenChange,
  onSaved,
}: {
  scope: BastionScope | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const [form, setForm] = useState<MetadataForm>({
    name: "",
    hostname: "",
    environment: "production",
    labelsText: "",
  });
  const [pending, setPending] = useState<PendingActionPublic | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (scope)
      setForm({
        name: scope.name,
        hostname: "",
        environment: scope.environment,
        labelsText: labelsToText(scope.labels),
      });
    setPending(null);
    setError("");
  }, [scope]);
  const preview = async () => {
    if (!scope || !form.name.trim()) return;
    setBusy(true);
    setError("");
    try {
      setPending(
        await api.connectors.previewUpdateBastionScope(scope.id, {
          name: form.name.trim(),
          environment: form.environment,
          labels: parseLabels(form.labelsText),
          expected_version: scope.resource_version,
        }),
      );
    } catch (cause) {
      setError(
        formatApiError(cause, t("hosts.edit.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <FormDrawer
      open={scope !== null}
      onOpenChange={onOpenChange}
      title={t("hosts.edit.title")}
      footer={
        !pending ? (
          <Button
            isPending={busy}
            onPress={() => void preview()}
            variant="primary"
          >
            {t("hosts.edit.save")}
          </Button>
        ) : undefined
      }
    >
      {error && (
        <Alert
          description={error}
          title={t("hosts.edit.failed")}
          tone="danger"
        />
      )}
      {!pending ? (
        <MetadataFields value={form} onChange={setForm} />
      ) : (
        <PendingActionConfirm
          action={pending}
          onCancel={() => setPending(null)}
          onDone={() => {
            setPending(null);
            onSaved();
            onOpenChange(false);
          }}
        />
      )}
    </FormDrawer>
  );
}

export function EditHostDrawer({
  host,
  onOpenChange,
  onSaved,
}: {
  host: Host | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const [form, setForm] = useState<MetadataForm>({
    name: "",
    hostname: "",
    environment: "production",
    labelsText: "",
  });
  const [pending, setPending] = useState<PendingActionPublic | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (host)
      setForm({
        name: host.name,
        hostname: host.hostname ?? "",
        environment: host.environment,
        labelsText: labelsToText(host.labels),
      });
    setPending(null);
    setError("");
  }, [host]);
  const preview = async () => {
    if (!host || !form.name.trim()) return;
    setBusy(true);
    setError("");
    try {
      setPending(
        await api.hosts.previewUpdateResource(host.id, {
          name: form.name.trim(),
          hostname: form.hostname.trim() || undefined,
          environment: form.environment,
          labels: parseLabels(form.labelsText),
          expected_version: host.resource_version,
        }),
      );
    } catch (cause) {
      setError(
        formatApiError(cause, t("hosts.edit.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <FormDrawer
      open={host !== null}
      onOpenChange={onOpenChange}
      title={t("hosts.edit.title")}
      footer={
        !pending ? (
          <Button
            isPending={busy}
            onPress={() => void preview()}
            variant="primary"
          >
            {t("hosts.edit.save")}
          </Button>
        ) : undefined
      }
    >
      {error && (
        <Alert
          description={error}
          title={t("hosts.edit.failed")}
          tone="danger"
        />
      )}
      {!pending ? (
        <MetadataFields value={form} onChange={setForm} includeHostname />
      ) : (
        <PendingActionConfirm
          action={pending}
          onCancel={() => setPending(null)}
          onDone={() => {
            setPending(null);
            onSaved();
            onOpenChange(false);
          }}
        />
      )}
    </FormDrawer>
  );
}
