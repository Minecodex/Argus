import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type KubernetesInventoryObject,
  type PendingActionPublic,
} from "@argus/api-client";
import { Button, Dialog, Field, Input, KeyValueGrid } from "@argus/ui";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";

const schema = z.object({
  name: z
    .string()
    .min(1)
    .max(63)
    .refine(
      (value) =>
        value
          .split(".")
          .every((part) => /^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$/.test(part)),
      "kubernetesWorkbench.jobRun.invalidName",
    ),
});

export function KubernetesJobRunDialog({
  clusterId,
  namespaceId,
  source,
  onClose,
  onSubmitted,
}: {
  clusterId: string;
  namespaceId: string;
  source: KubernetesInventoryObject;
  onClose: () => void;
  onSubmitted: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const [base, setBase] = useState(source);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<PendingActionPublic>();
  const form = useForm({
    resolver: zodResolver(schema),
    defaultValues: {
      name: `${source.name.slice(0, 40).replace(/[-.]$/u, "")}-manual-${crypto.randomUUID().slice(0, 8)}`,
    },
  });
  const current = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "job-source",
      source.uid,
      namespaceId,
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.object(
        clusterId,
        {
          type: "cronjob",
          name: source.name,
          uid: source.uid,
          namespace_id: namespaceId,
        },
        signal,
      ),
    refetchOnWindowFocus: "always",
    refetchInterval: 15000,
  });
  const changed =
    !!current.data && current.data.resource_version !== base.resource_version;
  const preview = form.handleSubmit(async ({ name }) => {
    setError("");
    try {
      const latest = await current.refetch();
      if (latest.error) throw latest.error;
      if (
        !latest.data ||
        latest.data.resource_version !== base.resource_version
      ) {
        setError(t("kubernetesWorkbench.mutation.versionChanged"));
        return;
      }
      setPending(
        await api.kubernetesWorkbench.preview(clusterId, {
          operation: "create",
          type: "job",
          name,
          namespace_id: namespaceId,
          cronjob_source: {
            name: base.name,
            uid: base.uid,
            resource_version: base.resource_version,
          },
        }),
      );
    } catch (cause) {
      setError(
        formatApiError(
          cause,
          t("kubernetesWorkbench.errors.unavailable"),
          (id) => t("common.requestReference", { requestId: id }),
        ),
      );
    }
  });
  return (
    <Dialog
      open
      size="lg"
      title={t("kubernetesWorkbench.jobRun.title", { name: source.name })}
      onOpenChange={(open) => {
        if (!open && !form.formState.isSubmitting) onClose();
      }}
    >
      {pending ? (
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
      ) : (
        <form className="argus-k8s-mutation-form" onSubmit={preview}>
          <p>{t("kubernetesWorkbench.jobRun.hint")}</p>
          <KeyValueGrid
            columns={2}
            items={[
              {
                label: t("kubernetesWorkbench.namespace"),
                value: base.namespace,
              },
              { label: t("kubernetesWorkbench.uid"), value: base.uid },
              {
                label: t("kubernetesWorkbench.version"),
                value: base.resource_version,
              },
            ]}
          />
          <Field
            label={t("kubernetesWorkbench.jobRun.name")}
            requirement="required"
            error={
              form.formState.errors.name
                ? t("kubernetesWorkbench.jobRun.invalidName")
                : undefined
            }
          >
            <Input {...form.register("name")} maxLength={63} required />
          </Field>
          {changed && (
            <div role="alert">
              <p>{t("kubernetesWorkbench.mutation.versionChanged")}</p>
              <Button
                type="button"
                onPress={() => {
                  if (current.data) {
                    setBase(current.data);
                    setError("");
                  }
                }}
              >
                {t("kubernetesWorkbench.mutation.reloadObject")}
              </Button>
            </div>
          )}
          {(error || current.error) && (
            <p role="alert">
              {error ||
                formatApiError(
                  current.error,
                  t("kubernetesWorkbench.errors.unavailable"),
                (id) => t("common.requestReference", { requestId: id }),
                )}
            </p>
          )}
          <div className="argus-form-actions">
            <Button
              type="button"
              onPress={onClose}
              isDisabled={form.formState.isSubmitting}
            >
              {t("common.cancel")}
            </Button>
            <Button
              variant="primary"
              type="submit"
              isDisabled={
                form.formState.isSubmitting ||
                current.isFetching ||
                !!current.error ||
                changed
              }
            >
              {t("kubernetesWorkbench.mutation.preview")}
            </Button>
          </div>
        </form>
      )}
    </Dialog>
  );
}
