import { createResourceDraft, hasResourceForm } from "./resource-create";
import { KubernetesHPANotice } from "./hpa-notice";
import { ResourceActionFields } from "./resource-action-fields";
import {
  applyResourcePatch,
  asRecord,
  resourcePatch,
  rebaseResource,
  ResourceRebaseConflict,
} from "./resource-patch";

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
  type ConfirmActionResult,
  type KubernetesDrainProgress,
} from "@argus/api-client";
import {
  Dialog,
  Checkbox,
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

export type KubernetesMutationEdit = {
  base?: KubernetesInventoryObject;
  draft: Record<string, unknown>;
  manifest: StructuredDocumentValue;
  replicas: string;
  capacity: string;
  structured: boolean;
  deletedKeys?: string[];
};
export function KubernetesMutationDialog({
  clusterId,
  definition,
  object,
  namespace,
  intent,
  onClose,
  onSubmitted,
  initialEdit,
}: {
  clusterId: string;
  definition: KubernetesInventoryDefinition;
  object?: KubernetesInventoryObject;
  namespace?: { id: string; name: string };
  onClose: () => void;
  intent?: string;
  initialEdit?: KubernetesMutationEdit;
  onSubmitted: (
    submission: ConfirmActionResult,
    edit: KubernetesMutationEdit,
  ) => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const canDrain = usePermission("kubernetes.node.drain");
  const [secretValues, setSecretValues] = useState<
    Array<{ id: string; key: string; value: string }>
  >([]);
  const [operation, setOperation] = useState<
      KubernetesMutationInput["operation"]
    >(
      object
        ? [
            "scale",
            "restart",
            "rollback",
            "cordon",
            "uncordon",
            "drain",
            "delete",
          ].includes(intent ?? "")
          ? (intent as KubernetesMutationInput["operation"])
          : "patch"
        : "create",
    ),
    [replicas, setReplicas] = useState(initialEdit?.replicas ?? "1"),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [pending, setPending] = useState<PendingActionPublic>();
  const [deletedKeys, setDeletedKeys] = useState<string[]>(
    initialEdit?.deletedKeys ?? [],
  );
  const [structured, setStructured] = useState(
    initialEdit?.structured ??
      (intent !== undefined &&
        intent !== "edit" &&
        hasResourceForm(definition.key)),
  );
  const [draft, setDraft] = useState<Record<string, unknown>>(
    () =>
      initialEdit?.draft ?? createResourceDraft(definition, namespace?.name),
  );
  const [capacity, setCapacity] = useState(initialEdit?.capacity ?? "");
  const [rebaseConflict, setRebaseConflict] = useState("");
  const [base, setBase] = useState<KubernetesInventoryObject | undefined>(
    initialEdit?.base,
  );
  const [rollback, setRollback] = useState<KubernetesRollbackReference>();
  const [choosingRevision, setChoosingRevision] = useState(false);
  const drainPreview = (
    (pending?.preview ?? {}) as { drain?: KubernetesDrainProgress }
  ).drain;
  const [manifest, setManifest] = useState<StructuredDocumentValue>(
    () =>
      initialEdit?.manifest ?? {
        format: "yaml",
        source: formatStructuredDocument(
          object ? {} : createResourceDraft(definition, namespace?.name),
          "yaml",
        ),
      },
  );
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
    refetchOnMount: "always",
    refetchOnWindowFocus: "always",
    refetchInterval: 15000,
  });
  const form = useForm({
    resolver: zodResolver(kubernetesMutationFormSchema),
    values: { operation, manifest, replicas },
  });
  const loadObject = useCallback((value: KubernetesInventoryObject) => {
    setBase(value);
    setDraft(editableKubernetesManifest(value.data));
    const spec = asRecord(value.data.spec);
    setReplicas(String(spec.replicas ?? 1));
    setCapacity(
      String(asRecord(asRecord(spec.resources).requests).storage ?? ""),
    );
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
    if (current.data && !base && !current.isFetching && !current.error)
      loadObject(current.data);
  }, [current.data, current.isFetching, current.error, base, loadObject]);
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
  const rebase = (keepMine = false) => {
    if (!current.data || !base) return;
    try {
      const edited = structured
        ? draft
        : applyResourcePatch(
            editableKubernetesManifest(base.data),
            parseStructuredDocument(manifest.source, manifest.format),
          );
      const merged = asRecord(
        rebaseResource(
          editableKubernetesManifest(base.data),
          edited,
          editableKubernetesManifest(current.data.data),
          keepMine,
        ),
      );
      setBase(current.data);
      setDraft(merged);
      setManifest((previous) => ({
        ...previous,
        source: formatStructuredDocument(merged, previous.format),
      }));
      setError("");
      setRebaseConflict("");
    } catch (cause) {
      if (cause instanceof ResourceRebaseConflict)
        setRebaseConflict(cause.path);
      else setError(t("kubernetesWorkbench.mutation.invalidManifest"));
    }
  };
  const preview = async () => {
    setError("");
    setBusy(true);
    try {
      if (operation === "rollback" && !rollback) {
        setError(t("kubernetesWorkbench.mutation.revisionRequired"));
        return;
      }
      let body: Record<string, unknown> | undefined;
      if (structured && (operation === "patch" || operation === "create")) {
        body = base
          ? resourcePatch(editableKubernetesManifest(base.data), draft)
          : draft;
        if (intent === "suspend" || intent === "resume")
          body = {
            spec: {
              [definition.key === "deployment" ? "paused" : "suspend"]:
                intent === "suspend",
            },
          };
        if (intent === "expand")
          body = { spec: { resources: { requests: { storage: capacity } } } };
      } else if (operation === "patch" || operation === "create") {
        try {
          const parsed = parseStructuredDocument(
            manifest.source,
            manifest.format,
          );
          body = parsed;
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
          if (deletedKeys.includes(row.key)) {
            setError(t("kubernetesResources.secretIntentConflict"));
            return;
          }
          privateValues[row.key] = row.value;
        }
      if (
        definition.key === "secret" &&
        operation === "patch" &&
        deletedKeys.length
      )
        body = {
          ...body,
          data: {
            ...asRecord(body?.data),
            ...Object.fromEntries(deletedKeys.map((key) => [key, null])),
          },
        };
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
      title={
        intent && object
          ? t("kubernetesResources.actions." + intent) + " · " + object.name
          : t(
              object
                ? "kubernetesWorkbench.mutation.manageTitle"
                : "kubernetesWorkbench.mutation.createTitle",
              { kind: definition.kind, name: object?.name ?? "" },
            )
      }
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
            completionMode="submitted"
            confirmLabel={t("kubernetesWorkbench.mutation.confirm")}
            onError={(cause) => {
              const code = (cause as { code?: string }).code;
              if (
                code === "KUBERNETES_MUTATION_CONFLICT" ||
                code === "ACTION_INVALIDATED"
              ) {
                setPending(undefined);
                setError(t("kubernetesResources.replaceWarning"));
                void current.refetch();
                return true;
              }
              return false;
            }}
            onDone={(result) => {
              onSubmitted(result, {
                base,
                draft,
                manifest,
                replicas,
                capacity,
                structured,
                deletedKeys,
              });
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
              {
                label: t("kubernetesWorkbench.name"),
                value: object?.name ?? definition.kind,
              },
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
              <Button type="button" onPress={() => rebase()}>
                {t("kubernetesResources.rebase")}
              </Button>
            </div>
          )}
          {rebaseConflict && (
            <div role="alert">
              <p>
                {t("kubernetesResources.rebaseConflict", {
                  path: rebaseConflict,
                })}
              </p>
              <Button onPress={() => rebase(true)}>
                {t("kubernetesResources.keepInput")}
              </Button>
            </div>
          )}
          {object && !intent && (
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
          {operation === "delete" &&
            definition.key === "pod" &&
            object?.owners.some((owner) => owner.controller) && (
              <p>{t("kubernetesResources.controlledPodDelete")}</p>
            )}
          {definition.key === "statefulset" &&
            ["patch", "restart", "rollback"].includes(operation) && (
              <p>{t("kubernetesResources.statefulChange")}</p>
            )}
          {operation === "drain" && (
            <p>{t("kubernetesWorkbench.drain.hint")}</p>
          )}
          {structured && (operation === "patch" || operation === "create") && (
            <ResourceActionFields
              kind={definition.key}
              intent={intent ?? "configure"}
              object={object}
              draft={draft}
              onChange={setDraft}
              capacity={capacity}
              onCapacity={setCapacity}
              disabled={busy || (!!object && !base)}
              onAdvanced={() => {
                setManifest((previous) => ({
                  ...previous,
                  source: formatStructuredDocument(draft, previous.format),
                }));
                setStructured(false);
              }}
            />
          )}
          {!structured && (operation === "patch" || operation === "create") && (
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
          {object &&
            ["scale", "restart", "rollback", "patch"].includes(operation) && (
              <KubernetesHPANotice
                clusterId={clusterId}
                object={object}
                namespaceId={namespace?.id}
              />
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
          {definition.key === "secret" && base && operation === "patch" && (
            <fieldset>
              <legend>{t("kubernetesResources.secretDelete")}</legend>
              {Object.keys(asRecord(base.data.data)).map((key) => (
                <Checkbox
                  key={key}
                  isSelected={deletedKeys.includes(key)}
                  onChange={(selected) =>
                    setDeletedKeys((old) =>
                      selected
                        ? [...old, key]
                        : old.filter((value) => value !== key),
                    )
                  }
                >
                  {key}
                </Checkbox>
              ))}
            </fieldset>
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
