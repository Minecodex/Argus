import { QueryBoundary } from "@argus/ui";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { z } from "zod";
import {
  formConstraint,
  presentApiFormError,
  useApi,
  type CreateSandboxProfileInput,
  type SandboxImage,
  type SandboxProfile,
} from "@argus/api-client";
import {
  Alert,
  Button,
  DataTable,
  Field,
  FormDrawer,
  Input,
  RowAction,
  Select,
  Spinner,
  Switch,
} from "@argus/ui";

const constraints = {
  name: formConstraint("SandboxProfileWrite", "name"),
  cpu: formConstraint("SandboxProfileWrite", "cpu_millis"),
  memory: formConstraint("SandboxProfileWrite", "memory_mib"),
  timeout: formConstraint("SandboxProfileWrite", "timeout_seconds"),
};
type FormState = {
  id: string | null;
  name: string;
  imageId: string;
  cpu: number;
  memoryMb: number;
  timeoutSeconds: number;
};
type ProfileRow = { [Key in keyof SandboxProfile]: SandboxProfile[Key] };
function toInput(form: FormState): CreateSandboxProfileInput {
  return {
    name: form.name.trim(),
    imageId: form.imageId,
    resources: { cpu: form.cpu, memoryMb: form.memoryMb },
    timeoutSeconds: form.timeoutSeconds,
  };
}
function fromProfile(profile: SandboxProfile): FormState {
  return {
    id: profile.id,
    name: profile.name,
    imageId: profile.imageId,
    ...profile.resources,
    timeoutSeconds: profile.timeoutSeconds,
  };
}

export function ProfilesTab() {
  const { t } = useTranslation();
  const api = useApi();
  const queryClient = useQueryClient();
  const [form, setForm] = useState<FormState | null>(null);
  const profiles = useQuery({
    queryKey: ["platform", "profiles"],
    queryFn: () => api.platform.profiles.list(),
  });
  const images = useQuery({
    queryKey: ["platform", "images"],
    queryFn: () => api.platform.images.list(),
  });
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["platform", "profiles"] });
  const save = useMutation({
    mutationFn: (input: FormState) =>
      input.id
        ? api.platform.profiles.update(input.id, toInput(input))
        : api.platform.profiles.create(toInput(input)),
    onSuccess: () => {
      setForm(null);
      void invalidate();
    },
  });
  const toggle = useMutation({
    mutationFn: (input: { id: string; enabled: boolean }) =>
      api.platform.profiles.update(input.id, { enabled: input.enabled }),
    onSuccess: () => void invalidate(),
  });
  return (
    <div className="argus-platform-stack">
      <Alert
        title={t("sandbox.profiles.offlineTitle")}
        description={t("sandbox.profiles.offlineDescription")}
      />
      <div className="argus-tab-toolbar">
        <Button
          variant="primary"
          onPress={() =>
            setForm({
              id: null,
              name: "",
              imageId: images.data?.find((image) => image.enabled)?.id ?? "",
              cpu: 1,
              memoryMb: 1024,
              timeoutSeconds: 900,
            })
          }
        >
          {t("sandbox.profiles.add")}
        </Button>
      </div>
      {toggle.isError && (
        <Alert
          tone="danger"
          title={t("sandbox.form.saveFailed")}
          description={String(toggle.error)}
        />
      )}
      <QueryBoundary query={profiles} dependencies={[images]}>
        {profiles.isPending ? (
          <Spinner />
        ) : (
          <DataTable<ProfileRow>
            columns={[
              { key: "name", header: t("sandbox.profiles.table.name") },
              {
                key: "imageId",
                header: t("sandbox.profiles.table.image"),
                render: (row) =>
                  images.data?.find((image) => image.id === row.imageId)
                    ?.name ?? row.imageId,
              },
              {
                key: "resources",
                header: t("sandbox.profiles.table.resources"),
                render: (row) =>
                  `${row.resources.cpu} CPU / ${row.resources.memoryMb} MiB`,
              },
              {
                key: "timeoutSeconds",
                header: t("sandbox.profiles.form.timeoutSeconds"),
              },
              {
                key: "taskKinds",
                header: t("sandbox.profiles.table.purpose"),
                render: (row) =>
                  row.taskKinds
                    .map((kind) => t(`sandbox.profiles.purpose.${kind}`))
                    .join(", "),
              },
              {
                key: "networkMode",
                header: t("sandbox.profiles.table.network"),
                render: (row) =>
                  t(
                    row.networkMode === "none"
                      ? "sandbox.profiles.network.deny_all"
                      : "sandbox.profiles.network.allow_list",
                  ),
              },
              {
                key: "enabled",
                header: t("sandbox.profiles.table.enabled"),
                render: (row) => (
                  <Switch
                    checked={row.enabled}
                    disabled={toggle.isPending}
                    label={t("sandbox.profiles.table.enabled")}
                    onChange={(enabled) =>
                      toggle.mutate({ id: row.id, enabled })
                    }
                  />
                ),
              },
              {
                key: "id",
                header: t("common.actions"),
                render: (row) => (
                  <RowAction onPress={() => setForm(fromProfile(row))}>
                    {t("common.edit")}
                  </RowAction>
                ),
              },
            ]}
            data={profiles.data ?? []}
            getRowKey={(row) => row.id}
          />
        )}
      </QueryBoundary>
      {form && (
        <ProfileForm
          initial={form}
          images={images.data ?? []}
          loading={save.isPending}
          onClose={() => setForm(null)}
          onSubmit={(input) => save.mutateAsync(input)}
        />
      )}
    </div>
  );
}

