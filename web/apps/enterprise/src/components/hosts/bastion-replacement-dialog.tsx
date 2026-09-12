import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import {
  formatApiError,
  useApi,
  type ActionOneTimeResult,
  type BastionScope,
  type ConfirmActionResult,
  type ConnectionTest,
  type Host,
  type PendingActionPublic,
} from "@argus/api-client";
import { Alert, Button, Dialog, Field, Input, Select } from "@argus/ui";

import { InstallInstructionPanel } from "./install-instruction-panel";
import { PendingActionConfirm } from "./pending-action-confirm";
import { ConnectionTestSummary } from "./connection-test-summary";

const FORM_ID = "argus-bastion-replacement-form";
type ReplacementForm = {
  address: string;
  port: string;
  username: string;
  credentialId: string;
};

export function BastionReplacementDialog({
  scope,
  host,
  onOpenChange,
  onChanged,
}: {
  scope: BastionScope | null;
  host: Host | null;
  onOpenChange: (open: boolean) => void;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const [pending, setPending] = useState<PendingActionPublic | null>(null);
  const [command, setCommand] = useState<ActionOneTimeResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [connectionTest, setConnectionTest] = useState<ConnectionTest | null>(
    null,
  );
  const commandMode = scope?.onboarding_mode === "command";
  const schema = useMemo(
    () =>
      z
        .object({
          address: z.string(),
          port: z.string(),
          username: z.string(),
          credentialId: z.string(),
        })
        .superRefine((value, context) => {
          if (commandMode) return;
          if (!value.address.trim())
            context.addIssue({
              code: "custom",
              path: ["address"],
              message: t("hosts.bastionForm.replacementConnectionRequired"),
            });
          const numericPort = Number(value.port);
          if (
            !Number.isInteger(numericPort) ||
            numericPort < 1 ||
            numericPort > 65_535
          )
            context.addIssue({
              code: "custom",
              path: ["port"],
              message: t("hosts.bastionForm.replacementConnectionRequired"),
            });
          for (const field of ["username", "credentialId"] as const)
            if (!value[field].trim())
              context.addIssue({
                code: "custom",
                path: [field],
                message: t("hosts.bastionForm.replacementConnectionRequired"),
              });
        }),
    [commandMode, t],
  );
  const form = useForm<ReplacementForm>({
    resolver: zodResolver(schema),
    defaultValues: {
      address: "",
      port: "22",
      username: "",
      credentialId: "",
    },
  });
  const values = form.watch();

  useEffect(() => {
    if (!scope || !host) return;
    form.reset({
      address: host.address ?? "",
      port: String(host.port || 22),
      username: "",
      credentialId: "",
    });
    setPending(null);
    setCommand(null);
    setError("");
    setConnectionTest(null);
  }, [form, host, scope]);

  const credentialsQuery = useQuery({
    queryKey: ["credentials", "ssh", "bastion-replacement"],
    queryFn: () => api.secrets.listCredentials(),
    enabled: Boolean(scope && !commandMode),
  });
  const credentials = (credentialsQuery.data ?? []).filter(
    (credential) =>
      credential.protocol === "ssh" && credential.status === "active",
  );

  const preview = async (input: ReplacementForm) => {
    if (!scope || !host || busy) return;
    setBusy(true);
    setError("");
    setConnectionTest(null);
    try {
      if (commandMode) {
        setPending(
          await api.connectors.previewConnectorReplacement(scope.id, {
            expected_version: scope.resource_version,
          }),
        );
        return;
      }
      let test = await api.hosts.createConnectionTest({
        address: input.address.trim(),
        port: Number(input.port),
        platform: "linux",
        ssh_path: "direct_executor",
        onboarding_control_path:
          scope.onboarding_mode === "direct_install_tunnel"
            ? "executor_tunnel"
            : "direct",
        credential_id: input.credentialId,
        username: input.username.trim(),
      });
      for (
        let attempt = 0;
        attempt < 60 && ["queued", "running"].includes(test.status);
        attempt += 1
      ) {
        await new Promise((resolve) => window.setTimeout(resolve, 500));
        test = await api.hosts.getConnectionTest(test.id);
      }
      if (test.status !== "succeeded") {
        setConnectionTest(test);
        return;
      }
      setPending(
        await api.connectors.previewConnectorReplacement(scope.id, {
          expected_version: scope.resource_version,
          address: input.address.trim(),
          port: Number(input.port),
          username: input.username.trim(),
          credential_id: input.credentialId,
          connection_test_id: test.id,
        }),
      );
    } catch (cause) {
      setError(
        formatApiError(
          cause,
          t("hosts.bastionForm.commandGenerateFailed"),
          (requestId) => t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };

  const committed = (result: ConfirmActionResult) => {
    setPending(null);
    onChanged();
    if (result.one_time_result) {
      setCommand(result.one_time_result);
      return;
    }
    onOpenChange(false);
  };

  return (
    <Dialog
      description={
        commandMode
          ? t("hosts.bastionForm.replacementCommandDescription")
          : t("hosts.bastionForm.replacementOperationDescription")
      }
      footer={
        !pending && !command ? (
          <>
            <Button onClick={() => onOpenChange(false)} variant="secondary">
              {t("common.cancel")}
            </Button>
            <Button form={FORM_ID} loading={busy} type="submit">
              {t("hosts.bastionForm.replaceConnector")}
            </Button>
          </>
        ) : undefined
      }
      onOpenChange={onOpenChange}
      open={Boolean(scope && host)}
      size="lg"
      title={t("hosts.bastionForm.replacementTitle")}
    >
      <div className="argus-dialog__flow">
        <Alert
          description={t("hosts.bastionForm.replacementWarning")}
          title={t("hosts.bastionForm.replacementWarningTitle")}
          tone="warning"
        />
        {error && (
          <Alert
            description={error}
            title={t("hosts.pendingActions.failed")}
            tone="danger"
          />
        )}
        {!pending && !command && (
          <form id={FORM_ID} onSubmit={form.handleSubmit(preview)}>
            {!commandMode && (
              <div className="argus-scenario-wizard__form">
                <Field
                  error={form.formState.errors.address?.message}
                  label={t("hosts.wizard.address")}
                  requirement="required"
                >
                  <Input {...form.register("address")} />
                </Field>
                <Field
                  error={form.formState.errors.port?.message}
                  label={t("hosts.wizard.port")}
                  requirement="required"
                >
                  <Input inputMode="numeric" {...form.register("port")} />
                </Field>
                <Field
                  error={form.formState.errors.username?.message}
                  label={t("hosts.wizard.account")}
                  requirement="required"
                >
                  <Input {...form.register("username")} />
                </Field>
                <Field
                  error={form.formState.errors.credentialId?.message}
                  label={t("hosts.wizard.credential")}
                  requirement="required"
                >
                  <Select
                    onValueChange={(value) =>
                      form.setValue("credentialId", value, {
                        shouldValidate: true,
                      })
                    }
                    options={credentials.map((credential) => ({
                      label: credential.name,
                      value: credential.id,
                    }))}
                    value={values.credentialId}
                  />
                </Field>
              </div>
            )}
          </form>
        )}
        {connectionTest && <ConnectionTestSummary result={connectionTest} />}
        {pending && (
          <PendingActionConfirm
            action={pending}
            claimOneTimeResult={commandMode}
            onCancel={() => setPending(null)}
            onDone={committed}
          />
        )}
        {command && (
          <>
            <InstallInstructionPanel result={command} />
            <Button onClick={() => onOpenChange(false)} variant="primary">
              {t("hosts.done")}
            </Button>
          </>
        )}
      </div>
    </Dialog>
  );
}
