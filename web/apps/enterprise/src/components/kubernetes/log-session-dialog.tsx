import { useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useTranslation } from "react-i18next";
import {
  kubernetesObjectContainers,
  type KubernetesInventoryObject,
} from "@argus/api-client";
import {
  Badge,
  Button,
  TroubleshootingWorkspace,
  Field,
  Input,
  LogViewer,
  Select,
  Spinner,
  Switch,
} from "@argus/ui";
import { useInventoryError } from "./inventory-error";
import { usePodLogSession } from "./use-pod-log-session";

const schema = z.object({
  container: z.string().min(1),
  previous: z.boolean(),
  tailLines: z.number().int().min(1).max(1000),
  sinceSeconds: z.number().int().min(0).max(86400),
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
  const { t, i18n } = useTranslation();
  const formatError = useInventoryError();
  const containers = kubernetesObjectContainers(object);
  const [config, setConfig] = useState<Form>({
    container: containers[0]?.name ?? "",
    previous: false,
    tailLines: 100,
    sinceSeconds: 0,
  });
  const [options, setOptions] = useState(false);
  const form = useForm<Form>({
    resolver: zodResolver(schema),
    defaultValues: config,
  });
  const input = useMemo(
    () => ({
      namespace_id: namespaceId,
      pod: object.name,
      pod_uid: object.uid,
      container: config.container,
      previous: config.previous,
      tail_lines: config.tailLines,
      since_seconds: config.sinceSeconds,
    }),
    [namespaceId, object.name, object.uid, config],
  );
  const log = usePodLogSession(clusterId, input, formatError);
  const lines = useMemo(
    () =>
      log.content
        ? log.content
            .replace(/\r\n/g, "\n")
            .split("\n")
            .filter((line, index, all) => line || index < all.length - 1)
            .map((content) => ({ content }))
        : [],
    [log.content],
  );
  const apply = form.handleSubmit((values) => {
    setConfig((current) =>
      JSON.stringify(current) === JSON.stringify(values) ? current : values,
    );
    setOptions(false);
  });
  return (
    <TroubleshootingWorkspace
      open
      size="lg"
      className="argus-k8s-session-dialog"
      footer={
        <Button onPress={onClose}>{t("kubernetesLogSession.close")}</Button>
      }
      title={t("kubernetesLogSession.title", { pod: object.name })}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <div className="argus-k8s-session-content">
        <div className="argus-k8s-log-controls">
          <Field
            label={t("kubernetesLogSession.container")}
            requirement="required"
          >
            <Select
              value={config.container}
              options={containers.map((c) => ({
                value: c.name,
                label: c.name,
              }))}
              onValueChange={(container) => {
                form.setValue("container", container);
                setConfig((current) => ({ ...current, container }));
              }}
            />
          </Field>
          <div className="argus-k8s-workbench-controls argus-k8s-log-previous">
            <Switch
              label={t("kubernetesLogSession.previous")}
              checked={config.previous}
              onChange={(previous) => {
                form.setValue("previous", previous);
                setConfig((current) => ({ ...current, previous }));
              }}
            />
            <span>{t("kubernetesLogSession.previous")}</span>
          </div>
          <Button
            onPress={() => setOptions((value) => !value)}
            aria-expanded={options}
          >
            {t("kubernetesLogSession.options")}
          </Button>
        </div>
        {options && (
          <form className="argus-k8s-log-options" onSubmit={apply}>
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
              hint={t("kubernetesLogSession.windowHint")}
              requirement="required"
              error={
                form.formState.errors.sinceSeconds
                  ? t("kubernetesLogSession.invalid")
                  : undefined
              }
            >
              <Input
                type="number"
                min={0}
                max={86400}
                {...form.register("sinceSeconds", { valueAsNumber: true })}
              />
            </Field>
            <Button type="submit" variant="primary">
              {t("kubernetesLogSession.apply")}
            </Button>
          </form>
        )}
        <p>{t("kubernetesLogSession.scrollHint")}</p>
        {log.error && (
          <div role="alert">
            <p>{log.error}</p>
            <Button onPress={log.retry}>
              {t("kubernetesLogSession.retry")}
            </Button>
          </div>
        )}
        {!config.container && (
          <p role="status">{t("kubernetesLogSession.noContainers")}</p>
        )}
        {log.reading && !log.observed && (
          <Spinner label={t("kubernetesLogSession.loading")} />
        )}
        <LogViewer
          key={log.session?.id ?? JSON.stringify(input)}
          lines={lines}
          emptyMessage={
            config.sinceSeconds
              ? t("kubernetesLogSession.windowEmpty", {
                  seconds: config.sinceSeconds,
                })
              : t("kubernetesLogSession.empty")
          }
          autoScroll={!config.previous}
          fileName={object.name + "-" + config.container + ".log"}
        />
        <div className="argus-k8s-workbench-footer">
          <span>
            {object.namespace} · {config.container}
            {log.observed &&
              " · " + new Date(log.observed).toLocaleString(i18n.language)}
          </span>
          {log.complete && <Badge>{t("kubernetesLogSession.complete")}</Badge>}
        </div>
        {log.truncated && (
          <p role="status">{t("kubernetesLogSession.truncated")}</p>
        )}
        <p>{t("kubernetesLogSession.retention")}</p>
      </div>
    </TroubleshootingWorkspace>
  );
}
