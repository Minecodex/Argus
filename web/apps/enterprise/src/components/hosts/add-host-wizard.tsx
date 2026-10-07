import { zodResolver } from "@hookform/resolvers/zod";
import { useMemo, useReducer, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";

import {
  presentApiFormError,
  useApi,
  type ActionOneTimeResult,
  type BastionScope,
  type ConnectionTest,
  type Environment,
  type HostPreviewCreate,
  type Host,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Alert,
  Button,
  ConfirmDialog,
  Dialog,
  Field,
  Input,
  ModeGrid,
  ScenarioCard,
  Select,
  Textarea,
  TopologyDiagram,
  WizardProgress,
} from "@argus/ui";

import { formatDateTime } from "../settings/shared";
import { parseLabels } from "./host-utils";
import { InstallInstructionPanel } from "./install-instruction-panel";
import {
  onboardingWizardReducer,
  onboardingWizardStep,
  type OnboardingWizardState,
} from "./onboarding-wizard-state";
import { PendingActionConfirm } from "./pending-action-confirm";
import { ConnectionTestSummary } from "./connection-test-summary";
import { connectionTestFailureMessage } from "./onboarding-errors";
import {
  useResourceNameAvailability,
  type ResourceNameSnapshot,
} from "./use-resource-name-availability";

const FORM_ID = "argus-host-onboarding-form";
const ENVIRONMENTS: Environment[] = ["development", "staging", "production"];

type HostOnboardingMode =
  | "command_direct"
  | "ssh_direct"
  | "ssh_tunnel"
  | "command_bastion"
  | "ssh_bastion";

type HostModeConfig = {
  installMethod: "manual" | "ssh";
  controlPath: "direct" | "bastion_relay" | "executor_tunnel";
  sshPath: "none" | "direct_executor" | "bastion_connector";
};

const HOST_MODE_CONFIG: Record<HostOnboardingMode, HostModeConfig> = {
  command_direct: {
    installMethod: "manual",
    controlPath: "direct",
    sshPath: "none",
  },
  ssh_direct: {
    installMethod: "ssh",
    controlPath: "direct",
    sshPath: "direct_executor",
  },
  ssh_tunnel: {
    installMethod: "ssh",
    controlPath: "executor_tunnel",
    sshPath: "direct_executor",
  },
  command_bastion: {
    installMethod: "manual",
    controlPath: "bastion_relay",
    sshPath: "none",
  },
  ssh_bastion: {
    installMethod: "ssh",
    controlPath: "bastion_relay",
    sshPath: "bastion_connector",
  },
};

const initialState: OnboardingWizardState<HostOnboardingMode> = {
  phase: "select_mode",
  mode: "command_direct",
};

type HostWizardForm = {
  name: string;
  environment: Environment;
  labelsText: string;
  platform: "linux" | "windows";
  architecture: "amd64" | "arm64";
  scopeId: string;
  address: string;
  port: string;
  username: string;
  credentialId: string;
};

export function AddHostWizard({
  open,
  onOpenChange,
  scopes,
  onCreated,
  retryHost,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  scopes: BastionScope[];
  onCreated: () => void;
  retryHost?: Host;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const [wizard, dispatch] = useReducer(
    onboardingWizardReducer<HostOnboardingMode>,
    retryHost
      ? {
          ...initialState,
          mode: (retryHost.control_path === "bastion_relay"
            ? "ssh_bastion"
            : retryHost.control_path === "executor_tunnel"
              ? "ssh_tunnel"
              : "ssh_direct") as HostOnboardingMode,
        }
      : initialState,
  );
  const [pendingMode, setPendingMode] = useState<HostOnboardingMode | null>(
    null,
  );
  const [pendingAction, setPendingAction] =
    useState<PendingActionPublic | null>(null);
  const [connectionTest, setConnectionTest] = useState<ConnectionTest | null>(
    null,
  );
  const [oneTimeResult, setOneTimeResult] =
    useState<ActionOneTimeResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [previewName, setPreviewName] = useState<ResourceNameSnapshot | null>(
    null,
  );
  const notified = useRef(false);
  const prepareGeneration = useRef(0);
  const mode = HOST_MODE_CONFIG[wizard.mode];
  const relayScopes = scopes.filter(
    (scope) => scope.relay_status === "ready" && Boolean(scope.relay_address),
  );

  const schema = useMemo(
    () =>
      z
        .object({
          name: z
            .string()
            .trim()
            .min(1)
            .refine(
              (name) => Array.from(name).length <= 128,
              t("resourceNames.tooLong"),
            ),
          environment: z.enum(["development", "staging", "production"]),
          labelsText: z.string(),
          platform: z.enum(["linux", "windows"]),
          architecture: z.enum(["amd64", "arm64"]),
          scopeId: z.string(),
          address: z.string(),
          port: z.string(),
          username: z.string(),
          credentialId: z.string(),
        })
        .superRefine((value, context) => {
          const issue = (
            path: keyof HostWizardForm,
            message = t("hosts.wizard.required"),
          ) => context.addIssue({ code: "custom", path: [path], message });
          if (value.platform === "windows" && value.architecture !== "amd64")
            issue("architecture");
          if (mode.controlPath === "bastion_relay" && !value.scopeId)
            issue("scopeId");
          if (mode.installMethod === "ssh") {
            if (!value.address.trim()) issue("address");
            const port = Number(value.port);
            if (!Number.isInteger(port) || port < 1 || port > 65535)
              issue("port", t("hosts.wizard.portInvalid"));
            if (!value.username.trim()) issue("username");
            if (!value.credentialId) issue("credentialId");
          }
        }),
    [mode.controlPath, mode.installMethod, t],
  );
  const form = useForm<HostWizardForm>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: retryHost?.name ?? "",
      environment: retryHost?.environment ?? "production",
      labelsText: "",
      platform: retryHost?.platform ?? "linux",
      architecture: retryHost?.architecture ?? "amd64",
      scopeId: retryHost?.bastion_scope_id ?? "",
      address: retryHost?.address ?? "",
      port: String(retryHost?.port || 22),
      username: "",
      credentialId: "",
    },
  });
  const values = form.watch();
  const nameAvailability = useResourceNameAvailability({
    name: values.name,
    enabled: open && !retryHost,
    check: api.hosts.checkNameAvailability,
  });
  const credentialsQuery = useQuery({
    queryKey: ["credentials", "ssh"],
    queryFn: () => api.secrets.listCredentials(),
    enabled:
      open && wizard.phase !== "select_mode" && mode.installMethod === "ssh",
  });
  const credentials = (credentialsQuery.data ?? []).filter(
    (credential) =>
      credential.protocol === "ssh" && credential.status === "active",
  );

  const reset = () => {
    prepareGeneration.current += 1;
    nameAvailability.reset();
    form.reset();
    dispatch({ type: "reset", mode: "command_direct" });
    setPendingMode(null);
    setPendingAction(null);
    setConnectionTest(null);
    setOneTimeResult(null);
    setBusy(false);
    notified.current = false;
  };
  const close = (next: boolean) => {
    if (!next) reset();
    onOpenChange(next);
  };

  const applyMode = (next: HostOnboardingMode) => {
    const current = HOST_MODE_CONFIG[wizard.mode];
    const target = HOST_MODE_CONFIG[next];
    if (current.installMethod === "ssh" && target.installMethod === "manual") {
      form.setValue("address", "");
      form.setValue("port", "22");
      form.setValue("username", "");
      form.setValue("credentialId", "");
    }
    if (target.controlPath !== "bastion_relay") form.setValue("scopeId", "");
    setConnectionTest(null);
    setPendingAction(null);
    dispatch({ type: "select_mode", mode: next });
  };
  const requestMode = (next: HostOnboardingMode) => {
    if (next === wizard.mode) return;
    const current = HOST_MODE_CONFIG[wizard.mode];
    const target = HOST_MODE_CONFIG[next];
    const losesSSHFields =
      current.installMethod === "ssh" &&
      target.installMethod === "manual" &&
      Boolean(
        values.address.trim() ||
        values.username.trim() ||
        values.credentialId ||
        values.port !== "22",
      );
    const losesScope =
      current.controlPath === "bastion_relay" &&
      target.controlPath !== "bastion_relay" &&
      Boolean(values.scopeId);
    if (losesSSHFields || losesScope || connectionTest) {
      setPendingMode(next);
      return;
    }
    applyMode(next);
  };

  const prepare = async (input: HostWizardForm) => {
    if (busy) return;
    setBusy(true);
    setConnectionTest(null);
    form.clearErrors();
    const nameSnapshot = nameAvailability.capture();
    if (input.name.trim() !== nameSnapshot.name.trim()) {
      setBusy(false);
      return;
    }
    const generation = ++prepareGeneration.current;
    const isCurrent = () =>
      retryHost || nameAvailability.isCurrent(nameSnapshot);
    try {
      if (!retryHost && !(await nameAvailability.validate())) return;
      let test: ConnectionTest | null = null;
      if (mode.installMethod === "ssh") {
        test = await api.hosts.createConnectionTest({
          address: input.address.trim(),
          port: Number(input.port),
          platform: input.platform,
          ssh_path: mode.sshPath as "direct_executor" | "bastion_connector",
          onboarding_control_path: mode.controlPath,
          bastion_scope_id:
            mode.sshPath === "bastion_connector" ? input.scopeId : undefined,
          credential_id: input.credentialId,
          username: input.username.trim(),
        });
        if (!isCurrent()) return;
        for (
          let attempt = 0;
          attempt < 60 && ["queued", "running"].includes(test.status);
          attempt += 1
        ) {
          await new Promise((resolve) => window.setTimeout(resolve, 500));
          test = await api.hosts.getConnectionTest(test.id);
          if (!isCurrent()) return;
        }
        setConnectionTest(test);
        if (test.status !== "succeeded") {
          form.setError("root", {
            type: "server",
            message: connectionTestFailureMessage(test.error_code, t),
          });
          return;
        }
      }
      const request: HostPreviewCreate = {
        name: input.name.trim(),
        platform: input.platform,
        role: "managed_host",
        control_path: mode.controlPath,
        install_method: mode.installMethod,
        ssh_path: mode.sshPath,
        bastion_scope_id:
          mode.controlPath === "bastion_relay" ? input.scopeId : undefined,
        architecture:
          mode.installMethod === "manual" ? input.architecture : undefined,
        environment: input.environment,
        labels: parseLabels(input.labelsText),
        address:
          mode.installMethod === "ssh" ? input.address.trim() : undefined,
        port: mode.installMethod === "ssh" ? Number(input.port) : undefined,
        username:
          mode.installMethod === "ssh" ? input.username.trim() : undefined,
        credential_id:
          mode.installMethod === "ssh" ? input.credentialId : undefined,
        connection_test_id: test?.id,
      };
      const action = retryHost
        ? await api.hosts.previewRetryResource(retryHost.id, request)
        : await api.hosts.previewCreateResource(request);
      if (!isCurrent()) {
        void api.approvals.cancel(action.action_ref).catch(() => {});
        return;
      }
      setPreviewName(nameSnapshot);
      setPendingAction(action);
      dispatch({
        type: "next",
        terminal:
          mode.installMethod === "manual" ? "confirm_command" : "verify",
      });
    } catch (error) {
      if (!isCurrent()) return;
      if (nameAvailability.handleConflict(error, nameSnapshot)) return;
      presentApiFormError(error, {
        fallback: t("hosts.wizard.previewFailed"),
        fieldMap: {
          address: "address",
          bastion_scope_id: "scopeId",
          credential_id: "credentialId",
          name: "name",
          platform: "platform",
          port: "port",
          username: "username",
        },
        requestReference: (requestId) =>
          t("common.requestReference", { requestId }),
        setFieldError: (field, message) =>
          form.setError(field, { message, type: "server" }),
        setFormError: (message) =>
          form.setError("root", { message, type: "server" }),
      });
    } finally {
      if (prepareGeneration.current === generation) setBusy(false);
    }
  };

  const steps = [
    {
      id: "mode",
      title: t("hosts.wizard.step1"),
      description: t("hosts.wizard.step1Desc"),
    },
    {
      id: "details",
      title: t("hosts.wizard.step2"),
      description: t("hosts.wizard.step2Desc"),
    },
    {
      id: "verify",
      title:
        mode.installMethod === "manual"
          ? t("hosts.wizard.generateCommand")
          : t("hosts.wizard.testAndPreview"),
      description: t("hosts.wizard.step3Desc"),
    },
  ];
  const terminalPhase =
    wizard.phase === "command_result" || wizard.phase === "completed";

  return (
    <>
      <Dialog
        className="argus-dialog--wizard"
        description={t(
          retryHost
            ? "hosts.onboardingProgress.retryHint"
            : "hosts.wizard.dialogDesc",
        )}
        footer={
          <>
            <span className="argus-dialog__footer-hint">
              {wizard.phase === "select_mode"
                ? t("hosts.wizard.scenarioHint")
                : t("hosts.wizard.footerHint")}
            </span>
            {wizard.phase === "select_mode" && (
              <>
                <Button onPress={() => close(false)} variant="secondary">
                  {t("hosts.cancel")}
                </Button>
                <Button
                  onPress={() =>
                    dispatch({
                      type: "next",
                      terminal:
                        mode.installMethod === "manual"
                          ? "confirm_command"
                          : "verify",
                    })
                  }
                  variant="primary"
                >
                  {t("hosts.wizard.next")}
                </Button>
              </>
            )}
            {wizard.phase === "details" && (
              <>
                <Button
                  onPress={() => dispatch({ type: "back" })}
                  variant="secondary"
                >
                  {t("hosts.wizard.back")}
                </Button>
                <Button
                  form={FORM_ID}
                  isPending={busy}
                  type="submit"
                  variant="primary"
                >
                  {mode.installMethod === "manual"
                    ? t("hosts.wizard.generateCommand")
                    : t("hosts.wizard.testAndPreview")}
                </Button>
              </>
            )}
            {terminalPhase && (
              <Button onPress={() => close(false)} variant="primary">
                {t("hosts.done")}
              </Button>
            )}
          </>
        }
        onOpenChange={close}
        open={open}
        title={t(
          retryHost ? "hosts.onboardingProgress.retry" : "hosts.wizard.title",
        )}
        width={1080}
      >
        <WizardProgress
          current={onboardingWizardStep(wizard.phase)}
          steps={steps}
        />
        <div
          className="argus-wizard-dialog__content"
          data-control-path={mode.controlPath}
          data-install-method={mode.installMethod}
          data-mode={wizard.mode}
          data-phase={wizard.phase}
          data-platform={values.platform}
          data-ssh-path={mode.sshPath}
          data-testid="host-onboarding-flow"
        >
          {wizard.phase === "select_mode" && (
            <HostModeStep
              mode={wizard.mode}
              onSelect={requestMode}
              relayAvailable={relayScopes.length > 0}
              t={t}
            />
          )}
          {wizard.phase === "details" && (
            <form
              className="argus-wizard-details"
              id={FORM_ID}
              onSubmit={form.handleSubmit(prepare)}
            >
              <SelectedHostMode
                mode={wizard.mode}
                onChange={() => dispatch({ type: "change_mode" })}
                t={t}
              />
              {form.formState.errors.root?.message &&
                connectionTest?.status !== "failed" && (
                  <Alert
                    description={form.formState.errors.root.message}
                    title={t("hosts.wizard.previewFailed")}
                    tone="danger"
                  />
                )}
              {connectionTest?.status === "failed" && (
                <ConnectionTestSummary result={connectionTest} />
              )}
              <div className="argus-scenario-wizard__form">
                <Field
                  error={
                    form.formState.errors.name?.message ??
                    nameAvailability.error
                  }
                  hint={
                    nameAvailability.checking
                      ? t("resourceNames.checking")
                      : undefined
                  }
                  label={t("hosts.wizard.name")}
                  requirement="required"
                >
                  <Input
                    autoFocus
                    {...form.register("name", {
                      onBlur: nameAvailability.onBlur,
                    })}
                  />
                </Field>
                <Field
                  label={t("hosts.wizard.environment")}
                  requirement="required"
                >
                  <Select
                    value={values.environment}
                    onValueChange={(value) =>
                      form.setValue("environment", value as Environment)
                    }
                    options={ENVIRONMENTS.map((value) => ({
                      value,
                      label: t(`hosts.env.${value}`),
                    }))}
                  />
                </Field>
                <Field
                  label={t("hosts.wizard.platform")}
                  requirement="required"
                >
                  <Select
                    value={values.platform}
                    onValueChange={(value) => {
                      form.setValue("platform", value as "linux" | "windows");
                      if (value === "windows")
                        form.setValue("architecture", "amd64");
                    }}
                    options={[
                      { value: "linux", label: "Linux" },
                      { value: "windows", label: "Windows Server 2019+" },
                    ]}
                  />
                </Field>
                {mode.installMethod === "manual" &&
                  values.platform === "linux" && (
                    <Field
                      label={t("hosts.wizard.architecture")}
                      requirement="required"
                    >
                      <Select
                        value={values.architecture}
                        onValueChange={(value) =>
                          form.setValue(
                            "architecture",
                            value as "amd64" | "arm64",
                          )
                        }
                        options={[
                          { value: "amd64", label: "x64 / amd64" },
                          { value: "arm64", label: "arm64" },
                        ]}
                      />
                    </Field>
                  )}
                {mode.controlPath === "bastion_relay" && (
                  <Field
                    className="argus-field--wide"
                    error={form.formState.errors.scopeId?.message}
                    label={t("hosts.wizard.scope")}
                    requirement="required"
                  >
                    <Select
                      value={values.scopeId}
                      onValueChange={(value) => form.setValue("scopeId", value)}
                      options={relayScopes.map((scope) => ({
                        value: scope.id,
                        label: `${scope.name} · ${scope.relay_address}:${scope.relay_https_port}`,
                      }))}
                      placeholder={t("hosts.wizard.selectScope")}
                    />
                  </Field>
                )}
                {mode.installMethod === "ssh" && (
                  <>
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
                        value={values.credentialId}
                        onValueChange={(value) =>
                          form.setValue("credentialId", value)
                        }
                        options={credentials.map((credential) => ({
                          value: credential.id,
                          label: credential.name,
                        }))}
                      />
                    </Field>
                  </>
                )}
                <Field
                  className="argus-field--wide"
                  label={t("hosts.wizard.labels")}
                  requirement="optional"
                >
                  <Textarea rows={3} {...form.register("labelsText")} />
                </Field>
              </div>
            </form>
          )}
          {(wizard.phase === "verify" || wizard.phase === "confirm_command") &&
            pendingAction && (
              <div className="argus-dialog__flow">
                {connectionTest && (
                  <ConnectionTestSummary result={connectionTest} />
                )}
                <PendingActionConfirm
                  action={pendingAction}
                  onError={(error) => {
                    if (retryHost) return false;
                    if (
                      !previewName ||
                      !nameAvailability.isCurrent(previewName)
                    )
                      return true;
                    if (!nameAvailability.handleConflict(error, previewName))
                      return false;
                    setPendingAction(null);
                    setConnectionTest(null);
                    dispatch({ type: "back" });
                    return true;
                  }}
                  claimOneTimeResult={mode.installMethod === "manual"}
                  onCancel={() => {
                    setPendingAction(null);
                    dispatch({ type: "back" });
                  }}
                  onDone={(result) => {
                    if (
                      !retryHost &&
                      (!previewName || !nameAvailability.isCurrent(previewName))
                    )
                      return;
                    setPendingAction(null);
                    if (!notified.current) {
                      notified.current = true;
                      onCreated();
                    }
                    if (result.one_time_result) {
                      setOneTimeResult(result.one_time_result);
                      dispatch({ type: "commit_command" });
                    } else {
                      dispatch({ type: "commit_complete" });
                    }
                  }}
                />
              </div>
            )}
          {wizard.phase === "command_result" && oneTimeResult && (
            <div className="argus-dialog__flow">
              <Alert
                description={t("hosts.wizard.commandOnce")}
                title={t("hosts.wizard.commandTitle")}
                tone="warning"
              />
              <InstallInstructionPanel result={oneTimeResult} />
              <p className="argus-muted">
                {t("hosts.wizard.commandExpires", {
                  time: formatDateTime(oneTimeResult.expires_at),
                })}
              </p>
            </div>
          )}
          {wizard.phase === "completed" && (
            <Alert
              description={t("hosts.wizard.completedDesc")}
              title={t("hosts.wizard.completed")}
              tone="success"
            />
          )}
        </div>
      </Dialog>
      <ConfirmDialog
        description={t("hosts.wizard.modeSwitchWarning")}
        onConfirm={() => {
          if (pendingMode) applyMode(pendingMode);
          setPendingMode(null);
        }}
        onOpenChange={(next) => !next && setPendingMode(null)}
        open={pendingMode !== null}
        title={t("hosts.wizard.modeSwitchTitle")}
      />
    </>
  );
}

