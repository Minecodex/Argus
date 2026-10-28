import { useNavigate } from "@tanstack/react-router";
import { DepartmentQuotaModels } from "../components/settings/department-quota-models";
import { zodResolver } from "@hookform/resolvers/zod";
import { ActionMenu, QueryBoundary } from "@argus/ui";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";
import { Plus } from "lucide-react";
import {
  apiErrorPresentation,
  formConstraint,
  formatApiError,
  useApi,
  type AIModel,
} from "@argus/api-client";
import {
  Badge,
  Alert,
  Button,
  DataTable,
  EmptyState,
  Field,
  FormDrawer,
  Input,
  Select,
  StatusBadge,
} from "@argus/ui";
import { QuotaDrawer } from "../components/settings/model-quota-drawer";
import { usePermission } from "../lib/permissions";
import "../styles/ai-settings.css";

type ModelDrawerState =
  { mode: "create" } | { mode: "edit"; model: AIModel } | null;

type ModelFormValues = {
  name: string;
  baseUrl: string;
  apiKey: string;
  modelId: string;
  apiProtocol: "chat_completions" | "responses";
  contextWindowTokens: number;
  maxOutputTokens: number;
  inputPricePerMillionTokens: number;
  outputPricePerMillionTokens: number;
};

const modelConstraints = {
  name: formConstraint("AIModelTestCreate", "name"),
  baseUrl: formConstraint("AIModelTestCreate", "base_url"),
  apiKey: formConstraint("AIModelTestCreate", "api_key"),
  contextWindowTokens: formConstraint(
    "AIModelTestCreate",
    "context_window_tokens",
  ),
  maxOutputTokens: formConstraint("AIModelTestCreate", "max_output_tokens"),
  modelId: formConstraint("AIModelTestCreate", "model_id"),
  inputPrice: formConstraint("AIModelTestCreate", "input_price_per_million"),
  outputPrice: formConstraint("AIModelTestCreate", "output_price_per_million"),
};

const modelFieldByApiName: Record<string, keyof ModelFormValues> = {
  api_key: "apiKey",
  api_protocol: "apiProtocol",
  base_url: "baseUrl",
  context_window_tokens: "contextWindowTokens",
  input_price_per_million: "inputPricePerMillionTokens",
  max_output_tokens: "maxOutputTokens",
  model_id: "modelId",
  name: "name",
  output_price_per_million: "outputPricePerMillionTokens",
};

