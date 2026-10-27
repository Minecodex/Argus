import { useCallback, useEffect, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type KubernetesInventoryDefinition,
  type KubernetesInventoryObject,
  type KubernetesMutationInput,
  type KubernetesRollbackReference,
  type PendingActionPublic,
  type KubernetesDrainProgress,
} from "@argus/api-client";
import {
  Dialog,
  Field,
  Input,
  Select,
  StructuredDocumentEditor,
  parseStructuredDocument,
  formatStructuredDocument,
  type StructuredDocumentValue,
  ProtectedTextarea,
  Button,
  Spinner,
  KeyValueGrid,
} from "@argus/ui";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";
import { editableKubernetesManifest } from "./manifest";
import { kubernetesMutationFormSchema } from "./mutation-form";
import { RollbackPicker } from "./rollback-picker";
import { usePermission } from "../../lib/permissions";
import { KubernetesDrainProgressView } from "./drain-progress";

export function KubernetesMutationDialog({
  clusterId,
  definition,
  object,
  namespace,
  onClose,
  onSubmitted,
}: {
  clusterId: string;
  definition: KubernetesInventoryDefinition;
  object?: KubernetesInventoryObject;
  namespace?: { id: string; name: string };
  onClose: () => void;
  onSubmitted: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const canDrain = usePermission("kubernetes.node.drain");
  const [secretValues, setSecretValues] = useState<
    Array<{ id: string; key: string; value: string }>
  >([]);
  const [operation, setOperation] = useState<
      KubernetesMutationInput["operation"]
    >(object ? "patch" : "create"),
    [replicas, setReplicas] = useState("1"),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [pending, setPending] = useState<PendingActionPublic>();
  const [base, setBase] = useState<KubernetesInventoryObject>();
  const [rollback, setRollback] = useState<KubernetesRollbackReference>();
  const [choosingRevision, setChoosingRevision] = useState(false);
  const drainPreview = (
    (pending?.preview ?? {}) as { drain?: KubernetesDrainProgress }
  ).drain;
  const [manifest, setManifest] = useState<StructuredDocumentValue>(() => ({
    format: "yaml",
    source: formatStructuredDocument(
      object
        ? {}
        : {
            apiVersion: definition.group
              ? `${definition.group}/${definition.version}`
              : definition.version,
            kind: definition.kind,
            metadata: {
              name: "",
              ...(namespace ? { namespace: namespace.name } : {}),
            },
          },
      "yaml",
    ),
  }));
  const current = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "mutation-object",
      object?.uid,
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.object(
        clusterId,
        {
          type: definition.key,
          name: object!.name,
          uid: object!.uid,
          namespace_id: namespace?.id,
        },
        signal,
      ),
    enabled: !!object,
    refetchOnWindowFocus: "always",
    refetchInterval: 15000,
  });
  const form = useForm({
    resolver: zodResolver(kubernetesMutationFormSchema),
    values: { operation, manifest, replicas },
  });
  const loadObject = useCallback((value: KubernetesInventoryObject) => {
    setBase(value);
    setRollback(undefined);
    setManifest((previous) => ({
      ...previous,
      source: formatStructuredDocument(
        editableKubernetesManifest(value.data),
        previous.format,
      ),
    }));
    setError("");
  }, []);
  useEffect(() => {
    if (current.data && !base) loadObject(current.data);
  }, [current.data, base, loadObject]);
  const serverChanged =
    !!base &&
    !!current.data &&
    base.resource_version !== current.data.resource_version;
  const options: KubernetesMutationInput["operation"][] = object
    ? ["patch", "delete"]
    : ["create"];
  if (
    object &&
    ["deployment", "statefulset", "replicaset"].includes(definition.key)
  )
    options.push("scale");
  if (
    object &&
    ["deployment", "statefulset", "daemonset"].includes(definition.key)
  )
    options.push("restart", "rollback");
  if (object && definition.key === "node") options.push("cordon", "uncordon");
  if (object && definition.key === "node" && canDrain) options.push("drain");
  const preview = async () => {
    setError("");
    setBusy(true);
    try {
      if (operation === "rollback" && !rollback) {
        setError(t("kubernetesWorkbench.mutation.revisionRequired"));
        return;
      }
      let body: Record<string, unknown> | undefined;
      if (operation === "patch" || operation === "create") {
        try {
          body = parseStructuredDocument(manifest.source, manifest.format);
        } catch {
          setError(t("kubernetesWorkbench.mutation.invalidManifest"));
          return;
        }
      }
      const name =
        object?.name ??
        String(
          (body?.metadata as Record<string, unknown> | undefined)?.name ?? "",
        );
      if (!name) {
        setError(t("kubernetesWorkbench.mutation.nameRequired"));
        return;
      }
      if (object) {
        const latest = await current.refetch();
        if (latest.error) throw latest.error;
        if (!base || latest.data?.resource_version !== base.resource_version) {
          setError(t("kubernetesWorkbench.mutation.versionChanged"));
          return;
        }
      }
      const privateValues: Record<string, string> = {};
      if (
        definition.key === "secret" &&
        (operation === "create" || operation === "patch")
      )
        for (const row of secretValues) {
          if (!row.key || Object.hasOwn(privateValues, row.key)) {
            setError(t("kubernetesWorkbench.mutation.secretKeyInvalid"));
            return;
          }
          privateValues[row.key] = row.value;
        }
      setPending(
        await api.kubernetesWorkbench.preview(clusterId, {
          operation,
          type: definition.key,
          name,
          namespace_id: namespace?.id,
          uid: base?.uid,
          expected_resource_version: base?.resource_version,
          object: body,
          replicas: operation === "scale" ? Number(replicas) : undefined,
          rollback: operation === "rollback" ? rollback : undefined,
          secret_values: Object.keys(privateValues).length
            ? privateValues
            : undefined,
        }),
      );
      setSecretValues([]);
    } catch (cause) {
      setError(
        formatApiError(
          cause,
          t("kubernetesWorkbench.errors.unavailable"),
          (id) => t("common.requestReference", { requestId: id }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <Dialog
      open
      size="lg"
      title={t(
        object
          ? "kubernetesWorkbench.mutation.manageTitle"
          : "kubernetesWorkbench.mutation.createTitle",
        { kind: definition.kind, name: object?.name ?? "" },
      )}
      onOpenChange={(open) => {
        if (!open && !busy) onClose();
      }}
    >
      {pending ? (
        <>
          {drainPreview && (
            <KubernetesDrainProgressView progress={drainPreview} />
          )}
          <PendingActionConfirm
            action={pending}
            confirmLabel={t("kubernetesWorkbench.mutation.confirm")}
            onDone={() => {
              onSubmitted();
              onClose();
            }}
            onCancel={onClose}
            onDismiss={onClose}
          />
        </>
      ) : (
        <form
          className="argus-k8s-mutation-form"
          onSubmit={form.handleSubmit(
            () => void preview(),
            (errors) =>
              setError(
                t(
                  errors.manifest?.source?.message ??
                    errors.replicas?.message ??
                    "kubernetesWorkbench.mutation.invalidManifest",
                ),
              ),
          )}
        >
          <KeyValueGrid
            columns={2}
            items={[
              {
                label: t("kubernetesWorkbench.namespace"),
                value:
                  namespace?.name ??
                  t("kubernetesWorkbench.categories.cluster"),
              },
              ...(object
                ? [
                    { label: t("kubernetesWorkbench.uid"), value: object.uid },
                    {
                      label: t("kubernetesWorkbench.version"),
                      value: base?.resource_version ?? "—",
                    },
                  ]
                : []),
            ]}
          />
          {object && current.isPending && (
            <Spinner label={t("common.loading")} />
          )}
          {current.error && (
            <p role="alert">
              {formatApiError(
                current.error,
                t("kubernetesWorkbench.errors.unavailable"),
                (id) => t("common.requestReference", { requestId: id }),
              )}
            </p>
          )}
          {serverChanged && (
            <div role="alert">
              <p>{t("kubernetesWorkbench.mutation.versionChanged")}</p>
              <Button
                type="button"
                onPress={() => current.data && loadObject(current.data)}
              >
                {t("kubernetesWorkbench.mutation.reloadObject")}
              </Button>
            </div>
          )}
          {object && (
            <Field
              label={t("kubernetesWorkbench.mutation.operation")}
              requirement="required"
            >
              <Select
                value={operation}
                options={options.map((value) => ({
                  value,
                  label: t(`kubernetesWorkbench.mutation.actions.${value}`),
                }))}
                onValueChange={(value) =>
                  setOperation(value as KubernetesMutationInput["operation"])
                }
              />
            </Field>
          )}
          {operation === "drain" && (
            <p>{t("kubernetesWorkbench.drain.hint")}</p>
          )}
          {(operation === "patch" || operation === "create") && (
            <Field
              label={t(
                operation === "patch"
                  ? "kubernetesWorkbench.mutation.patch"
                  : "kubernetesWorkbench.mutation.manifest",
              )}
              requirement="required"
              hint={
                operation === "patch"
                  ? t("kubernetesWorkbench.mutation.patchHint")
                  : undefined
              }
            >
              <StructuredDocumentEditor
                label={t("kubernetesWorkbench.mutation.manifest")}
                value={manifest}
                onChange={setManifest}
                disabled={busy || (!!object && !base)}
                required
              />
            </Field>
          )}
          {operation === "scale" && (
            <Field
              label={t("kubernetesWorkbench.mutation.replicas")}
              requirement="required"
            >
              <Input
                type="number"
                min={0}
                max={10000}
                step={1}
                value={replicas}
                onChange={(e) => setReplicas(e.target.value)}
                required
              />
            </Field>
          )}
          {operation === "rollback" && base && namespace && (
            <Field
              label={t("kubernetesWorkbench.mutation.revision")}
              requirement="required"
              hint={t("kubernetesWorkbench.mutation.rollbackHint")}
            >
              <Button
                type="button"
                onPress={() => setChoosingRevision(true)}
                isDisabled={busy}
              >
                {rollback
                  ? `${rollback.revision} · ${rollback.name}`
                  : t("kubernetesWorkbench.mutation.chooseRevision")}
              </Button>
              {choosingRevision && (
                <RollbackPicker
                  clusterId={clusterId}
                  namespaceId={namespace.id}
                  workload={base}
                  onSelect={setRollback}
                  onClose={() => setChoosingRevision(false)}
                />
              )}
            </Field>
          )}
          {definition.key === "secret" &&
            (operation === "create" || operation === "patch") && (
              <section
                className="argus-k8s-mutation-form"
                aria-label={t("kubernetesWorkbench.mutation.secretValues")}
              >
                <p>{t("kubernetesWorkbench.mutation.secretHint")}</p>
                {secretValues.map((row, index) => (
                  <div className="argus-k8s-mutation-form" key={row.id}>
                    <Field
                      label={t("kubernetesWorkbench.mutation.secretKey", {
                        index: index + 1,
                      })}
                      requirement="required"
                    >
                      <Input
                        value={row.key}
                        onChange={(e) =>
                          setSecretValues((values) =>
                            values.map((v) =>
                              v.id === row.id
                                ? { ...v, key: e.target.value }
                                : v,
                            ),
                          )
                        }
                        maxLength={253}
                        required
                      />
                    </Field>
                    <Field
                      label={t("kubernetesWorkbench.mutation.secretValue", {
                        index: index + 1,
                      })}
                      requirement="optional"
                    >
                      <ProtectedTextarea
                        rows={3}
                        value={row.value}
                        onChange={(e) =>
                          setSecretValues((values) =>
                            values.map((v) =>
                              v.id === row.id
                                ? { ...v, value: e.target.value }
                                : v,
                            ),
                          )
                        }
                        maxLength={65536}
                      />
                    </Field>
                    <Button
                      onPress={() =>
                        setSecretValues((values) =>
                          values.filter((v) => v.id !== row.id),
                        )
                      }
                    >
                      {t("kubernetesWorkbench.mutation.removeSecretKey")}
                    </Button>
                  </div>
                ))}
                <Button
                  onPress={() =>
                    setSecretValues((values) => [
                      ...values,
                      { id: crypto.randomUUID(), key: "", value: "" },
                    ])
                  }
                  isDisabled={secretValues.length >= 128}
                >
                  {t("kubernetesWorkbench.mutation.addSecretKey")}
                </Button>
              </section>
            )}
          <p>{t("kubernetesWorkbench.mutation.previewHint")}</p>
          {error && <p role="alert">{error}</p>}
          <div className="argus-form-actions">
            <Button onPress={onClose} isDisabled={busy}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              isDisabled={
                busy ||
                (!!object && (!base || serverChanged || current.isFetching))
              }
              variant="primary"
            >
              {t("kubernetesWorkbench.mutation.preview")}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