type Translate = (key: string, options?: Record<string, unknown>) => string;

function HostModeStep({
  mode,
  onSelect,
  relayAvailable,
  t,
}: {
  mode: HostOnboardingMode;
  onSelect: (mode: HostOnboardingMode) => void;
  relayAvailable: boolean;
  t: Translate;
}) {
  const renderCard = (candidate: HostOnboardingMode) => {
    const isBastion =
      HOST_MODE_CONFIG[candidate].controlPath === "bastion_relay";
    const available = !isBastion || relayAvailable;
    return (
      <ScenarioCard
        description={t(`hosts.hostMode.summaryOf.${candidate}`)}
        diagram={<HostTopology mode={candidate} t={t} />}
        key={candidate}
        onSelect={() => onSelect(candidate)}
        refLabel={t(`hosts.hostMode.referenceOf.${candidate}`)}
        selected={mode === candidate}
        status={available ? "supported" : "unavailable"}
        statusLabel={
          available
            ? t("hosts.scenario.statusSupported")
            : t("hosts.wizard.noActiveScope")
        }
        title={t(`hosts.hostMode.titleOf.${candidate}`)}
      />
    );
  };
  return (
    <div className="argus-mode-selection">
      <div className="argus-mode-selection__cards argus-scenario-wizard__list">
        <div className="argus-scenario-wizard__group">
          {t("hosts.hostMode.groupStandalone")}
        </div>
        {(["command_direct", "ssh_direct", "ssh_tunnel"] as const).map(
          renderCard,
        )}
        <div className="argus-scenario-wizard__group">
          {t("hosts.hostMode.groupBastion")}
        </div>
        {(["command_bastion", "ssh_bastion"] as const).map(renderCard)}
      </div>
      <div className="argus-mode-selection__detail">
        <h3>{t(`hosts.hostMode.titleOf.${mode}`)}</h3>
        <p>{t(`hosts.hostMode.summaryOf.${mode}`)}</p>
        <ModeGrid items={hostModeGrid(mode, t)} />
      </div>
    </div>
  );
}