function ProfileForm({
  initial,
  images,
  loading,
  onClose,
  onSubmit,
}: {
  initial: FormState;
  images: SandboxImage[];
  loading: boolean;
  onClose: () => void;
  onSubmit: (input: FormState) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const required = t("sandbox.form.required");
  const bounds = {
    cpu: {
      minimum: (constraints.cpu.minimum ?? 100) / 1000,
      maximum: (constraints.cpu.maximum ?? 16000) / 1000,
      step: 0.1,
    },
    memoryMb: {
      minimum: constraints.memory.minimum ?? 128,
      maximum: constraints.memory.maximum ?? 65536,
      step: 1,
    },
    timeoutSeconds: {
      minimum: constraints.timeout.minimum ?? 10,
      maximum: constraints.timeout.maximum ?? 3600,
      step: 1,
    },
  };
  const schema = z.object({
    name: z
      .string()
      .trim()
      .min(constraints.name.minLength ?? 1, required)
      .max(constraints.name.maxLength ?? 128),
    imageId: z.string().min(1, required),
    cpu: z
      .number({ error: required })
      .min(bounds.cpu.minimum)
      .max(bounds.cpu.maximum),
    memoryMb: z
      .number({ error: required })
      .int()
      .min(bounds.memoryMb.minimum)
      .max(bounds.memoryMb.maximum),
    timeoutSeconds: z
      .number({ error: required })
      .int()
      .min(bounds.timeoutSeconds.minimum)
      .max(bounds.timeoutSeconds.maximum),
  });
  const {
    control,
    register,
    clearErrors,
    handleSubmit,
    setError,
    formState: { errors },
  } = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: initial,
  });
  const submit = handleSubmit(async (values) => {
    clearErrors();
    try {
      await onSubmit({ ...values, id: initial.id });
    } catch (error) {
      presentApiFormError(error, {
        fallback: t("sandbox.form.saveFailed"),
        fieldMap: {
          name: "name",
          backend_id: "imageId",
          image_id: "imageId",
          cpu_millis: "cpu",
          memory_mib: "memoryMb",
          timeout_seconds: "timeoutSeconds",
        },
        requestReference: (requestId) =>
          t("common.requestReference", { requestId }),
        setFieldError: (field, message) =>
          setError(field, { message, type: "server" }, { shouldFocus: true }),
        setFormError: (message) =>
          setError("root", { message, type: "server" }),
      });
    }
  });
  return (
    <FormDrawer
      loading={loading}
      onOpenChange={(open) => !open && onClose()}
      onSubmit={submit}
      open
      submitLabel={t("common.save")}
      title={t(initial.id ? "sandbox.profiles.edit" : "sandbox.profiles.add")}
      width={560}
    >
      <div className="argus-drawer-stack">
        {errors.root?.message && (
          <Alert
            tone="danger"
            title={t("sandbox.form.saveFailed")}
            description={errors.root.message}
          />
        )}
        <Field
          error={errors.name?.message}
          requirement="required"
          label={t("sandbox.profiles.form.name")}
        >
          <Input {...register("name")} maxLength={constraints.name.maxLength} />
        </Field>
        <Field
          error={errors.imageId?.message}
          requirement="required"
          label={t("sandbox.profiles.form.image")}
        >
          <Controller
            control={control}
            name="imageId"
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={field.onChange}
                options={images
                  .filter(
                    (image) => image.enabled || image.id === initial.imageId,
                  )
                  .map((image) => ({ value: image.id, label: image.name }))}
              />
            )}
          />
        </Field>
        <div className="argus-form-grid">
          {(["cpu", "memoryMb", "timeoutSeconds"] as const).map((key) => (
            <Field
              key={key}
              error={errors[key]?.message}
              requirement="required"
              label={t(`sandbox.profiles.form.${key}`)}
            >
              <Input
                {...register(key, { valueAsNumber: true })}
                type="number"
                min={bounds[key].minimum}
                max={bounds[key].maximum}
                step={bounds[key].step}
              />
            </Field>
          ))}
        </div>
        <Alert
          title={t("sandbox.profiles.offlineTitle")}
          description={t("sandbox.profiles.offlineDescription")}
        />
      </div>
    </FormDrawer>
  );
}
