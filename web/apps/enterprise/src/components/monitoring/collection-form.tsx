import { useState } from "react";
import { useTranslation } from "react-i18next";
import type {
  MonitoringIntegration,
  MonitoringMutation,
} from "@argus/api-client";
import { Badge, Button, Field, FormDialog, Input, Switch } from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { NativeFields, splitValues, type NativeOptions } from "./native-fields";
import {
  ApplicationPicker,
  CollectorPicker,
  ServicePicker,
  type ApplicationAssociation,
  type CollectorChoice,
} from "./pickers";

export function CollectionForm({
  integration,
  busy,
  error,
  onSubmit,
  onClose,
  initial,
}: {
  integration: MonitoringIntegration;
  busy: boolean;
  error: string;
  onSubmit: (input: MonitoringMutation) => Promise<boolean>;
  onClose: () => void;
  initial?: { name?: string; endpoint?: string; database?: string };
}) {
  const { t } = useTranslation(),
    canManageApps = usePermission("application.manage");
  const [name, setName] = useState(initial?.name ?? ""),
    [endpoint, setEndpoint] = useState(
      initial?.endpoint ??
        (integration.form?.endpoint_kind === "local" ? "local" : ""),
    ),
    [database, setDatabase] = useState(initial?.database ?? "");
  const [username, setUsername] = useState(""),
    [password, setPassword] = useState("");
  const [interval, setInterval] = useState(30),
    [timeout, setTimeout] = useState(10),
    [enabled, setEnabled] = useState(true);
  const [members, setMembers] = useState(""),
    [hostnames, setHostnames] = useState("");
  const [options, setOptions] = useState<NativeOptions>({});
  const [collector, setCollector] = useState<CollectorChoice>(),
    [applications, setApplications] = useState<ApplicationAssociation[]>([]);
  const [picker, setPicker] = useState<
      "applications" | "collector" | undefined
    >(),
    [serviceFor, setServiceFor] = useState<ApplicationAssociation>();
  const [invalid, setInvalid] = useState("");
  const authenticated = integration.form?.authentication !== "none";
  const requiredAuthentication =
    integration.form?.authentication === "required";
  const submit = async () => {
    if (
      !name.trim() ||
      !endpoint.trim() ||
      !collector ||
      interval < 1 ||
      interval > 3600 ||
      timeout < 1 ||
      timeout > interval ||
      timeout > 60 ||
      (integration.form?.database_required && !database.trim()) ||
      (integration.form?.hostnames && !splitValues(hostnames).length) ||
      (requiredAuthentication && !username.trim()) ||
      (requiredAuthentication && integration.key !== "mysql" && !password)
    ) {
      setInvalid(t("monitoring.completeRequired"));
      return;
    }
    setInvalid("");
    const credential = authenticated && Boolean(username || password);
    const input: MonitoringMutation = {
      kind: "collection",
      operation: "create",
      collection: {
        name: name.trim(),
        target_name: name.trim(),
        integration: integration.key,
        target_identity: {
          endpoint: endpoint.trim(),
          ...(database ? { database } : {}),
          ...(members ? { members: splitValues(members) } : {}),
          ...(hostnames ? { hostnames: splitValues(hostnames) } : {}),
        },
        collector_id: collector.id,
        enabled,
        config: {
          interval: `${interval}s`,
          timeout: `${timeout}s`,
          ...options,
        },
        credential_update: credential ? "replace" : "keep",
        bindings: applications.map((item) => ({
          application_id: item.application.id,
          service_id: item.service?.id,
        })),
      },
      ...(credential ? { credentials: { username, password } } : {}),
    };
    if (await onSubmit(input)) {
      setUsername("");
      setPassword("");
      onClose();
    }
  };
  return (
    <>
      <FormDialog
        open
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
        title={t("monitoring.createIntegration", { name: integration.name })}
        description={t("monitoring.creationHint")}
        loading={busy}
        onSubmit={() => void submit()}
        submitLabel={t("monitoring.previewCollection")}
      >
        <Field label={t("monitoring.collectionName")} requirement="required">
          <Input
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            autoComplete="off"
          />
        </Field>
        <NativeFields
          integration={integration}
          endpoint={endpoint}
          setEndpoint={setEndpoint}
          database={database}
          setDatabase={setDatabase}
          members={members}
          setMembers={setMembers}
          hostnames={hostnames}
          setHostnames={setHostnames}
          options={options}
          setOptions={setOptions}
        />
        <Field
          label={t("monitoring.executingCollector")}
          requirement="required"
          hint={t("monitoring.collectorHint")}
        >
          <Button type="button" onPress={() => setPicker("collector")}>
            {collector
              ? `${collector.name} · ${collector.platform}`
              : t("monitoring.chooseCollector")}
          </Button>
        </Field>
        {authenticated && (
          <div className="argus-monitoring-fields">
            <Field
              label={t("monitoring.username")}
              requirement={requiredAuthentication ? "required" : "optional"}
            >
              <Input
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="off"
              />
            </Field>
            <Field
              label={t("monitoring.password")}
              requirement={
                requiredAuthentication && integration.key !== "mysql"
                  ? "required"
                  : "optional"
              }
              hint={t("monitoring.privateCredentialHint")}
            >
              <Input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                autoComplete="new-password"
              />
            </Field>
          </div>
        )}
        <div className="argus-monitoring-fields">
          <Field label={t("monitoring.intervalSeconds")} requirement="required">
            <Input
              type="number"
              min={1}
              max={3600}
              value={interval}
              onChange={(event) => setInterval(Number(event.target.value))}
            />
          </Field>
          <Field label={t("monitoring.timeoutSeconds")} requirement="required">
            <Input
              type="number"
              min={1}
              max={Math.min(interval, 60)}
              value={timeout}
              onChange={(event) => setTimeout(Number(event.target.value))}
            />
          </Field>
        </div>
        <Field
          label={t("monitoring.associatedApplications")}
          requirement="optional"
          hint={t("monitoring.applicationContextHint")}
        >
          <Button
            type="button"
            isDisabled={!canManageApps}
            onPress={() => setPicker("applications")}
          >
            {t("monitoring.chooseApplications")}
          </Button>
          {applications.map((item) => (
            <div
              className="argus-monitoring-association"
              key={item.application.id}
            >
              <strong>{item.application.name}</strong>
              <Badge>
                {t(`monitoring.env.${item.application.environment}`, {
                  defaultValue: item.application.environment,
                })}
              </Badge>
              <Button
                type="button"
                size="sm"
                onPress={() => setServiceFor(item)}
              >
                {item.service?.service_name ??
                  t("monitoring.chooseOptionalService")}
              </Button>
              <Button
                type="button"
                size="sm"
                onPress={() =>
                  setApplications((old) =>
                    old.filter(
                      (value) => value.application.id !== item.application.id,
                    ),
                  )
                }
              >
                {t("monitoring.removeAssociation")}
              </Button>
            </div>
          ))}
        </Field>
        <Field label={t("monitoring.desiredEnabled")} requirement="required">
          <Switch
            label={t("monitoring.desiredEnabled")}
            checked={enabled}
            onChange={setEnabled}
          />
        </Field>
        {(invalid || error) && <p role="alert">{invalid || error}</p>}
      </FormDialog>
      {picker === "applications" && (
        <ApplicationPicker
          selected={applications}
          onDone={setApplications}
          onClose={() => setPicker(undefined)}
        />
      )}
      {picker === "collector" && (
        <CollectorPicker
          integration={integration}
          onSelect={setCollector}
          onClose={() => setPicker(undefined)}
        />
      )}
      {serviceFor && (
        <ServicePicker
          application={serviceFor.application}
          onSelect={(service) =>
            setApplications((old) =>
              old.map((item) =>
                item.application.id === serviceFor.application.id
                  ? { ...item, service }
                  : item,
              ),
            )
          }
          onClose={() => setServiceFor(undefined)}
        />
      )}
    </>
  );
}
