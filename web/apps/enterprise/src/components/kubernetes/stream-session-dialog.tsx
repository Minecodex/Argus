import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import {
  KubernetesTerminalConnection,
  kubernetesObjectContainers,
  useApi,
  type KubernetesInventoryObject,
  type KubernetesStreamSession,
  type KubernetesStreamTicket,
} from "@argus/api-client";
import {
  Badge,
  Button,
  CodeBlock,
  Dialog,
  Field,
  Input,
  KeyValueGrid,
  ProtectedTextarea,
  Select,
  TerminalEmulator,
  type TerminalLine,
} from "@argus/ui";
import { useInventoryError } from "./inventory-error";

const schema = z.object({
  container: z.string(),
  shell: z.enum(["sh", "bash", "powershell", "cmd"]),
  port: z.number().int().min(1).max(65535),
  reason: z.string().trim().min(1).max(500),
});
type Form = z.infer<typeof schema>;
export function KubernetesStreamSessionDialog({
  clusterId,
  object,
  namespaceId,
  mode,
  onClose,
}: {
  clusterId: string;
  object: KubernetesInventoryObject;
  namespaceId: string;
  mode: "exec" | "port_forward";
  onClose: () => void;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    formatError = useInventoryError();
  const containers = kubernetesObjectContainers(object);
  const form = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: {
      container: containers[0]?.name ?? "",
      shell: "sh",
      port: 8080,
      reason: "",
    },
  });
  const [session, setSession] = useState<KubernetesStreamSession>();
  const [ticket, setTicket] = useState<KubernetesStreamTicket>();
  const [state, setState] = useState<
    "connecting" | "connected" | "disconnected"
  >("disconnected");
  const [output, setOutput] = useState<{
    lines: TerminalLine[];
    offset: number;
    size: number;
  }>({ lines: [], offset: 0, size: 0 });
  const [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [clock, setClock] = useState(Date.now());
  const owned = useRef<string | undefined>(undefined),
    mounted = useRef(true),
    connection = useRef<KubernetesTerminalConnection | undefined>(undefined);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      connection.current?.close();
      const id = owned.current;
      owned.current = undefined;
      if (id)
        void api.kubernetesWorkbench
          .closeStreamSession(clusterId, id)
          .catch(() => undefined);
    };
  }, [api, clusterId]);
  useEffect(() => {
    if (!session) return;
    const timer = setInterval(() => setClock(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [session]);
  const sessionId = session?.id;
  useEffect(() => {
    if (!sessionId) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function check() {
      try {
        const current = await api.kubernetesWorkbench.getStreamSession(
          clusterId,
          sessionId!,
          controller.signal,
        );
        if (controller.signal.aborted) return;
        setSession(current);
        if (
          ["closed", "invalidated", "connection_lost"].includes(current.status)
        ) {
          connection.current?.close();
          setTicket(undefined);
          return;
        }
      } catch (cause) {
        if (controller.signal.aborted) return;
        connection.current?.close();
        setTicket(undefined);
        setError(formatError(cause));
        return;
      }
      timer = setTimeout(() => void check(), 2000);
    }
    timer = setTimeout(() => void check(), 2000);
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [api, clusterId, sessionId, formatError]);
  const submit = form.handleSubmit(async (values) => {
    setError("");
    try {
      const result = await api.kubernetesWorkbench.createStreamSession(
        clusterId,
        {
          namespace_id: namespaceId,
          type: object.definition.key as "pod" | "service",
          name: object.name,
          uid: object.uid,
          mode,
          reason: values.reason,
          ...(mode === "exec"
            ? { container: values.container, shell: values.shell }
            : { port: values.port }),
        },
      );
      if (!mounted.current) {
        void api.kubernetesWorkbench
          .closeStreamSession(clusterId, result.session.id)
          .catch(() => undefined);
        return;
      }
      owned.current = result.session.id;
      setSession(result.session);
      if (mode === "port_forward") {
        setTicket(result);
        return;
      }
      const terminal = new KubernetesTerminalConnection({
        output: (data) => {
          if (mounted.current)
            setOutput((previous) => {
              const lines = [
                ...previous.lines,
                { kind: "stdout" as const, content: data },
              ];
              let size = previous.size + data.length,
                offset = previous.offset;
              while (size > 256 * 1024 && lines.length > 1) {
                size -= lines.shift()!.content.length;
                offset++;
              }
              return { lines, size, offset };
            });
        },
        state: (next, reason) => {
          if (mounted.current) {
            setState(next);
            if (reason && !["client_close", "remote_closed"].includes(reason))
              setError(
                t(
                  reason === "KUBERNETES_API_FORBIDDEN"
                    ? "kubernetesWorkbench.errors.forbidden"
                    : "kubernetesStream.stopped",
                ),
              );
          }
        },
      });
      connection.current = terminal;
      terminal.connect(result);
    } catch (cause) {
      if (mounted.current) setError(formatError(cause));
    }
  });
  const close = async () => {
    setBusy(true);
    connection.current?.close();
    try {
      if (owned.current)
        await api.kubernetesWorkbench.closeStreamSession(
          clusterId,
          owned.current,
        );
      owned.current = undefined;
      setTicket(undefined);
      setOutput({ lines: [], offset: 0, size: 0 });
      onClose();
    } catch (cause) {
      setError(formatError(cause));
      setBusy(false);
    }
  };
  const renew = async () => {
    if (!session) return;
    setBusy(true);
    try {
      setTicket(
        await api.kubernetesWorkbench.issueStreamTicket(clusterId, session.id),
      );
      setError("");
    } catch (cause) {
      setError(formatError(cause));
    } finally {
      setBusy(false);
    }
  };
  const remaining = ticket
    ? Math.max(
        0,
        Math.ceil((Date.parse(ticket.ticket_expires_at) - clock) / 1000),
      )
    : 0;
  const active =
    !!session &&
    ["pending", "connecting", "active", "closing"].includes(session.status);
  return (
    <Dialog
      open
      size="lg"
      title={t(
        mode === "exec"
          ? "kubernetesStream.execTitle"
          : "kubernetesStream.forwardTitle",
        { name: object.name },
      )}
      onOpenChange={(open) => {
        if (!open) void close();
      }}
    >
      <p>
        {t(
          mode === "exec"
            ? "kubernetesStream.execHint"
            : "kubernetesStream.forwardHint",
        )}
      </p>
      {error && <p role="alert">{error}</p>}
      {!session ? (
        <form className="argus-k8s-mutation-form" onSubmit={submit}>
          {mode === "exec" ? (
            <>
              <Field
                label={t("kubernetesStream.container")}
                requirement="required"
              >
                <Select
                  value={form.watch("container")}
                  options={containers.map((c) => ({
                    value: c.name,
                    label: c.name,
                  }))}
                  onValueChange={(v) => form.setValue("container", v)}
                />
              </Field>
              <Field label={t("kubernetesStream.shell")} requirement="required">
                <Select
                  value={form.watch("shell")}
                  options={["sh", "bash", "powershell", "cmd"].map((v) => ({
                    value: v,
                    label: v,
                  }))}
                  onValueChange={(v) =>
                    form.setValue("shell", v as Form["shell"])
                  }
                />
              </Field>
            </>
          ) : (
            <Field
              label={t("kubernetesStream.port")}
              requirement="required"
              error={
                form.formState.errors.port
                  ? t("kubernetesStream.invalid")
                  : undefined
              }
            >
              <Input
                type="number"
                min={1}
                max={65535}
                {...form.register("port", { valueAsNumber: true })}
              />
            </Field>
          )}
          <Field
            label={t("kubernetesStream.reason")}
            requirement="required"
            error={
              form.formState.errors.reason
                ? t("kubernetesStream.invalid")
                : undefined
            }
          >
            <Input maxLength={500} {...form.register("reason")} />
          </Field>
          <Button
            type="submit"
            variant="primary"
            isDisabled={form.formState.isSubmitting}
          >
            {t("kubernetesStream.open")}
          </Button>
        </form>
      ) : (
        <>
          <Badge tone={active ? "success" : "neutral"}>
            {t(
              `kubernetesStream.status.${mode === "exec" && state === "connected" && session.status === "pending" ? "active" : session.status}`,
            )}
          </Badge>
          <KeyValueGrid
            items={[
              { label: "Pod", value: session.pod },
              { label: "Pod UID", value: session.pod_uid },
              ...(mode === "port_forward"
                ? [
                    {
                      label: t("kubernetesStream.port"),
                      value: String(session.port),
                    },
                    ...(session.service
                      ? [{ label: "Service", value: session.service }]
                      : []),
                  ]
                : []),
              {
                label: t("kubernetesStream.container"),
                value: session.container,
              },
              {
                label: t("kubernetesStream.expires"),
                value: new Date(session.expires_at).toLocaleString(
                  i18n.language,
                ),
              },
              {
                label: t("kubernetesStream.bytes"),
                value: `${session.bytes_in} / ${session.bytes_out}`,
              },
            ]}
          />
          {mode === "exec" ? (
            <TerminalEmulator
              host={session.pod}
              account={session.container}
              protocol="Kubernetes"
              mode="pty"
              sessionId={session.id}
              lines={output.lines}
              chunkOffset={output.offset}
              state={state}
              readOnly={state !== "connected"}
              onData={(data) => connection.current?.input(data)}
              onResize={(cols, rows) => connection.current?.resize(cols, rows)}
            />
          ) : (
            <>
              {ticket && active && (
                <>
                  <p>{t("kubernetesStream.cliHint")}</p>
                  <CodeBlock
                    code={`argusctl kubernetes port-forward --url ${ticket.websocket_url} --ticket-stdin --listen 127.0.0.1:18080`}
                    language="bash"
                  />
                  {remaining > 0 ? (
                    <Field
                      label={t("kubernetesStream.ticket", {
                        seconds: remaining,
                      })}
                      controlMode="group"
                      requirement="optional"
                    >
                      <ProtectedTextarea
                        readOnly
                        value={ticket.ticket}
                        aria-label={t("kubernetesStream.ticketValue")}
                      />
                    </Field>
                  ) : (
                    <p role="status">{t("kubernetesStream.ticketExpired")}</p>
                  )}
                  {session.status === "pending" && (
                    <Button onPress={() => void renew()} isDisabled={busy}>
                      {t("kubernetesStream.renew")}
                    </Button>
                  )}
                </>
              )}
            </>
          )}
        </>
      )}
      <div className="argus-k8s-workbench-controls">
        <Button onPress={() => void close()} isDisabled={busy}>
          {t("kubernetesStream.close")}
        </Button>
      </div>
    </Dialog>
  );
}
