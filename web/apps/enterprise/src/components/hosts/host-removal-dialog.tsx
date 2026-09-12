import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";

import {
  ApiError,
  formatApiError,
  formatErrorCode,
  useApi,
  type ActionOneTimeResult,
  type HostRemovalInstruction,
  type HostRemovalOperation,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Alert,
  Button,
  CodeBlock,
  Dialog,
  Field,
  Input,
  ScenarioCard,
  Select,
  StatusBadge,
} from "@argus/ui";

import { InstallInstructionPanel } from "./install-instruction-panel";
import { PendingActionConfirm } from "./pending-action-confirm";
import { HostRemovalActions } from "./host-removal-actions";
import type { RemovalTarget } from "./host-removal-target";

const REMOVAL_FORM_ID = "argus-host-removal-form";
const SSH_FAILURE_MESSAGES: Record<string, string> = {
  TIMEOUT: "hosts.removal.sshTimeout",
  CONNECTION_REFUSED: "hosts.removal.sshRefused",
  TARGET_UNROUTABLE: "hosts.removal.sshUnreachable",
  AUTH_FAILED: "hosts.removal.sshAuthFailed",
};
type RemovalForm = {
  username: string;
  credentialId: string;
  typedName: string;
};

type Dependency = { type: string; id: string; name: string; reason: string };

export function HostRemovalDialog({
  target,
  onOpenChange,
  onChanged,
}: {
  target: RemovalTarget | null;
  onOpenChange: (open: boolean) => void;
  onChanged: () => void;
}) {
  const sessionKey = target
    ? [
        target.type,
        target.id,
        target.expectedVersion,
        target.registeredConnectorId ?? "unregistered",
        target.existingOperationId ?? "new",
        target.onboardingState,
      ].join("/")
    : "closed";
  return (
    <HostRemovalDialogSession
      key={sessionKey}
      target={target}
      onOpenChange={onOpenChange}
      onChanged={onChanged}
    />
  );
}

