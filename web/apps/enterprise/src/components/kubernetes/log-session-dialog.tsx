import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import {
  kubernetesObjectContainers,
  useApi,
  type KubernetesInventoryObject,
  type KubernetesLogSession,
} from "@argus/api-client";
import {
  Badge,
  Button,
  CodeBlock,
  Dialog,
  Field,
  Input,
  KeyValueGrid,
  Select,
  Spinner,
  Switch,
} from "@argus/ui";
import { useInventoryError } from "./inventory-error";

const schema = z.object({
  container: z.string().min(1, "kubernetesLogSession.invalid"),
  previous: z.boolean(),
  tailLines: z
    .number()
    .int()
    .min(1, "kubernetesLogSession.invalid")
    .max(1000, "kubernetesLogSession.invalid"),
  sinceSeconds: z
    .number()
    .int()
    .min(1, "kubernetesLogSession.invalid")
    .max(86400, "kubernetesLogSession.invalid"),
});
type Form = z.infer<typeof schema>;

export function KubernetesLogSessionDialog({
  clusterId,
  object,
  namespaceId,
  onClose,
}: {
  clusterId: string;
  object: KubernetesInventoryObject;
  namespaceId: string;
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
      previous: false,
      tailLines: 100,
      sinceSeconds: 300,
    },
  });
  const [session, setSession] = useState<KubernetesLogSession>();
  const [content, setContent] = useState(""),
    [error, setError] = useState(""),
    [observed, setObserved] = useState("");
  const [following, setFollowing] = useState(true),
    [reading, setReading] = useState(false),
    [complete, setComplete] = useState(false),
    [truncated, setTruncated] = useState(false),
    [reload, setReload] = useState(0);
  const cursor = useRef<string | undefined>(undefined),
    owned = useRef<string | undefined>(undefined),
    mounted = useRef(true);
  const sessionId = session?.status === "active" ? session.id : undefined;
  useEffect(() => {
    if (!sessionId || (following && !complete)) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function check() {
      try {
        await api.kubernetesWorkbench.getLogSession(
          clusterId,
          sessionId!,
          controller.signal,
        );
        if (!controller.signal.aborted)
          timer = setTimeout(() => void check(), 2000);
      } catch (cause) {
        if (!controller.signal.aborted) {
          setContent("");
          setSession(undefined);
          setError(formatError(cause));
          setFollowing(false);
        }
      }
    }
    void check();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [api, clusterId, sessionId, following, complete, formatError]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      const id = owned.current;
      owned.current = undefined;
      if (id)
        void api.kubernetesWorkbench
          .closeLogSession(clusterId, id)
          .catch(() => undefined);
    };
  }, [api, clusterId]);
  useEffect(() => {
    if (!sessionId) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    async function read() {
      setReading(true);
      try {
        const batch = await api.kubernetesWorkbench.readLogSession(
          clusterId,
          sessionId!,
          cursor.current,
          controller.signal,
        );
        if (controller.signal.aborted) return;
        cursor.current = batch.next_cursor;
        setContent((current) => {
          const next = current + batch.content;
          return next.length > 262144 ? next.slice(-262144) : next;
        });
        setObserved(batch.observed_at);
        setComplete(batch.complete);
        setTruncated(batch.truncated);
        if (following && !batch.complete)
          timer = setTimeout(() => void read(), 2000);
      } catch (cause) {
        if (!controller.signal.aborted) {
          setContent("");
          setError(formatError(cause));
          setFollowing(false);
          setSession(undefined);
          cursor.current = undefined;
        }
      } finally {
        if (!controller.signal.aborted) setReading(false);
      }
    }
    void read();
    return () => {
      controller.abort();
      if (timer) clearTimeout(timer);
    };
  }, [api, clusterId, sessionId, following, reload, formatError]);
  const submit = form.handleSubmit(async (values) => {
    setError("");
    setContent("");
    setObserved("");
    setComplete(false);
    setTruncated(false);
    cursor.current = undefined;
    let created: string | undefined;
    try {
      const ticket = await api.kubernetesWorkbench.createLogSession(clusterId, {
        namespace_id: namespaceId,
        pod: object.name,
        pod_uid: object.uid,
        container: values.container,
        previous: values.previous,
        tail_lines: values.tailLines,
        since_seconds: values.sinceSeconds,
      });
      created = ticket.session.id;
      if (!mounted.current) {
        await api.kubernetesWorkbench.closeLogSession(clusterId, created);
        return;
      }
      owned.current = created;
      let attached: KubernetesLogSession;
      try {
        attached = await api.kubernetesWorkbench.attachLogSession(
          clusterId,
          created,
          ticket.ticket,
        );
      } finally {
        ticket.ticket = "";
      }
      if (!mounted.current) {
        await api.kubernetesWorkbench.closeLogSession(clusterId, created);
        return;
      }
      setFollowing(!values.previous);
      setSession(attached);
    } catch (cause) {
      if (created)
        void api.kubernetesWorkbench
          .closeLogSession(clusterId, created)
          .catch(() => undefined);
      owned.current = undefined;
      if (mounted.current) setError(formatError(cause));
    }
  });
  const close = () => {
    const id = owned.current;
    owned.current = undefined;
    if (id)
      void api.kubernetesWorkbench
        .closeLogSession(clusterId, id)
        .catch(() => undefined);
    setContent("");
    cursor.current = undefined;
    onClose();
  };
  return (
    <Dialog
      open
      size="lg"
      title={t("kubernetesLogSession.title", { pod: object.name })}
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <p>{t("kubernetesLogSession.hint")}</p>
      {!session ? (
        <form className="argus-k8s-mutation-form" onSubmit={submit}>
          <Field
            label={t("kubernetesLogSession.container")}
            requirement="required"
            error={
              form.formState.errors.container
                ? t("kubernetesLogSession.invalid")
                : undefined
            }
          >
            <Select
              value={form.watch("container")}
              options={containers.map((c) => ({
                value: c.name,
                label: c.name,
              }))}
              onValueChange={(value) =>
                form.setValue("container", value, { shouldValidate: true })
              }
            />
          </Field>
          <Field
            label={t("kubernetesLogSession.tail")}
            requirement="required"
            error={
              form.formState.errors.tailLines
                ? t("kubernetesLogSession.invalid")
                : undefined
            }
          >
            <Input
              type="number"
              min={1}
              max={1000}
              {...form.register("tailLines", { valueAsNumber: true })}
            />
          </Field>
          <Field
            label={t("kubernetesLogSession.window")}
            requirement="required"
            error={
              form.formState.errors.sinceSeconds
                ? t("kubernetesLogSession.invalid")
                : undefined
            }
          >
            <Input
              type="number"
              min={1}
              max={86400}
              {...form.register("sinceSeconds", { valueAsNumber: true })}
            />
          </Field>
          <Field
            label={t("kubernetesLogSession.previous")}
            requirement="optional"
            controlMode="group"
          >
            <Switch
              label={t("kubernetesLogSession.previous")}
              checked={form.watch("previous")}
              onChange={(value) => form.setValue("previous", value)}
            />
          </Field>
          <div className="argus-k8s-workbench-controls">
            <Button
              type="submit"
              variant="primary"
              isDisabled={form.formState.isSubmitting}
            >
              {t("kubernetesLogSession.open")}
            </Button>
            <Button type="button" onPress={close}>
              {t("common.cancel")}
            </Button>
          </div>
        </form>
      ) : (
        <>
          <KeyValueGrid
            items={[
              {
                label: t("kubernetesWorkbench.namespace"),
                value: session.namespace,
              },
              {
                label: t("kubernetesLogSession.container"),
                value: session.container,
              },
              {
                label: t("kubernetesLogSession.podUid"),
                value: session.pod_uid,
              },
              {
                label: t("kubernetesLogSession.restarts"),
                value: String(session.restart_count),
              },
            ]}
          />
          <div className="argus-k8s-workbench-controls">
            <Field
              label={t("kubernetesLogSession.follow")}
              requirement="optional"
              controlMode="group"
            >
              <Switch
                label={t("kubernetesLogSession.follow")}
                checked={following && !complete}
                disabled={complete || session.previous}
                onChange={setFollowing}
              />
            </Field>
            <Button
              isDisabled={reading || complete}
              onPress={() => setReload((v) => v + 1)}
            >
              {t("kubernetesLogSession.refresh")}
            </Button>
            <Button onPress={close}>{t("kubernetesLogSession.close")}</Button>
          </div>
          {observed && (
            <p>
              {t("kubernetesWorkbench.observed")}{" "}
              {new Date(observed).toLocaleString(i18n.language)}
            </p>
          )}
          {complete && <Badge>{t("kubernetesLogSession.complete")}</Badge>}
          {truncated && (
            <p role="status">{t("kubernetesLogSession.truncated")}</p>
          )}
          {reading && !observed && <Spinner label={t("common.loading")} />}
          <section aria-label={t("kubernetesLogSession.output")}>
            <CodeBlock code={content || t("kubernetesLogSession.empty")} />
          </section>
          <p>{t("kubernetesLogSession.retention")}</p>
        </>
      )}
      {error && <p role="alert">{error}</p>}
    </Dialog>
  );
}