export function ModelSettingsPanel() {
  const { t } = useTranslation();
  const api = useApi();
  const navigate = useNavigate();
  const canManageModels = usePermission("model.manage");
  const canReadUsage = usePermission("model.usage.read");
  const canManageQuota = usePermission("model.quota.manage");
  const [failure, setFailure] = useState("");
  const openDashboard = async (modelId?: string) => {
    try {
      setFailure("");
      const item = (await api.dashboards.list()).find(
        (item) => item.managed_template_key === "argus.model-governance",
      );
      if (!item) throw new Error(t("modelGovernance.unavailable"));
      await navigate({
        to: "/dashboards/$dashboardId",
        params: { dashboardId: item.id },
        search: { model: modelId },
      });
    } catch (error) {
      setFailure(
        formatApiError(error, t("modelGovernance.unavailable"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    }
  };
  return (
    <div className="argus-settings-stack">
      <p className="argus-settings-section__hint">
        {t("aiSettings.description")}
      </p>
      {failure && (
        <Alert
          tone="danger"
          title={t("modelGovernance.unavailable")}
          description={failure}
        />
      )}
      {canReadUsage && (
        <div className="argus-ai-toolbar">
          <Button onPress={() => void openDashboard()}>
            {t("modelGovernance.open")}
          </Button>
        </div>
      )}
      {canManageModels ? (
        <ModelsView onDashboard={(id) => void openDashboard(id)} />
      ) : canManageQuota ? (
        <DepartmentQuotaModels onDashboard={(id) => void openDashboard(id)} />
      ) : null}
    </div>
  );
}

function ModelsView({ onDashboard }: { onDashboard: (id: string) => void }) {
  const { t } = useTranslation();
  const api = useApi();
  const canManageQuota = usePermission("model.quota.manage");
  const canReadUsage = usePermission("model.usage.read");
  const queryClient = useQueryClient();
  const [drawer, setDrawer] = useState<ModelDrawerState>(null);
  const [quotaModel, setQuotaModel] = useState<AIModel | null>(null);
  const models = useQuery({
    queryKey: ["models"],
    queryFn: () => api.models.list(),
  });
  const update = useMutation({
    mutationFn: ({
      id,
      patch,
    }: {
      id: string;
      patch: Parameters<typeof api.models.update>[1];
    }) => api.models.update(id, patch),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: ["models"] }),
  });
  return (
    <div className="argus-ai-stack">
      <div className="argus-ai-toolbar">
        <span />
        <Button onPress={() => setDrawer({ mode: "create" })} variant="primary">
          <Plus size={15} />
          {t("aiSettings.model.add")}
        </Button>
      </div>
      <QueryBoundary query={models}>
        {(models.data ?? []).length === 0 ? (
          <EmptyState description="" title={t("aiSettings.model.empty")} />
        ) : (
          <DataTable<AIModel & Record<string, unknown>>
            columns={[
              {
                key: "name",
                header: t("aiSettings.model.name"),
                render: (model) => (
                  <div className="argus-ai-model-name">
                    <b>{model.name}</b>
                    <small>{model.modelId}</small>
                  </div>
                ),
              },
              {
                key: "baseUrl",
                header: t("aiSettings.model.baseUrl"),
                render: (model) => <code>{model.baseUrl}</code>,
              },
              {
                key: "inputPrice",
                header: t("aiSettings.model.inputPrice"),
                render: (model) => model.inputPricePerMillionTokens.toFixed(2),
              },
              {
                key: "outputPrice",
                header: t("aiSettings.model.outputPrice"),
                render: (model) => model.outputPricePerMillionTokens.toFixed(2),
              },
              {
                key: "health",
                header: t("settings.common.status"),
                render: (model) => (
                  <StatusBadge
                    tone={
                      model.healthStatus === "healthy" ? "success" : "danger"
                    }
                  >
                    {t(`aiSettings.model.${model.healthStatus}`)}
                  </StatusBadge>
                ),
              },
              {
                key: "revision",
                header: t("aiSettings.model.revisionLabel"),
                render: (model) => <Badge>{model.revision}</Badge>,
              },
              {
                key: "actions",
                header: t("settings.common.actions"),
                render: (model) => (
                  <ActionMenu
                    items={[
                      {
                        label: t("aiSettings.model.test"),
                        onSelect: () =>
                          void api.models
                            .test(model.id)
                            .then(() => models.refetch()),
                      },
                      {
                        label: t("aiSettings.model.edit"),
                        onSelect: () => setDrawer({ mode: "edit", model }),
                      },
                      ...(canManageQuota
                        ? [
                            {
                              label: t("aiSettings.model.quota"),
                              onSelect: () => setQuotaModel(model),
                            },
                          ]
                        : []),
                      ...(canReadUsage
                        ? [
                            {
                              label: t("aiSettings.model.dashboard"),
                              onSelect: () => onDashboard(model.id),
                            },
                          ]
                        : []),
                      {
                        label: model.enabled
                          ? t("aiSettings.model.disable")
                          : t("aiSettings.model.enable"),
                        onSelect: () =>
                          update.mutate({
                            id: model.id,
                            patch: { enabled: !model.enabled },
                          }),
                        disabled: update.isPending,
                      },
                    ]}
                  />
                ),
              },
            ]}
            data={
              (models.data ?? []) as Array<AIModel & Record<string, unknown>>
            }
            getRowKey={(model) => model.id}
          />
        )}
      </QueryBoundary>
      {drawer && <ModelDrawer onClose={() => setDrawer(null)} state={drawer} />}
      {quotaModel && (
        <QuotaDrawer model={quotaModel} onClose={() => setQuotaModel(null)} />
      )}
    </div>
  );
}

function ModelDrawer({
  state,
  onClose,
}: {
  state: NonNullable<ModelDrawerState>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const queryClient = useQueryClient();
  const existing = state.mode === "edit" ? state.model : undefined;
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("aiSettings.validation.required"))
          .max(
            modelConstraints.name.maxLength ?? 128,
            t("aiSettings.validation.maxLength", {
              max: modelConstraints.name.maxLength ?? 128,
            }),
          ),
        baseUrl: z
          .string()
          .trim()
          .min(1, t("aiSettings.validation.required"))
          .max(
            modelConstraints.baseUrl.maxLength ?? 2048,
            t("aiSettings.validation.maxLength", {
              max: modelConstraints.baseUrl.maxLength ?? 2048,
            }),
          )
          .refine((value) => {
            try {
              const url = new URL(value);
              return url.protocol === "http:" || url.protocol === "https:";
            } catch {
              return false;
            }
          }, t("aiSettings.validation.invalidUrl")),
        apiKey: existing
          ? z.string().max(
              modelConstraints.apiKey.maxLength ?? 8192,
              t("aiSettings.validation.maxLength", {
                max: modelConstraints.apiKey.maxLength ?? 8192,
              }),
            )
          : z
              .string()
              .min(1, t("aiSettings.validation.required"))
              .max(
                modelConstraints.apiKey.maxLength ?? 8192,
                t("aiSettings.validation.maxLength", {
                  max: modelConstraints.apiKey.maxLength ?? 8192,
                }),
              ),
        modelId: z
          .string()
          .trim()
          .min(1, t("aiSettings.validation.required"))
          .max(
            modelConstraints.modelId.maxLength ?? 256,
            t("aiSettings.validation.maxLength", {
              max: modelConstraints.modelId.maxLength ?? 256,
            }),
          ),
        apiProtocol: z.enum(["chat_completions", "responses"]),
        contextWindowTokens: z
          .number({ error: t("aiSettings.validation.required") })
          .int(t("aiSettings.validation.integer"))
          .min(
            modelConstraints.contextWindowTokens.minimum ?? 8192,
            t("aiSettings.validation.minimum", {
              min: modelConstraints.contextWindowTokens.minimum ?? 8192,
            }),
          ),
        maxOutputTokens: z
          .number({ error: t("aiSettings.validation.required") })
          .int(t("aiSettings.validation.integer"))
          .min(
            modelConstraints.maxOutputTokens.minimum ?? 1,
            t("aiSettings.validation.minimum", {
              min: modelConstraints.maxOutputTokens.minimum ?? 1,
            }),
          ),
        inputPricePerMillionTokens: z
          .number({ error: t("aiSettings.validation.required") })
          .min(
            modelConstraints.inputPrice.minimum ?? 0,
            t("aiSettings.validation.minimum", {
              min: modelConstraints.inputPrice.minimum ?? 0,
            }),
          ),
        outputPricePerMillionTokens: z
          .number({ error: t("aiSettings.validation.required") })
          .min(
            modelConstraints.outputPrice.minimum ?? 0,
            t("aiSettings.validation.minimum", {
              min: modelConstraints.outputPrice.minimum ?? 0,
            }),
          ),
      }),
    [existing, t],
  );
  const {
    control,
    register,
    handleSubmit,
    setError,
    formState: { errors },
  } = useForm<ModelFormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: existing?.name ?? "",
      baseUrl: existing?.baseUrl ?? "",
      apiKey: "",
      modelId: existing?.modelId ?? "",
      apiProtocol: existing?.apiProtocol ?? "responses",
      contextWindowTokens: existing?.contextWindowTokens ?? 128_000,
      maxOutputTokens: existing?.maxOutputTokens ?? 8192,
      inputPricePerMillionTokens: existing?.inputPricePerMillionTokens,
      outputPricePerMillionTokens: existing?.outputPricePerMillionTokens,
    },
  });
  const mutation = useMutation({
    mutationFn: async (values: ModelFormValues) => {
      if (existing) {
        return api.models.update(existing.id, {
          ...values,
          apiKey: values.apiKey || undefined,
        });
      }
      const result = await api.models.testAndCreate(values);
      if (!result.created)
        throw new Error(result.compatibility.diagnostics.join("; "));
      return result.model;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["models"] });
      onClose();
    },
    onError: (caught) => {
      const presentation = apiErrorPresentation(caught);
      const apiField = presentation?.params?.field;
      const formField =
        typeof apiField === "string"
          ? modelFieldByApiName[apiField]
          : undefined;
      const message =
        presentation?.publicMessage ??
        t("aiSettings.model.compatibilityFailed");
      if (formField) {
        setError(formField, { message, type: "server" }, { shouldFocus: true });
        return;
      }
      setError("root", {
        message: formatApiError(
          caught,
          t("aiSettings.model.compatibilityFailed"),
          (requestId) => t("common.requestReference", { requestId }),
        ),
        type: "server",
      });
    },
  });
  return (
    <FormDrawer
      loading={mutation.isPending}
      onOpenChange={(open) => !open && onClose()}
      onSubmit={handleSubmit((values) => mutation.mutate(values))}
      open
      submitLabel={
        existing ? t("settings.common.save") : t("aiSettings.model.testCreate")
      }
      title={
        existing
          ? t("aiSettings.model.editTitle")
          : t("aiSettings.model.createTitle")
      }
    >
      <div className="argus-settings-form">
        {errors.root?.message && (
          <Alert
            description={errors.root.message}
            title={t("aiSettings.model.compatibilityFailed")}
            tone="danger"
          />
        )}
        <Field
          error={errors.name?.message}
          requirement="required"
          label={t("aiSettings.model.name")}
        >
          <Input
            {...register("name")}
            maxLength={modelConstraints.name.maxLength}
          />
        </Field>
        <Field
          error={errors.baseUrl?.message}
          requirement="required"
          label={t("aiSettings.model.baseUrl")}
        >
          <Input
            {...register("baseUrl")}
            maxLength={modelConstraints.baseUrl.maxLength}
            type="url"
          />
        </Field>
        <Field
          error={errors.apiKey?.message}
          requirement={existing ? "optional" : "required"}
          hint={t("aiSettings.model.apiKeyHint")}
          label={t("aiSettings.model.apiKey")}
        >
          <Input
            {...register("apiKey")}
            maxLength={modelConstraints.apiKey.maxLength}
            type="password"
          />
        </Field>
        <Field
          error={errors.modelId?.message}
          requirement="required"
          label={t("aiSettings.model.modelId")}
        >
          <Input
            {...register("modelId")}
            maxLength={modelConstraints.modelId.maxLength}
          />
        </Field>
        <Field
          error={errors.apiProtocol?.message}
          requirement="required"
          label={t("aiSettings.model.apiProtocol")}
        >
          <Controller
            control={control}
            name="apiProtocol"
            render={({ field }) => (
              <Select
                onValueChange={field.onChange}
                options={[
                  { value: "responses", label: "Responses" },
                  { value: "chat_completions", label: "Chat Completions" },
                ]}
                value={field.value}
              />
            )}
          />
        </Field>
        <details
          className="argus-config-advanced"
          open={Boolean(
            errors.contextWindowTokens ||
            errors.maxOutputTokens ||
            errors.inputPricePerMillionTokens ||
            errors.outputPricePerMillionTokens,
          )}
        >
          <summary>{t("aiSettings.model.advanced")}</summary>
          <div className="argus-form-row">
            <Field
              error={errors.contextWindowTokens?.message}
              requirement="required"
              label={t("aiSettings.model.contextWindowTokens")}
            >
              <Input
                {...register("contextWindowTokens", { valueAsNumber: true })}
                min={modelConstraints.contextWindowTokens.minimum}
                step="1"
                type="number"
              />
            </Field>
            <Field
              error={errors.maxOutputTokens?.message}
              requirement="required"
              label={t("aiSettings.model.maxOutputTokens")}
            >
              <Input
                {...register("maxOutputTokens", { valueAsNumber: true })}
                min={modelConstraints.maxOutputTokens.minimum}
                step="1"
                type="number"
              />
            </Field>
          </div>
          <div className="argus-form-row">
            <Field
              error={errors.inputPricePerMillionTokens?.message}
              requirement="required"
              label={t("aiSettings.model.inputPrice")}
            >
              <Input
                {...register("inputPricePerMillionTokens", {
                  valueAsNumber: true,
                })}
                min={modelConstraints.inputPrice.minimum}
                step="0.01"
                type="number"
              />
            </Field>
            <Field
              error={errors.outputPricePerMillionTokens?.message}
              requirement="required"
              label={t("aiSettings.model.outputPrice")}
            >
              <Input
                {...register("outputPricePerMillionTokens", {
                  valueAsNumber: true,
                })}
                min={modelConstraints.outputPrice.minimum}
                step="0.01"
                type="number"
              />
            </Field>
          </div>
        </details>
      </div>
    </FormDrawer>
  );
}