function HostTopology({ mode, t }: { mode: HostOnboardingMode; t: Translate }) {
  if (mode === "command_bastion" || mode === "ssh_bastion") {
    return (
      <TopologyDiagram
        label={t(`hosts.hostMode.titleOf.${mode}`)}
        layout="member"
        links={[
          {
            mode: mode === "ssh_bastion" ? "ok" : "blocked",
            direction: mode === "ssh_bastion" ? "down" : "none",
            label:
              mode === "ssh_bastion"
                ? t("hosts.topology.sshManage")
                : t("hosts.topology.noDirect"),
            slot: "left",
          },
          {
            mode: "ok",
            direction: "up",
            label: t("hosts.topology.relayConnect"),
            slot: "right",
          },
          {
            mode: "ok",
            direction: "right",
            label: t("hosts.topology.egress"),
            slot: "h",
          },
        ]}
        nodes={[
          { label: t("hosts.topology.bastion"), kind: "bastion" },
          { label: t("hosts.topology.member") },
          { label: t("hosts.topology.argus"), kind: "argus" },
        ]}
      />
    );
  }
  const links =
    mode === "command_direct"
      ? [
          {
            mode: "ok" as const,
            direction: "right" as const,
            label: t("hosts.topology.selfPush"),
            slot: "mid" as const,
          },
        ]
      : mode === "ssh_direct"
        ? [
            {
              mode: "ok" as const,
              direction: "left" as const,
              label: t("hosts.topology.sshOk"),
              slot: "top" as const,
            },
            {
              mode: "ok" as const,
              direction: "right" as const,
              label: t("hosts.topology.pushOk"),
              slot: "mid" as const,
            },
          ]
        : [
            {
              mode: "ok" as const,
              direction: "left" as const,
              label: t("hosts.topology.sshOk"),
              slot: "top" as const,
            },
            {
              mode: "blocked" as const,
              direction: "none" as const,
              label: t("hosts.topology.noEgress"),
              slot: "mid" as const,
            },
            {
              mode: "tunnel" as const,
              direction: "right" as const,
              label: t("hosts.topology.tunnelBack"),
              slot: "bottom" as const,
            },
          ];
  return (
    <TopologyDiagram
      label={t(`hosts.hostMode.titleOf.${mode}`)}
      layout="pair"
      links={links}
      nodes={[
        { label: t("hosts.topology.host") },
        { label: t("hosts.topology.argus"), kind: "argus" },
      ]}
    />
  );
}

function hostModeGrid(mode: HostOnboardingMode, t: Translate) {
  const config = HOST_MODE_CONFIG[mode];
  return [
    {
      label: t("hosts.wizard.installMethod"),
      value:
        config.installMethod === "manual"
          ? t("hosts.wizard.commandInstall")
          : t("hosts.wizard.sshInstall"),
      tone: "info" as const,
    },
    {
      label: t("hosts.wizard.controlPath"),
      value: t(`hosts.controlPath.${config.controlPath}`),
    },
    {
      label: t("hosts.wizard.networkPrerequisite"),
      value: t(`hosts.hostMode.prerequisiteOf.${mode}`),
      tone: "warn" as const,
    },
  ];
}

function SelectedHostMode({
  mode,
  onChange,
  t,
}: {
  mode: HostOnboardingMode;
  onChange: () => void;
  t: Translate;
}) {
  return (
    <div className="argus-selected-mode">
      <div>
        <span>{t("hosts.wizard.selectedMode")}</span>
        <strong>{t(`hosts.hostMode.titleOf.${mode}`)}</strong>
      </div>
      <Button onPress={onChange} type="button" variant="ghost">
        {t("hosts.wizard.changeMode")}
      </Button>
    </div>
  );
}
