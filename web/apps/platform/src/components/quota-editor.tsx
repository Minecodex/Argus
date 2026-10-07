import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";
import {
  formConstraint,
  formatApiError,
  useApi,
  type EnterpriseSandboxQuota,
} from "@argus/api-client";
import { Alert, Button, Field, FormDrawer, Input, Spinner } from "@argus/ui";

const quotaConstraints = {
  concurrent: formConstraint("SandboxQuotaWrite", "max_concurrent_sessions"),
  sessionSeconds: formConstraint(
    "SandboxQuotaWrite",
    "monthly_session_seconds",
  ),
};

/**
 * 企业 Sandbox 配额编辑抽屉统一拥有加载、校验和提交的单一表单。
 * 未配置企业从零额度开始，由管理员明确保存后生效。
 */
export function QuotaEditor({
  enterpriseId,
  enterpriseName,
  onClose,
}: {
  enterpriseId: string;
  enterpriseName: string;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const queryClient = useQueryClient();

  const quota = useQuery({
    queryKey: ["platform", "quota", enterpriseId],
    queryFn: () => api.platform.quotas.get(enterpriseId),
    retry: false,
  });

  const schema = z.object({
    maxConcurrentSessions: z
      .number()
      .int()
      .min(quotaConstraints.concurrent.minimum ?? 0)
      .max(quotaConstraints.concurrent.maximum ?? 10000),
    monthlySessionSeconds: z
      .number()
      .int()
      .min(quotaConstraints.sessionSeconds.minimum ?? 0),
  });
  type QuotaFormValues = z.infer<typeof schema>;
  const {
    handleSubmit,
    register,
    reset,
    formState: { errors },
  } = useForm<QuotaFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      maxConcurrentSessions: 0,
      monthlySessionSeconds: 0,
    },
  });
  useEffect(() => {
    if (quota.data) reset(quota.data);
  }, [quota.data, reset]);

  const save = useMutation({
    mutationFn: (input: QuotaFormValues) =>
      api.platform.quotas.update(enterpriseId, {
        expectedVersion: quota.data?.version ?? 0,
        maxConcurrentSessions: input.maxConcurrentSessions,
        monthlySessionSeconds: input.monthlySessionSeconds,
      }),
    onSuccess: (updated) => {
      queryClient.setQueryData(["platform", "quota", enterpriseId], updated);
      void queryClient.invalidateQueries({ queryKey: ["platform", "quota"] });
    },
  });

  const numberField = (
    label: string,
    key: keyof Omit<EnterpriseSandboxQuota, "enterpriseId" | "version">,
  ) => (
    <Field error={errors[key]?.message} requirement="required" label={label}>
      <Input
        {...register(key, { valueAsNumber: true })}
        min={0}
        type="number"
      />
    </Field>
  );

  const canEdit = !!quota.data && !quota.isPending && !quota.isError;

  return (
    <FormDrawer
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={`${t("sandbox.quotas.edit")} — ${enterpriseName}`}
      width={560}
      loading={save.isPending}
      submitLabel={t("common.save")}
      cancelLabel={t("common.close")}
      onSubmit={
        canEdit ? handleSubmit((values) => save.mutate(values)) : undefined
      }
      footer={
        !canEdit ? (
          <Button onPress={onClose} variant="secondary">
            {t("common.close")}
          </Button>
        ) : undefined
      }
    >
      {quota.isPending ? (
        <Spinner />
      ) : quota.isError || !quota.data ? (
        <Alert
          description={t("sandbox.quotas.loadFailed")}
          title={t("sandbox.quotas.edit")}
          tone="warning"
        />
      ) : (
        <div className="argus-quota-editor">
          {save.isError && (
            <Alert
              description={formatApiError(
                save.error,
                t("sandbox.form.saveFailed"),
                (requestId) => t("common.requestReference", { requestId }),
              )}
              title={t("sandbox.form.saveFailed")}
              tone="danger"
            />
          )}
          <Alert
            title={t("sandbox.quotas.retentionTitle")}
            description={t("sandbox.quotas.retentionDescription")}
          />
          <div className="argus-form-grid">
            {numberField(
              t("sandbox.quotas.table.concurrent"),
              "maxConcurrentSessions",
            )}
            {numberField(
              t("sandbox.quotas.table.monthlySeconds"),
              "monthlySessionSeconds",
            )}
          </div>
        </div>
      )}
    </FormDrawer>
  );
}