function HostRemovalDialogSession({
  target,
  onOpenChange,
  onChanged,
}: {
  target: RemovalTarget | null;
  onOpenChange: (open: boolean) => void;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const queryClient = useQueryClient();
  const [mode, setMode] = useState<"uninstall" | "forget">("uninstall");
  const [action, setAction] = useState<PendingActionPublic | null>(null);
  const [operationId, setOperationId] = useState(
    target?.existingOperationId ?? "",
  );
  const [oneTimeResult, setOneTimeResult] =
    useState<ActionOneTimeResult | null>(null);
  const [replacementInstruction, setReplacementInstruction] =
    useState<HostRemovalInstruction | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [editingSSH, setEditingSSH] = useState(false);
  const [defaultsTarget, setDefaultsTarget] = useState("");
  const [serverNotInstalled, setServerNotInstalled] = useState(false);
  const previewAttempt = useRef(0);
  const schema = useMemo(
    () =>
      z
        .object({
          username: z.string(),
          credentialId: z.string(),
          typedName: z.string(),
        })
        .superRefine((value, context) => {
          if (mode === "uninstall" && target?.installMethod === "ssh") {
            if (!value.username.trim())
              context.addIssue({
                code: "custom",
                path: ["username"],
                message: t("hosts.removal.sshRequired"),
              });
            if (!value.credentialId)
              context.addIssue({
                code: "custom",
                path: ["credentialId"],
                message: t("hosts.removal.sshRequired"),
              });
          }
          if (mode === "forget" && value.typedName !== target?.name)
            context.addIssue({
              code: "custom",
              path: ["typedName"],
              message: t("hosts.removal.nameMismatch"),
            });
        }),
    [mode, t, target],
  );
  const form = useForm<RemovalForm>({
    resolver: zodResolver(schema),
    defaultValues: { username: "", credentialId: "", typedName: "" },
  });
  const values = form.watch();
  const resetForm = form.reset;

  const targetKey = target
    ? `${target.type}/${target.id}/${target.expectedVersion}`
    : "";
  const notInstalled =
    serverNotInstalled ||
    Boolean(
      target && !target.registeredConnectorId && !target.existingOperationId,
    );
  const incompleteInstallRunning = target?.onboardingState === "installing";
  const needsSSH = Boolean(
    target?.installMethod === "ssh" &&
    mode === "uninstall" &&
    !operationId &&
    !notInstalled,
  );
  useEffect(() => {
    previewAttempt.current += 1;
    setBusy(false);
    resetForm({ username: "", credentialId: "", typedName: "" });
    setDefaultsTarget("");
    setEditingSSH(false);
    return () => {
      previewAttempt.current += 1;
    };
  }, [resetForm, targetKey]);
  const defaultsQuery = useQuery({
    queryKey: ["host-removal-connection-defaults", targetKey],
    queryFn: () =>
      api.hosts.getRemovalConnectionDefaults({
        target_type: target!.type,
        target_id: target!.id,
        expected_version: target!.expectedVersion,
      }),
    enabled: needsSSH,
    retry: false,
  });
  useEffect(() => {
    if (
      defaultsQuery.error instanceof ApiError &&
      defaultsQuery.error.code === "HOST_REMOVAL_NOT_INSTALLED"
    ) {
      previewAttempt.current += 1;
      setBusy(false);
      setEditingSSH(false);
      setError("");
      setServerNotInstalled(true);
    }
  }, [defaultsQuery.error]);
  useEffect(() => {
    const saved = defaultsQuery.data;
    if (!needsSSH || !saved) return;
    if (
      defaultsTarget === targetKey &&
      (editingSSH || saved.status === "available")
    )
      return;
    resetForm({
      username: saved.username,
      credentialId: saved.credential_id ?? "",
      typedName: "",
    });
    setEditingSSH(saved.status !== "available");
    setDefaultsTarget(targetKey);
  }, [
    defaultsQuery.data,
    defaultsTarget,
    editingSSH,
    resetForm,
    needsSSH,
    targetKey,
  ]);

  const credentialsQuery = useQuery({
    queryKey: ["credentials", "ssh", "host-removal"],
    queryFn: () => api.secrets.listCredentials(),
    enabled: Boolean(needsSSH && editingSSH),
  });
  const credentials = (credentialsQuery.data ?? []).filter(
    (credential) =>
      credential.protocol === "ssh" && credential.status === "active",
  );
  const operationQuery = useQuery({
    queryKey: ["host-removal-operation", operationId],
    queryFn: () => api.hosts.getRemovalOperation(operationId),
    enabled: Boolean(operationId),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status &&
        ["succeeded", "failed", "cleanup_unknown"].includes(status)
        ? false
        : 1_500;
    },
  });
  const operation = operationQuery.data;
  const dependencies = useMemo(() => dependenciesOf(action), [action]);

  useEffect(() => {
    if (target?.existingOperationId) {
      setOperationId(target.existingOperationId);
      return;
    }
    if (!target) {
      setMode("uninstall");
      form.reset();
      setDefaultsTarget("");
      setEditingSSH(false);
      setAction(null);
      setOperationId("");
      setOneTimeResult(null);
      setReplacementInstruction(null);
      setError("");
    }
  }, [form, target]);

  const changeOpen = (open: boolean) => {
    if (!open) {
      previewAttempt.current += 1;
      setBusy(false);
    }
    onOpenChange(open);
  };
  const close = () => changeOpen(false);

  const changeMode = (next: "uninstall" | "forget") => {
    if (mode === next) return;
    previewAttempt.current += 1;
    setBusy(false);
    setMode(next);
    setError("");
    form.clearErrors();
  };

  const previewIncompleteDelete = async () => {
    if (!target || !notInstalled || busy) return;
    const attempt = ++previewAttempt.current;
    const isCurrent = () => previewAttempt.current === attempt;
    setBusy(true);
    setError("");
    try {
      const nextAction =
        target.type === "bastion_scope"
          ? await api.connectors.previewDeleteBastionScope(
              target.id,
              target.expectedVersion,
            )
          : await api.hosts.previewDeleteResource(
              target.id,
              target.expectedVersion,
            );
      if (isCurrent()) setAction(nextAction);
      else void api.approvals.cancel(nextAction.action_ref).catch(() => {});
    } catch (cause) {
      if (!isCurrent()) return;
      setError(
        formatApiError(
          cause,
          t("hostRemoval.incomplete.previewFailed"),
          (requestId) => t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      if (isCurrent()) setBusy(false);
    }
  };

  const preview = async (input: RemovalForm) => {
    if (!target || busy || notInstalled) return;
    const attempt = ++previewAttempt.current;
    const isCurrent = () => previewAttempt.current === attempt;
    setBusy(true);
    setError("");
    try {
      let connectionTestId: string | undefined;
      if (mode === "uninstall" && target.installMethod === "ssh") {
        let test = await api.hosts.createConnectionTest({
          address: target.address,
          port: target.port || 22,
          platform: target.platform,
          ssh_path:
            target.sshPath === "bastion_connector"
              ? "bastion_connector"
              : "direct_executor",
          bastion_scope_id:
            target.sshPath === "bastion_connector"
              ? target.bastionScopeId
              : undefined,
          credential_id: input.credentialId,
          username: input.username.trim(),
        });
        if (!isCurrent()) return;
        const deadline = Date.now() + 60_000;
        while (
          ["queued", "running"].includes(test.status) &&
          Date.now() < deadline
        ) {
          await new Promise((resolve) => window.setTimeout(resolve, 800));
          if (!isCurrent()) return;
          test = await api.hosts.getConnectionTest(test.id);
          if (!isCurrent()) return;
        }
        if (test.status !== "succeeded") {
          const code =
            test.error_code ??
            (["queued", "running"].includes(test.status)
              ? "TIMEOUT"
              : "CONNECTION_TEST_FAILED");
          const messageKey = SSH_FAILURE_MESSAGES[code];
          setError(
            messageKey
              ? t(messageKey, {
                  address: target.address,
                  port: target.port || 22,
                })
              : formatErrorCode(code, t("hosts.removal.sshTestFailed")),
          );
          return;
        }
        connectionTestId = test.id;
      }
      const nextAction = await api.hosts.previewRemoval({
        target_type: target.type,
        target_id: target.id,
        expected_version: target.expectedVersion,
        mode,
        connection_test_id: connectionTestId,
        credential_id: connectionTestId ? input.credentialId : undefined,
        confirmation_name: mode === "forget" ? input.typedName : undefined,
      });
      if (isCurrent()) setAction(nextAction);
      else void api.approvals.cancel(nextAction.action_ref).catch(() => {});
    } catch (cause) {
      if (!isCurrent()) return;
      setError(
        formatApiError(
          cause,
          t(
            mode === "forget"
              ? "hosts.removal.forgetPreviewFailed"
              : "hosts.removal.previewFailed",
          ),
          (requestId) => t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      if (isCurrent()) setBusy(false);
    }
  };
  const submitPreview = form.handleSubmit(preview);

  const onConfirmed = (result: {
    execution?: { operation_ref?: { id: string; kind: string } };
    one_time_result?: ActionOneTimeResult;
  }) => {
    const reference = result.execution?.operation_ref;
    if (reference?.kind === "host_removal") setOperationId(reference.id);
    if (result.one_time_result) setOneTimeResult(result.one_time_result);
    setAction(null);
    onChanged();
  };
  const onIncompleteDeleteConfirmed = () => {
    setAction(null);
    onChanged();
    close();
  };

  const retry = async () => {
    if (!operation || busy) return;
    setBusy(true);
    try {
      const next = await api.hosts.retryRemovalOperation(operation.id);
      queryClient.setQueryData(["host-removal-operation", operation.id], next);
      if (next.delivery_method === "manual")
        setReplacementInstruction(
          await api.hosts.regenerateRemovalCommand(next.id),
        );
    } catch (cause) {
      setError(
        formatApiError(cause, t("hosts.removal.retryFailed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };

  const regenerate = async () => {
    if (!operation || busy) return;
    setBusy(true);
    try {
      let current = operation;
      if (["failed", "cleanup_unknown"].includes(current.status)) {
        current = await api.hosts.retryRemovalOperation(current.id);
        queryClient.setQueryData(
          ["host-removal-operation", current.id],
          current,
        );
      }
      setReplacementInstruction(
        await api.hosts.regenerateRemovalCommand(current.id),
      );
    } catch (cause) {
      setError(
        formatApiError(cause, t("hosts.removal.commandFailed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };

  const canForget = Boolean(
    target &&
    (target.connectionStatus === "offline" ||
      ["removal_failed", "cleanup_unknown"].includes(target.status)),
  );

  return (
    <Dialog
      description={
        target
          ? t(
              notInstalled
                ? "hostRemoval.incomplete.dialogDescription"
                : "hosts.removal.description",
              { name: target.name },
            )
          : ""
      }
      footer={
        notInstalled && !action ? (
          <>
            <Button onClick={close} variant="secondary">
              {t("hostRemoval.incomplete.backToResource")}
            </Button>
            <Button
              disabled={busy}
              onClick={() => void previewIncompleteDelete()}
              variant="danger"
            >
              {busy
                ? t("hostRemoval.incomplete.previewing")
                : t(
                    incompleteInstallRunning
                      ? "hostRemoval.incomplete.cancelAndDelete"
                      : "hostRemoval.incomplete.deleteRecord",
                  )}
            </Button>
          </>
        ) : !action && !operationId ? (
          <>
            <Button onClick={close} variant="secondary">
              {t("common.cancel")}
            </Button>
            <Button
              disabled={
                busy ||
                (needsSSH && (defaultsQuery.isPending || defaultsQuery.isError))
              }
              form={REMOVAL_FORM_ID}
              type="submit"
              variant={mode === "forget" ? "danger" : "primary"}
            >
              {busy
                ? t("hosts.removal.preparing")
                : t(
                    mode === "forget"
                      ? "hosts.removal.forgetPreview"
                      : "hosts.removal.preview",
                  )}
            </Button>
          </>
        ) : undefined
      }
      onOpenChange={changeOpen}
      open={Boolean(target)}
      size="lg"
      title={
        notInstalled
          ? t(
              incompleteInstallRunning
                ? "hostRemoval.incomplete.runningTitle"
                : "hostRemoval.incomplete.failedTitle",
            )
          : mode === "forget"
            ? t("hosts.removal.forgetTitle")
            : t("hosts.removal.title")
      }
    >
      {target && (
        <div className="argus-removal-layout">
          {notInstalled && (
            <Alert
              title={t(
                incompleteInstallRunning
                  ? "hostRemoval.incomplete.runningTitle"
                  : "hostRemoval.incomplete.failedTitle",
              )}
              description={t("hostRemoval.incomplete.warning")}
              tone="warning"
            />
          )}
          {!notInstalled && !action && !operationId && (
            <form
              className="argus-removal-layout"
              id={REMOVAL_FORM_ID}
              noValidate
              onSubmit={submitPreview}
            >
              <div
                aria-label={t("hosts.removal.chooseMode")}
                className="argus-removal-modes"
                role="group"
              >
                <ScenarioCard
                  title={t("hosts.removal.uninstallOption")}
                  description={t(
                    target.installMethod === "ssh"
                      ? "hosts.removal.uninstallSSHOptionDescription"
                      : "hosts.removal.uninstallManualOptionDescription",
                  )}
                  diagram={null}
                  selected={mode === "uninstall"}
                  onSelect={() => changeMode("uninstall")}
                />
                <ScenarioCard
                  title={t("hosts.removal.forgetTitle")}
                  description={t("hosts.removal.forgetOptionDescription")}
                  diagram={null}
                  selected={mode === "forget"}
                  status={canForget ? "supported" : "unavailable"}
                  statusLabel={
                    canForget ? undefined : t("hosts.removal.forgetUnavailable")
                  }
                  onSelect={() => changeMode("forget")}
                />
              </div>
              {mode === "uninstall" && (
                <Alert
                  description={t(
                    target.installMethod === "manual"
                      ? "hosts.removal.manualDescription"
                      : "hosts.removal.sshDescription",
                  )}
                  title={t(
                    target.installMethod === "manual"
                      ? "hosts.removal.manualTitle"
                      : "hosts.removal.sshTitle",
                  )}
                  tone="info"
                />
              )}
              {mode === "uninstall" &&
                target.installMethod === "ssh" &&
                target.connectionStatus === "offline" && (
                  <Alert
                    title={t("hosts.removal.offlineTitle")}
                    description={t("hosts.removal.offlineDescription")}
                    tone="warning"
                  />
                )}
              {mode === "uninstall" &&
                target.installMethod === "ssh" &&
                (defaultsQuery.isPending ? (
                  <p>{t("hosts.removal.loadingConnection")}</p>
                ) : defaultsQuery.isError ? (
                  <div className="argus-removal-connection">
                    <Alert
                      title={t("hosts.removal.connectionLoadFailedTitle")}
                      tone="warning"
                      description={formatApiError(
                        defaultsQuery.error,
                        t("hosts.removal.connectionLoadFailed"),
                        (requestId) =>
                          t("common.requestReference", { requestId }),
                      )}
                    />
                    <Button
                      onClick={() => void defaultsQuery.refetch()}
                      variant="secondary"
                    >
                      {t("hosts.removal.retryConnectionDefaults")}
                    </Button>
                  </div>
                ) : !editingSSH &&
                  defaultsQuery.data?.status === "available" ? (
                  <div className="argus-removal-connection">
                    <Alert
                      title={t("hosts.removal.savedConnection")}
                      description={t(
                        "hosts.removal.savedConnectionDescription",
                        {
                          username: values.username,
                          credential: defaultsQuery.data.credential_name,
                        },
                      )}
                      tone="info"
                    />
                    <Button
                      onClick={() => setEditingSSH(true)}
                      variant="secondary"
                    >
                      {t("hosts.removal.changeConnection")}
                    </Button>
                  </div>
                ) : (
                  <>
                    {defaultsQuery.data?.status !== "available" && (
                      <Alert
                        title={t("hosts.removal.connectionNeedsUpdate")}
                        tone="warning"
                        description={t("hosts.removal.connectionUnavailable")}
                      />
                    )}
                    <div className="argus-scenario-wizard__form">
                      <Field
                        error={form.formState.errors.username?.message}
                        label={t("hosts.removal.username")}
                        requirement="required"
                      >
                        <Input {...form.register("username")} />
                      </Field>
                      <Field
                        error={form.formState.errors.credentialId?.message}
                        label={t("hosts.removal.credential")}
                        requirement="required"
                      >
                        <Select
                          onValueChange={(value) =>
                            form.setValue("credentialId", value, {
                              shouldValidate: true,
                            })
                          }
                          options={credentials.map((credential) => ({
                            value: credential.id,
                            label: credential.name,
                          }))}
                          value={values.credentialId}
                        />
                      </Field>
                    </div>
                  </>
                ))}
              {mode === "forget" && (
                <>
                  <Alert
                    description={t("hosts.removal.forgetWarning")}
                    title={t("hosts.removal.forgetTitle")}
                    tone="danger"
                  />
                  <Field
                    error={form.formState.errors.typedName?.message}
                    label={t("hosts.removal.typeName", { name: target.name })}
                    requirement="required"
                  >
                    <Input {...form.register("typedName")} />
                  </Field>
                </>
              )}
            </form>
          )}

          {error && (
            <Alert
              description={error}
              title={t(
                notInstalled
                  ? "hostRemoval.incomplete.previewFailed"
                  : mode === "forget"
                    ? "hosts.removal.forgetFailed"
                    : "hosts.removal.failed",
              )}
              tone="danger"
            />
          )}

          {action &&
            target.type === "bastion_scope" &&
            dependencies.length > 0 && (
              <>
                <Alert
                  description={t("hosts.removal.dependenciesDescription")}
                  title={t("hosts.removal.dependenciesTitle")}
                  tone="warning"
                />
                <ul className="argus-removal-dependencies">
                  {dependencies.map((dependency) => (
                    <li key={`${dependency.type}:${dependency.id}`}>
                      <strong>
                        <a
                          className="argus-removal-dependencies__link"
                          href={dependencyHref(dependency, target.hostId)}
                        >
                          {dependency.name}
                          <span className="argus-sr-only">
                            {t("hosts.removal.viewDependency")}
                          </span>
                        </a>
                      </strong>
                      <span>
                        {t(`hosts.removal.dependency.${dependency.type}`, {
                          defaultValue: dependency.type,
                        })}{" "}
                        · {dependency.reason}
                      </span>
                    </li>
                  ))}
                </ul>
                <Button
                  onClick={() => {
                    void api.approvals.cancel(action.action_ref);
                    setAction(null);
                  }}
                  variant="secondary"
                >
                  {t("common.close")}
                </Button>
              </>
            )}
          {action &&
            (target.type !== "bastion_scope" || dependencies.length === 0) && (
              <PendingActionConfirm
                action={action}
                claimOneTimeResult
                confirmLabel={
                  notInstalled
                    ? t("hostRemoval.incomplete.confirmDelete")
                    : undefined
                }
                onCancel={() => setAction(null)}
                onDone={
                  notInstalled ? onIncompleteDeleteConfirmed : onConfirmed
                }
              />
            )}

          {oneTimeResult && <InstallInstructionPanel result={oneTimeResult} />}
          {replacementInstruction?.command && (
            <CodeBlock
              code={replacementInstruction.command}
              language={
                replacementInstruction.shell === "powershell"
                  ? "powershell"
                  : "bash"
              }
            />
          )}
          {operation && <RemovalProgress operation={operation} />}
          {operation?.delivery_method === "manual" &&
            ["awaiting_manual_execution", "running"].includes(
              operation.status,
            ) && (
              <Button
                disabled={busy}
                onClick={() => void regenerate()}
                variant="secondary"
              >
                {t("hosts.removal.regenerate")}
              </Button>
            )}
          {operation &&
            ["failed", "cleanup_unknown"].includes(operation.status) && (
              <HostRemovalActions
                busy={busy}
                manual={operation.delivery_method === "manual"}
                onRegenerate={() => void regenerate()}
                onRetry={() => void retry()}
                regenerateLabel={t("hosts.removal.regenerate")}
                retryLabel={t("hosts.removal.retry")}
              />
            )}
          {operation?.status === "succeeded" && (
            <Button
              onClick={() => {
                onChanged();
                close();
              }}
              variant="primary"
            >
              {t("hosts.done")}
            </Button>
          )}
          {oneTimeResult && !operation && (
            <Button
              onClick={() => {
                onChanged();
                close();
              }}
              type="button"
              variant="primary"
            >
              {t("hosts.done")}
            </Button>
          )}
        </div>
      )}
    </Dialog>
  );
}

function dependencyHref(dependency: Dependency, targetHostId: string) {
  if (dependency.type === "member_host") return `/hosts/${dependency.id}`;
  if (dependency.type === "remote_session") return "/remote-sessions";
  if (
    [
      "onboarding_operation",
      "removal_operation",
      "collector_operation",
      "connector_command",
      "credential_lease",
    ].includes(dependency.type)
  )
    return "/tasks";
  return `/hosts/${targetHostId}`;
}

function dependenciesOf(action: PendingActionPublic | null): Dependency[] {
  if (!action || typeof action.preview !== "object" || action.preview === null)
    return [];
  const value = (action.preview as Record<string, unknown>).dependencies;
  if (!Array.isArray(value)) return [];
  return value.filter((entry): entry is Dependency =>
    Boolean(
      entry && typeof entry === "object" && "id" in entry && "type" in entry,
    ),
  ) as Dependency[];
}

function RemovalProgress({ operation }: { operation: HostRemovalOperation }) {
  const { t } = useTranslation();
  const tone =
    operation.status === "succeeded"
      ? "success"
      : operation.status === "failed" || operation.status === "cleanup_unknown"
        ? "danger"
        : "warning";
  return (
    <div className="argus-removal-progress">
      <div>
        <StatusBadge pulse={operation.status === "running"} tone={tone}>
          {t(`hosts.removal.status.${operation.status}`)}
        </StatusBadge>
        <strong>{t(`hosts.removal.stage.${operation.stage}`)}</strong>
      </div>
      {operation.error_code && (
        <span className="argus-muted">{operation.error_code}</span>
      )}
      <ol>
        {operation.events.map((event) => (
          <li key={event.id}>
            <span>{t(`hosts.removal.stage.${event.stage}`)}</span>
            <span>{t(`hosts.removal.event.${event.status}`)}</span>
          </li>
        ))}
      </ol>
    </div>
  );
}
