import { useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  MonitoringIntegration,
  MonitoringMutation,
  MonitoringService,
  MonitoringSchemas,
} from "@argus/api-client";
import {
  Badge,
  Button,
  Field,
  FormDialog,
  Input,
  Select,
  Switch,
  Textarea,
} from "@argus/ui";
import {
  ApplicationPicker,
  CollectorPicker,
  ServicePicker,
  type ApplicationAssociation,
  type CollectorChoice,
} from "./pickers";
import { KubernetesLogFields } from "./kubernetes-log-fields";

export function LogCollectionForm({
  integration,
  busy,
  error,
  onSubmit,
  onClose,
}: {
  integration: MonitoringIntegration;
  busy: boolean;
  error: string;
  onSubmit: (input: MonitoringMutation) => Promise<boolean>;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(""),
    [scope, setScope] = useState("business"),
    [enabled, setEnabled] = useState(true);
  const [include, setInclude] = useState(""),
    [exclude, setExclude] = useState(""),
    [units, setUnits] = useState(""),
    [channel, setChannel] = useState("Application");
  const [encoding, setEncoding] = useState("utf-8"),
    [format, setFormat] = useState("text"),
    [startAt, setStartAt] = useState("end"),
    [multiline, setMultiline] = useState(""),
    [maxBytes, setMaxBytes] = useState(262144);
  const [application, setApplication] = useState<ApplicationAssociation>(),
    [service, setService] = useState<MonitoringService>(),
    [collector, setCollector] = useState<CollectorChoice>();
  const [picker, setPicker] = useState<
      "application" | "service" | "collector"
    >(),
    [invalid, setInvalid] = useState("");
  const lines = (text: string) =>
    text
      .split(/\r?\n/)
      .map((v) => v.trim())
      .filter(Boolean);
  const isFile = integration.key === "filelog";
  const isKubernetes = integration.key === "k8slog";
  const [workload, setWorkload] =
    useState<MonitoringSchemas["MonitoringLogWorkload"]>();
  const submit = async () => {
    if (
      !name.trim() ||
      !collector ||
      (scope === "business" && (!application || !service)) ||
      (isFile && !lines(include).length) ||
      (isKubernetes && !workload) ||
      maxBytes < 1024 ||
      maxBytes > 1048576
    ) {
      setInvalid(t("monitoring.completeRequired"));
      return;
    }
    setInvalid("");
    const mutation: MonitoringMutation = {
      kind: "collection",
      operation: "create",
      collection: {
        name: name.trim(),
        integration: integration.key,
        collector_id: collector.id,
        enabled,
        ...(scope === "business"
          ? {
              application_id: application!.application.id,
              service_id: service!.id,
            }
          : {}),
        config: {
          scope: scope as "business" | "system" | "component",
          start_at: startAt as "end" | "beginning",
          encoding: encoding as "utf-8" | "utf-16le" | "utf-16be" | "ascii",
          format: format as "text" | "json",
          max_log_bytes: maxBytes,
          ...(isKubernetes ? { workload } : {}),
          ...(isFile
            ? {
                include: lines(include),
                exclude: lines(exclude),
                multiline_start: multiline,
              }
            : {}),
          ...(integration.key === "journald" ? { units: lines(units) } : {}),
          ...(integration.key === "eventlog" ? { channel } : {}),
        },
      },
    };
    if (await onSubmit(mutation)) onClose();
  };
  return (
    <>
      <FormDialog
        open
        title={t("monitoring.createIntegration", { name: integration.name })}
        description={t("logCollection.description")}
        loading={busy}
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
        submitLabel={t("monitoring.previewCollection")}
        onSubmit={() => void submit()}
      >
        <Field label={t("monitoring.collectionName")} requirement="required">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={200}
            required
          />
        </Field>
        <Field label={t("logCollection.scope")} requirement="required">
          <Select
            value={scope}
            onValueChange={(value) => {
              setScope(value);
              setApplication(undefined);
              setService(undefined);
            }}
            options={["business", "system", "component"].map((value) => ({
              value,
              label: t(`logCollection.scopes.${value}`),
            }))}
          />
        </Field>
        {scope === "business" ? (
          <>
            <Field
              label={t("logCollection.application")}
              requirement="required"
              hint={t("monitoring.applicationContextHint")}
            >
              <Button type="button" onPress={() => setPicker("application")}>
                {application?.application.name ??
                  t("monitoring.chooseApplications")}
              </Button>
              {application && (
                <Badge>
                  {t(`monitoring.env.${application.application.environment}`, {
                    defaultValue: application.application.environment,
                  })}
                </Badge>
              )}
            </Field>
            <Field
              label={t("monitoring.serviceName")}
              requirement="required"
              hint={t("logCollection.serviceHint")}
            >
              <Button
                type="button"
                isDisabled={!application}
                onPress={() => setPicker("service")}
              >
                {service?.service_name ?? t("monitoring.chooseService")}
              </Button>
            </Field>
          </>
        ) : (
          <p>
            {t(
              isKubernetes
                ? "logCollection.clusterInfrastructureHint"
                : "logCollection.infrastructureHint",
            )}
          </p>
        )}
        <Field
          label={t("monitoring.executingCollector")}
          requirement="required"
          hint={t(
            isKubernetes
              ? "logCollection.clusterCollectorHint"
              : "logCollection.collectorHint",
          )}
        >
          <Button type="button" onPress={() => setPicker("collector")}>
            {collector?.name ?? t("monitoring.chooseCollector")}
          </Button>
        </Field>
        {isKubernetes && collector && (
          <KubernetesLogFields
            key={collector.id}
            clusterId={collector.resource.id}
            onChange={setWorkload}
          />
        )}
        {isFile && (
          <>
            <Field
              label={t("logCollection.include")}
              requirement="required"
              hint={t("logCollection.patternHint")}
            >
              <Textarea
                value={include}
                onChange={(e) => setInclude(e.target.value)}
                rows={3}
                required
                placeholder={
                  collector?.platform === "windows_amd64"
                    ? "C:/logs/orders/*.log"
                    : "/var/log/orders/*.log"
                }
              />
            </Field>
            <Field label={t("logCollection.exclude")} requirement="optional">
              <Textarea
                value={exclude}
                onChange={(e) => setExclude(e.target.value)}
                rows={2}
              />
            </Field>
            <Field label={t("logCollection.encoding")} requirement="required">
              <Select
                value={encoding}
                onValueChange={setEncoding}
                options={["utf-8", "utf-16le", "utf-16be", "ascii"].map(
                  (value) => ({ value, label: value }),
                )}
              />
            </Field>
            <Field label={t("logCollection.format")} requirement="required">
              <Select
                value={format}
                onValueChange={setFormat}
                options={[
                  { value: "text", label: t("logCollection.text") },
                  { value: "json", label: "JSON" },
                ]}
              />
            </Field>
            <Field
              label={t("logCollection.multiline")}
              requirement="optional"
              hint={t("logCollection.multilineHint")}
            >
              <Input
                value={multiline}
                onChange={(e) => setMultiline(e.target.value)}
                maxLength={1024}
              />
            </Field>
            <Field label={t("logCollection.maxBytes")} requirement="required">
              <Input
                type="number"
                min={1024}
                max={1048576}
                value={maxBytes}
                onChange={(e) => setMaxBytes(Number(e.target.value))}
                required
              />
            </Field>
          </>
        )}
        {integration.key === "journald" && (
          <Field
            label={t("logCollection.units")}
            requirement={scope === "business" ? "required" : "optional"}
            hint={t("logCollection.journalHint")}
          >
            <Textarea
              value={units}
              onChange={(e) => setUnits(e.target.value)}
              rows={3}
              required={scope === "business"}
            />
          </Field>
        )}
        {integration.key === "eventlog" && (
          <Field label={t("logCollection.channel")} requirement="required">
            <Input
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
              required
            />
          </Field>
        )}
        <Field
          label={t("logCollection.startAt")}
          requirement="required"
          hint={t("logCollection.checkpointHint")}
        >
          <Select
            value={startAt}
            onValueChange={setStartAt}
            options={["end", "beginning"].map((value) => ({
              value,
              label: t(`logCollection.${value}`),
            }))}
          />
        </Field>
        <Switch
          label={t("monitoring.desiredEnabled")}
          checked={enabled}
          onChange={setEnabled}
        />
        {(error || invalid) && <p role="alert">{error || invalid}</p>}
      </FormDialog>
      {picker === "application" && (
        <ApplicationPicker
          multiple={false}
          selected={application ? [application] : []}
          onClose={() => setPicker(undefined)}
          onDone={(rows) => {
            setApplication(rows[0]);
            setService(undefined);
            setPicker(undefined);
          }}
        />
      )}
      {picker === "service" && application && (
        <ServicePicker
          application={application.application}
          onClose={() => setPicker(undefined)}
          onSelect={(row) => {
            setService(row);
            setPicker(undefined);
          }}
        />
      )}
      {picker === "collector" && (
        <CollectorPicker
          integration={integration}
          resourceType={isKubernetes ? "kubernetes_cluster" : "host"}
          requiredComponents={
            isKubernetes
              ? ["file_storage", "k8sattributes", "argus_log_identity"]
              : ["file_storage"]
          }
          onClose={() => setPicker(undefined)}
          onSelect={(row) => {
            setCollector(row);
            setWorkload(undefined);
            setPicker(undefined);
          }}
        />
      )}
    </>
  );
}
