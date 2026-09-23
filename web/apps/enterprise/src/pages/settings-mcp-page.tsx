import { Controller, useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type MCPConnection,
  type MCPConnectionWrite,
} from "@argus/api-client";
import {
  Alert,
  Button,
  DataTable,
  Field,
  FormDrawer,
  Input,
  PageShell,
  Select,
} from "@argus/ui";
import { usePermission } from "../lib/permissions";
import "../styles/planv5.css";

export function SettingsMCPPage() {
  const api = useApi();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const canManage = usePermission("mcp_connection.manage");
  const query = useQuery({
    queryKey: ["mcp-connections"],
    queryFn: () => api.mcp.list(),
    enabled: canManage,
  });
  const [editing, setEditing] = useState<MCPConnection | "new" | null>(null);
  const [failure, setFailure] = useState<string | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ["mcp-connections"] });
  const perform = async (
    value: MCPConnection,
    operation: "test" | "toggle",
  ) => {
    setBusy(value.id);
    setFailure(null);
    try {
      if (operation === "test") await api.mcp.test(value.id);
      else
        await api.mcp.setState(
          value.id,
          value.status === "enabled" ? "disabled" : "enabled",
          value.version,
        );
      await refresh();
    } catch (error) {
      setFailure(
        formatApiError(error, t("planv5.mcp.failed"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(null);
    }
  };
  return (
    <PageShell
      title={t("planv5.mcp.title")}
      description={t("planv5.mcp.description")}
    >
      {!canManage ? (
        <p role="alert">{t("planv5.mcp.forbidden")}</p>
      ) : (
        <>
          <div className="argus-planv5-toolbar">
            <Button onClick={() => setEditing("new")}>
              {t("planv5.mcp.create")}
            </Button>
          </div>
          {(failure || query.isError) && (
            <Alert
              title={t("planv5.mcp.failed")}
              description={failure ?? t("planv5.mcp.failed")}
              tone="danger"
            />
          )}
          <DataTable<MCPConnection & Record<string, unknown>>
            getRowKey={(item) => item.id}
            data={query.data ?? []}
            columns={[
              {
                key: "name",
                header: t("planv5.mcp.name"),
                render: (item) => (
                  <div>
                    <b>{item.name}</b>
                    <p>{item.endpoint}</p>
                  </div>
                ),
              },
              {
                key: "status",
                header: t("planv5.mcp.status"),
                render: (item) => (
                  <span>
                    {t(`planv5.mcp.${item.status}`)} ·{" "}
                    {t(`planv5.mcp.${item.health_status}`)}
                  </span>
                ),
              },
              {
                key: "tool_count",
                header: t("planv5.mcp.tools"),
                render: (item) => item.tool_count,
              },
              {
                key: "actions",
                header: t("planv5.mcp.actions"),
                render: (item) => (
                  <div className="argus-planv5-toolbar">
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => setEditing(item)}
                    >
                      {t("planv5.mcp.edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy === item.id}
                      onClick={() => void perform(item, "test")}
                    >
                      {t("planv5.mcp.test")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={busy === item.id}
                      onClick={() => void perform(item, "toggle")}
                    >
                      {t(
                        item.status === "enabled"
                          ? "planv5.mcp.disable"
                          : "planv5.mcp.enable",
                      )}
                    </Button>
                  </div>
                ),
              },
            ]}
          />
          {editing && (
            <ConnectionEditor
              value={editing === "new" ? undefined : editing}
              onClose={() => setEditing(null)}
              onSaved={refresh}
            />
          )}
        </>
      )}
    </PageShell>
  );
}
function ConnectionEditor({
  value,
  onClose,
  onSaved,
}: {
  value?: MCPConnection;
  onClose(): void;
  onSaved(): Promise<void>;
}) {
  const api = useApi();
  const { t } = useTranslation();
  const schema = z
    .object({
      name: z.string().trim().min(1, t("planv5.mcp.required")),
      endpoint: z
        .string()
        .url(t("planv5.mcp.invalidURL"))
        .refine((value) => /^https:/.test(value), t("planv5.mcp.invalidURL")),
      auth_type: z.enum(["none", "bearer", "basic"]),
      credential: z.string(),
      username: z.string(),
      password: z.string(),
      member_ids: z.array(z.string()),
    })
    .superRefine((input, context) => {
      if (
        input.auth_type === "bearer" &&
        !input.credential &&
        value?.auth_type !== "bearer"
      )
        context.addIssue({
          code: "custom",
          path: ["credential"],
          message: t("planv5.mcp.required"),
        });
      if (
        input.auth_type === "basic" &&
        (input.username || input.password || value?.auth_type !== "basic") &&
        (!input.username || !input.password)
      )
        context.addIssue({
          code: "custom",
          path: ["username"],
          message: t("planv5.mcp.basicRequired"),
        });
    });
  type Fields = z.infer<typeof schema>;
  const {
    register,
    control,
    watch,
    setValue,
    handleSubmit,
    formState: { errors },
  } = useForm<Fields>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: value?.name ?? "",
      endpoint: value?.endpoint ?? "",
      auth_type: value?.auth_type ?? "none",
      credential: "",
      username: "",
      password: "",
      member_ids: value?.member_ids ?? [],
    },
  });
  const members = useQuery({
    queryKey: ["org", "users"],
    queryFn: () => api.org.listUsers(),
  });
  const auth = watch("auth_type");
  const selected = watch("member_ids");
  const mutation = useMutation({
    mutationFn: (input: Fields) => {
      const body: MCPConnectionWrite = {
        name: input.name,
        endpoint: input.endpoint,
        auth_type: input.auth_type,
        member_ids: input.member_ids,
        expected_version: value?.version,
      };
      if (input.auth_type === "bearer" && input.credential)
        body.credential_value = input.credential;
      if (input.auth_type === "basic" && input.username && input.password)
        body.credential_value = JSON.stringify({
          username: input.username,
          password: input.password,
        });
      return value ? api.mcp.update(value.id, body) : api.mcp.create(body);
    },
    onSuccess: async () => {
      await onSaved();
      onClose();
    },
  });
  return (
    <FormDrawer
      open
      title={t(value ? "planv5.mcp.edit" : "planv5.mcp.create")}
      onOpenChange={(open) => !open && onClose()}
      onSubmit={handleSubmit((input) => mutation.mutate(input))}
      loading={mutation.isPending}
      submitLabel={t("planv5.mcp.save")}
    >
      <div className="argus-planv5-form">
        {mutation.isError && (
          <Alert
            title={t("planv5.mcp.failed")}
            description={formatApiError(
              mutation.error,
              t("planv5.mcp.failed"),
              (requestId) => t("common.requestReference", { requestId }),
            )}
            tone="danger"
          />
        )}
        <Field
          requirement="required"
          label={t("planv5.mcp.name")}
          error={errors.name?.message}
        >
          <Input {...register("name")} />
        </Field>
        <Field
          requirement="required"
          label={t("planv5.mcp.endpoint")}
          error={errors.endpoint?.message}
        >
          <Input type="url" {...register("endpoint")} />
        </Field>
        <Field requirement="required" label={t("planv5.mcp.auth")}>
          <Controller
            control={control}
            name="auth_type"
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={(value) => {
                  field.onChange(value);
                  setValue("credential", "");
                  setValue("username", "");
                  setValue("password", "");
                }}
                options={["none", "bearer", "basic"].map((value) => ({
                  value,
                  label: t(`planv5.mcp.${value}`),
                }))}
              />
            )}
          />
        </Field>
        {auth === "bearer" && (
          <Field
            requirement={
              value?.auth_type === "bearer" ? "optional" : "required"
            }
            label={t("planv5.mcp.credential")}
            error={errors.credential?.message}
            hint={t("planv5.mcp.credentialHint")}
          >
            <Input
              type="password"
              autoComplete="new-password"
              {...register("credential")}
            />
          </Field>
        )}
        {auth === "basic" && (
          <>
            <Field
              requirement={
                value?.auth_type === "basic" ? "optional" : "required"
              }
              label={t("planv5.mcp.username")}
              error={errors.username?.message}
              hint={t("planv5.mcp.credentialHint")}
            >
              <Input autoComplete="off" {...register("username")} />
            </Field>
            <Field
              requirement={
                value?.auth_type === "basic" ? "optional" : "required"
              }
              label={t("planv5.mcp.password")}
            >
              <Input
                type="password"
                autoComplete="new-password"
                {...register("password")}
              />
            </Field>
          </>
        )}
        <fieldset className="argus-planv5-members">
          <legend>{t("planv5.mcp.members")}</legend>
          {members.data
            ?.filter((member) => member.status !== "disabled")
            .map((member) => (
              <label key={member.id}>
                <input
                  type="checkbox"
                  checked={selected.includes(member.id)}
                  onChange={(event) =>
                    setValue(
                      "member_ids",
                      event.target.checked
                        ? [...selected, member.id]
                        : selected.filter((id) => id !== member.id),
                    )
                  }
                />
                {member.displayName}
              </label>
            ))}
        </fieldset>
      </div>
    </FormDrawer>
  );
}
