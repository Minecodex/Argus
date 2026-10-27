import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type KubernetesHelmPreviewInput,
  type KubernetesHelmRelease,
  type PendingActionPublic,
} from "@argus/api-client";
import {
  Button,
  Dialog,
  Field,
  Input,
  KeyValueGrid,
  ProtectedTextarea,
  Spinner,
} from "@argus/ui";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";
import type { NamespaceChoice } from "./namespace-picker";
import {
  HelmSourceFields,
  type HelmSource,
  type HelmSourceKind,
  type HelmSourceCredentials,
} from "./helm-source-fields";

const schema = z.object({
  name: z
    .string()
    .min(1)
    .max(53)
    .regex(/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/),
  reason: z.string().trim().min(1).max(500),
  timeout: z.coerce.number().int().min(10).max(240),
});
async function chartBase64(file: File) {
  if (!file.name.endsWith(".tgz") || file.size > 384 * 1024 || file.size === 0)
    throw new Error("chart");
  const data = new Uint8Array(await file.arrayBuffer());
  try {
    let raw = "";
    for (let i = 0; i < data.length; i += 8192)
      raw += String.fromCharCode(...data.subarray(i, i + 8192));
    return btoa(raw);
  } finally {
    data.fill(0);
  }
}
type PreviewObject = {
  definition: { kind: string };
  namespace: string;
  name: string;
  action: string;
  keep: boolean;
  validation: string;
  hook?: string;
  changes?: string[];
};
export function HelmMutationDialog({
  clusterId,
  namespace,
  operation,
  release,
  revision,
  onClose,
  onSubmitted,
}: {
  clusterId: string;
  namespace: NamespaceChoice;
  operation: KubernetesHelmPreviewInput["operation"];
  release?: KubernetesHelmRelease;
  revision?: number;
  onClose: () => void;
  onSubmitted: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const form = useForm<
    z.input<typeof schema>,
    unknown,
    z.output<typeof schema>
  >({
    resolver: zodResolver(schema),
    defaultValues: { name: release?.name ?? "", reason: "", timeout: 120 },
  });
  const [file, setFile] = useState<File>(),
    [sourceKind, setSourceKind] = useState<HelmSourceKind>("upload"),
    [source, setSource] = useState<HelmSource>({
      kind: "repository",
      url: "",
      version: release?.chart_version ?? "",
      chart: release?.chart ?? "",
    }),
    [sourceCredentials, setSourceCredentials] = useState<HelmSourceCredentials>(
      {},
    ),
    [values, setValues] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [pending, setPending] = useState<PendingActionPublic>();
  const needsChart = operation === "install" || operation === "upgrade";
  const objects =
    (pending?.preview as { objects?: PreviewObject[] } | undefined)?.objects ??
    [];
  const preview = form.handleSubmit(
    async (fields) => {
      setError("");
      setBusy(true);
      try {
        if (needsChart && sourceKind === "upload" && !file)
          throw new Error("chart");
        if (new TextEncoder().encode(values).length > 96 * 1024)
          throw new Error("values");
        const input: KubernetesHelmPreviewInput = {
          namespace_id: namespace.id,
          operation,
          name: fields.name,
          reason: fields.reason,
          timeout_seconds: fields.timeout,
          expected_revision: release?.revision ?? 0,
          ...(revision ? { rollback_revision: revision } : {}),
          ...(needsChart
            ? sourceKind === "upload"
              ? { chart_archive: await chartBase64(file!), values }
              : {
                  source: {
                    kind: sourceKind,
                    url: source.url.trim(),
                    version: source.version.trim(),
                    ...(sourceKind === "repository"
                      ? { chart: source.chart?.trim() }
                      : {}),
                    ...(source.digest?.trim()
                      ? { digest: source.digest.trim() }
                      : {}),
                    ...(source.auth_url?.trim() && sourceKind === "oci"
                      ? { auth_url: source.auth_url.trim() }
                      : {}),
                    plain_http: source.plain_http ?? false,
                  },
                  source_credentials: sourceCredentials,
                  values,
                }
            : {}),
        };
        try {
          setPending(
            await api.kubernetesWorkbench.previewHelm(clusterId, input),
          );
          setValues("");
          setFile(undefined);
        } finally {
          delete input.chart_archive;
          delete input.values;
          delete input.source_credentials;
          setSourceCredentials({});
        }
      } catch (e) {
        setError(
          e instanceof Error &&
            (e.message === "chart" || e.message === "values")
            ? t(`kubernetesHelm.errors.${e.message}`)
            : formatApiError(
                e,
                t("kubernetesWorkbench.errors.unavailable"),
                (id) => t("common.requestReference", { requestId: id }),
              ),
        );
      } finally {
        setBusy(false);
      }
    },
    () => setError(t("kubernetesHelm.errors.form")),
  );
  return (
    <Dialog
      open
      size="lg"
      title={t("kubernetesHelm.changeTitle", {
        action: t(`kubernetesHelm.actions.${operation}`),
      })}
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      <p>{t("kubernetesHelm.sensitiveHint")}</p>
      <KeyValueGrid
        columns={2}
        items={[
          { label: t("kubernetesWorkbench.namespace"), value: namespace.name },
          {
            label: t("kubernetesHelm.expectedRevision"),
            value: String(release?.revision ?? 0),
          },
          ...(revision
            ? [
                {
                  label: t("kubernetesHelm.targetRevision"),
                  value: String(revision),
                },
              ]
            : []),
        ]}
      />
      {error && <p role="alert">{error}</p>}
      {pending ? (
        <>
          {(
            pending.preview as {
              source?: {
                kind: string;
                chart: string;
                version: string;
                digest: string;
                archive_digest: string;
              };
            }
          )?.source && (
            <KeyValueGrid
              columns={1}
              items={Object.entries(
                (pending.preview as { source: Record<string, string> }).source,
              )
                .filter(([key]) =>
                  [
                    "kind",
                    "origin",
                    "transport",
                    "auth_url",
                    "chart",
                    "version",
                    "digest",
                    "archive_digest",
                  ].includes(key),
                )
                .map(([key, value]) => ({
                  label: t(`kubernetesHelm.provenance.${key}`),
                  value:
                    key === "kind"
                      ? t(`kubernetesHelm.sourceKinds.${value}`)
                      : value,
                }))}
            />
          )}
          <ul>
            {objects.map((o, i) => (
              <li key={i}>
                {t(`kubernetesHelm.objectActions.${o.action}`, {
                  defaultValue: o.action,
                })}{" "}
                · {o.definition.kind} · {o.namespace}/{o.name}
                {o.keep ? ` · ${t("kubernetesHelm.keep")}` : ""}
                {o.hook ? ` · ${t("kubernetesHelm.hook")}: ${o.hook}` : ""}
                {o.validation !== "server"
                  ? ` · ${t("kubernetesHelm.deferred")}`
                  : ""}
              </li>
            ))}
          </ul>
          <PendingActionConfirm
            action={pending}
            confirmLabel={t("kubernetesWorkbench.mutation.confirm")}
            onDone={onSubmitted}
            onCancel={onClose}
            onDismiss={onClose}
          />
        </>
      ) : (
        <form className="argus-k8s-mutation-form" onSubmit={preview}>
          <Field label={t("kubernetesHelm.name")} requirement="required">
            <Input
              {...form.register("name")}
              readOnly={!!release}
              maxLength={53}
            />
          </Field>
          {needsChart && (
            <>
              <HelmSourceFields
                kind={sourceKind}
                source={source}
                credentials={sourceCredentials}
                onSource={setSource}
                onCredentials={setSourceCredentials}
                onKind={(kind) => {
                  setSourceKind(kind);
                  setFile(undefined);
                  setSourceCredentials({});
                  setSource((previous) => ({
                    ...previous,
                    kind: kind === "upload" ? "repository" : kind,
                    url: "",
                    auth_url: "",
                    digest: "",
                  }));
                }}
              />
              {sourceKind === "upload" && (
                <Field
                  label={t("kubernetesHelm.chartArchive")}
                  requirement="required"
                >
                  <Input
                    type="file"
                    accept=".tgz,application/gzip"
                    onChange={(e) => setFile(e.target.files?.[0])}
                  />
                </Field>
              )}
              <Field label={t("kubernetesHelm.values")} requirement="optional">
                <ProtectedTextarea
                  value={values}
                  onChange={(e) => setValues(e.target.value)}
                  rows={8}
                  autoComplete="off"
                  spellCheck={false}
                />
              </Field>
            </>
          )}
          <Field label={t("kubernetesHelm.timeout")} requirement="required">
            <Input
              type="number"
              min={10}
              max={240}
              {...form.register("timeout")}
            />
          </Field>
          <Field label={t("kubernetesHelm.reason")} requirement="required">
            <Input maxLength={500} {...form.register("reason")} />
          </Field>
          {operation === "uninstall" && (
            <p>{t("kubernetesHelm.uninstallHint")}</p>
          )}
          {busy && <Spinner label={t("common.loading")} />}
          <Button type="submit" variant="primary" isDisabled={busy}>
            {t("kubernetesWorkbench.mutation.preview")}
          </Button>
        </form>
      )}
    </Dialog>
  );
}
