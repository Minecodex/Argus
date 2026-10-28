import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo } from "react";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";
import { formatApiError, useApi, type AIModel } from "@argus/api-client";
import { useEnterpriseAuthStore } from "@argus/auth";
import { Alert, Field, FormDrawer, Input, Select } from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { useModelGovernanceMetadata } from "./use-model-governance-metadata";

export function QuotaDrawer({
  model,
  onClose,
}: {
  model: Pick<AIModel, "id" | "name">;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const session = useEnterpriseAuthStore((state) => state.session);
  const enterpriseAdmin = usePermission("model.manage");
  const metadata = useModelGovernanceMetadata();
  const departments = useMemo(
    () => ({
      data: metadata.data?.subjects?.filter(
        (item) => item.kind === "department",
      ),
    }),
    [metadata.data],
  );
  const users = useMemo(
    () => ({
      data: metadata.data?.subjects
        ?.filter((item) => item.kind === "user")
        .map((item) => ({ ...item, display_name: item.name })),
    }),
    [metadata.data],
  );
  const queryClient = useQueryClient();
  const schema = useMemo(
    () =>
      z.object({
        subjectType: z.enum(["department", "user"]),
        subjectId: z.string().min(1, t("aiSettings.validation.required")),
        amount: z
          .number({ error: t("aiSettings.validation.nonnegative") })
          .min(0, t("aiSettings.validation.nonnegative"))
          .optional(),
      }),
    [t],
  );
  type QuotaFormValues = z.infer<typeof schema>;
  const {
    control,
    handleSubmit,
    register,
    setError,
    setValue,
    watch,
    formState: { errors },
  } = useForm<QuotaFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      subjectType: enterpriseAdmin ? "department" : "user",
      subjectId: "",
      amount: undefined,
    },
  });
  const subjectType = watch("subjectType");
  const subjects = useMemo(() => {
    if (subjectType === "department") return departments.data ?? [];
    return (users.data ?? []).filter(
      (user) =>
        enterpriseAdmin ||
        user.department_id === session?.session.department_id,
    );
  }, [
    departments.data,
    enterpriseAdmin,
    session?.session.department_id,
    subjectType,
    users.data,
  ]);
  useEffect(() => {
    setValue("subjectId", subjects[0]?.id ?? "", { shouldValidate: false });
  }, [setValue, subjectType, subjects]);
  const save = useMutation({
    mutationFn: (values: QuotaFormValues) =>
      api.models.setQuota({
        modelId: model.id,
        subjectType: values.subjectType,
        subjectId: values.subjectId,
        monthlyAmount: values.amount,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["model-quotas"] });
      onClose();
    },
    onError: (caught) => {
      setError("root", {
        message: formatApiError(
          caught,
          t("aiSettings.quota.saveFailed"),
          (requestId) => t("common.requestReference", { requestId }),
        ),
        type: "server",
      });
    },
  });
  return (
    <FormDrawer
      loading={save.isPending}
      onOpenChange={(open) => !open && onClose()}
      onSubmit={handleSubmit((values) => save.mutate(values))}
      open
      submitLabel={t("aiSettings.quota.save")}
      title={`${t("aiSettings.quota.title")} · ${model.name}`}
    >
      <div className="argus-settings-form">
        {errors.root?.message && (
          <Alert
            description={errors.root.message}
            title={t("aiSettings.quota.saveFailed")}
            tone="danger"
          />
        )}
        {enterpriseAdmin && (
          <Field
            error={errors.subjectType?.message}
            requirement="required"
            label={t("aiSettings.quota.subjectType")}
          >
            <Controller
              control={control}
              name="subjectType"
              render={({ field }) => (
                <Select
                  onValueChange={field.onChange}
                  options={[
                    {
                      value: "department",
                      label: t("aiSettings.quota.department"),
                    },
                    { value: "user", label: t("aiSettings.quota.user") },
                  ]}
                  value={field.value}
                />
              )}
            />
          </Field>
        )}
        <Field
          error={errors.subjectId?.message}
          requirement="required"
          label={t("aiSettings.quota.subject")}
        >
          <Controller
            control={control}
            name="subjectId"
            render={({ field }) => (
              <Select
                onValueChange={field.onChange}
                options={subjects.map((subject) => ({
                  value: subject.id,
                  label:
                    (subject as { name?: string; displayName?: string }).name ??
                    (subject as { displayName?: string }).displayName ??
                    subject.id,
                }))}
                value={field.value}
              />
            )}
          />
        </Field>
        <Field
          error={errors.amount?.message}
          requirement="optional"
          hint={t("aiSettings.quota.unlimited")}
          label={t("aiSettings.quota.amount")}
        >
          <Input
            {...register("amount", {
              setValueAs: (value) =>
                value === "" || value === undefined ? undefined : Number(value),
            })}
            min="0"
            step="0.01"
            type="number"
          />
        </Field>
        {!enterpriseAdmin && (
          <p className="argus-settings-section__hint">
            {t("aiSettings.quota.departmentOnly")}
          </p>
        )}
      </div>
    </FormDrawer>
  );
}
