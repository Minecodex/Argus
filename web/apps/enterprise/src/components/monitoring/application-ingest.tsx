import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type MonitoringApplication,
  type MonitoringSchemas,
} from "@argus/api-client";
import {
  Button,
  Pagination,
  DataTable,
  Dialog,
  EmptyState,
  Field,
  FormDialog,
  Input,
  Select,
  Switch,
} from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { useMonitoringActions } from "./actions";
import { CollectorPicker, type CollectorChoice } from "./pickers";
import { SDKConnectionGuide } from "./sdk-connection-guide";

type Endpoint = MonitoringSchemas["MonitoringIngestEndpoint"];
type Protocol = Endpoint["protocol"];

export function ApplicationIngest({
  application,
}: {
  application: MonitoringApplication;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    actions = useMonitoringActions();
  const canManage = usePermission("application.manage"),
    canCollect = usePermission("collector.use");
  const [creating, setCreating] = useState(false),
    [selected, setSelected] = useState<Endpoint>(),
    [connection, setConnection] = useState<Endpoint>();
  const [cursors, setCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const query = useQuery({
    queryKey: [
      "monitoring",
      "ingest-endpoints",
      application.id,
      cursors.at(-1),
    ],
    queryFn: () =>
      api.monitoring.ingestEndpoints(application.id, {
        cursor: cursors.at(-1),
        limit: 30,
      }),
  });
  return (
    <section className="argus-monitoring-ingest">
      <div className="argus-monitoring-toolbar">
        <h2>{t("monitoring.ingest.title")}</h2>
        {canManage && canCollect && application.status === "active" && (
          <Button onPress={() => setCreating(true)}>
            {t("monitoring.ingest.create")}
          </Button>
        )}
      </div>
      <p>{t("monitoring.ingest.discoveryHint")}</p>
      {query.isPending && <p role="status">{t("common.loading")}</p>}
      {(query.error || actions.error) && (
        <p role="alert">
          {actions.error ||
            formatApiError(
              query.error,
              t("monitoring.errors.unavailable"),
              (requestId) => t("common.requestReference", { requestId }),
            )}
        </p>
      )}
      {query.data &&
        (query.data.items.length ? (
          <DataTable
            data={query.data.items}
            getRowKey={(v) => v.id}
            columns={[
              { key: "name", header: t("monitoring.ingest.name") },
              {
                key: "protocol",
                header: t("monitoring.ingest.protocol"),
                render: (v) => v.protocol.toUpperCase(),
              },
              {
                key: "signals",
                header: t("monitoring.signals"),
                render: (v) =>
                  v.signals
                    .map((signal) => t(`monitoring.signalsLabel.${signal}`))
                    .join(" / "),
              },
              {
                key: "last_received_at",
                header: t("monitoring.lastSample"),
                render: (v) =>
                  v.last_received_at
                    ? new Date(v.last_received_at).toLocaleString(i18n.language)
                    : t("monitoring.freshnessStates.no_data"),
              },
              {
                key: "enabled",
                header: t("monitoring.desiredEnabled"),
                render: (v) => (
                  <Switch
                    label={t("monitoring.toggleCollection", { name: v.name })}
                    checked={v.enabled}
                    disabled={!canManage || actions.busy}
                    onChange={(enabled) =>
                      void actions.preview({
                        kind: "endpoint",
                        operation: "set_enabled",
                        id: v.id,
                        expected_version: v.version,
                        endpoint: { enabled },
                      })
                    }
                  />
                ),
              },
              {
                key: "actions",
                header: t("monitoring.actions"),
                render: (v) => (
                  <div className="argus-monitoring-toolbar">
                    <Button size="sm" onPress={() => setConnection(v)}>
                      {t("monitoring.ingest.guide.title")}
                    </Button>
                    <Button size="sm" onPress={() => setSelected(v)}>
                      {t("monitoring.ingest.manageKeys")}
                    </Button>
                  </div>
                ),
              },
            ]}
          />
        ) : (
          <EmptyState
            title={t("monitoring.ingest.empty")}
            description={t("monitoring.ingest.discoveryHint")}
          />
        ))}
      <Pagination
        page={cursors.length}
        hasNext={!!query.data?.next_cursor}
        isDisabled={query.isFetching}
        onChange={(page) =>
          setCursors((old) =>
            page <= old.length
              ? old.slice(0, page)
              : [...old, query.data?.next_cursor],
          )
        }
      />
      {creating && (
        <EndpointForm
          application={application}
          onClose={() => setCreating(false)}
          actions={actions}
        />
      )}
      {selected && (
        <IngestKeys
          endpoint={
            query.data?.items.find((v) => v.id === selected.id) ?? selected
          }
          canManage={canManage}
          onClose={() => setSelected(undefined)}
        />
      )}
      {connection && (
        <SDKConnectionGuide
          endpoint={
            query.data?.items.find((v) => v.id === connection.id) ?? connection
          }
          onClose={() => setConnection(undefined)}
        />
      )}
      {actions.confirmation}
    </section>
  );
}

function EndpointForm({
  application,
  onClose,
  actions,
}: {
  application: MonitoringApplication;
  onClose: () => void;
  actions: ReturnType<typeof useMonitoringActions>;
}) {
  const { t } = useTranslation();
  const [name, setName] = useState(""),
    [protocol, setProtocol] = useState<Protocol>("otlp"),
    [collector, setCollector] = useState<CollectorChoice>(),
    [picker, setPicker] = useState(false);
  const [signals, setSignals] = useState<string[]>(["traces"]),
    [invalid, setInvalid] = useState("");
  const submit = async () => {
    if (!name.trim() || !collector || !signals.length) {
      setInvalid(t("monitoring.completeRequired"));
      return;
    }
    if (
      await actions.preview({
        kind: "endpoint",
        operation: "create",
        endpoint: {
          name: name.trim(),
          application_id: application.id,
          collector_id: collector.id,
          protocol,
          signals: signals as Endpoint["signals"],
          service_allowlist: [],
          enabled: true,
        },
      })
    )
      onClose();
  };
  return (
    <>
      <FormDialog
        open
        title={t("monitoring.ingest.create")}
        description={`${application.name} · ${t(`monitoring.env.${application.environment}`, { defaultValue: application.environment })}`}
        onOpenChange={(open) => {
          if (!open) onClose();
        }}
        loading={actions.busy}
        onSubmit={() => void submit()}
        submitLabel={t("monitoring.preview")}
      >
        <Field label={t("monitoring.ingest.name")} requirement="required">
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
          />
        </Field>
        <Field label={t("monitoring.ingest.protocol")} requirement="required">
          <Select
            value={protocol}
            options={["otlp", "jaeger", "skywalking"].map((value) => ({
              value,
              label: value.toUpperCase(),
              disabled: value === "skywalking",
              description:
                value === "skywalking"
                  ? t(
                      "monitoring.reasons.upstream_acknowledgement_and_payload_logging",
                    )
                  : undefined,
            }))}
            onValueChange={(value) => {
              setProtocol(value as Protocol);
              setSignals(["traces"]);
              setCollector(undefined);
            }}
          />
        </Field>
        {protocol === "otlp" && (
          <Field label={t("monitoring.signals")} requirement="required">
            <div className="argus-monitoring-fields">
              {["traces", "logs", "metrics"].map((signal) => (
                <Switch
                  key={signal}
                  label={t(`monitoring.signalsLabel.${signal}`)}
                  checked={signals.includes(signal)}
                  onChange={(enabled) =>
                    setSignals((old) =>
                      enabled
                        ? [...old, signal]
                        : old.filter((v) => v !== signal),
                    )
                  }
                />
              ))}
            </div>
          </Field>
        )}
        <Field
          label={t("monitoring.executingCollector")}
          requirement="required"
        >
          <Button onPress={() => setPicker(true)}>
            {collector?.name ?? t("monitoring.chooseCollector")}
          </Button>
        </Field>
        <p>{t("monitoring.ingest.discoveryHint")}</p>
        {(invalid || actions.error) && (
          <p role="alert">{invalid || actions.error}</p>
        )}
      </FormDialog>
      {picker && (
        <CollectorPicker
          integration={{
            component: protocol,
            platforms: ["linux_amd64", "linux_arm64", "windows_amd64"],
          }}
          requiredComponents={[
            "argus_application_auth",
            "argus_application_identity",
          ]}
          onSelect={setCollector}
          onClose={() => setPicker(false)}
        />
      )}
    </>
  );
}

function IngestKeys({
  endpoint,
  canManage,
  onClose,
}: {
  endpoint: Endpoint;
  canManage: boolean;
  onClose: () => void;
}) {
  const api = useApi(),
    cache = useQueryClient(),
    { t, i18n } = useTranslation(),
    actions = useMonitoringActions();
  const [cursors, setCursors] = useState<Array<string | undefined>>([
      undefined,
    ]),
    [value, setValue] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [days, setDays] = useState(90);
  const query = useQuery({
    queryKey: ["monitoring", "ingest-keys", endpoint.id, cursors.at(-1)],
    queryFn: () =>
      api.monitoring.ingestKeys(endpoint.id, {
        cursor: cursors.at(-1),
        limit: 30,
      }),
  });
  const reveal = async (id: string) => {
    setBusy(true);
    setError("");
    try {
      const response = await api.monitoring.revealIngestKey(id);
      setValue(response.key);
      void cache.invalidateQueries({
        queryKey: ["monitoring", "ingest-keys", endpoint.id],
      });
    } catch (cause) {
      setError(
        formatApiError(cause, t("monitoring.errors.unavailable"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Dialog
        open
        title={`${endpoint.name} · ${t("monitoring.ingest.manageKeys")}`}
        size="lg"
        onOpenChange={(open) => {
          if (!open) {
            setValue("");
            onClose();
          }
        }}
      >
        <p>{t("monitoring.ingest.rotationHint")}</p>
        {canManage && endpoint.enabled && (
          <div className="argus-monitoring-toolbar">
            <Field
              label={t("monitoring.ingest.expiresDays")}
              requirement="required"
            >
              <Input
                type="number"
                min={1}
                max={365}
                value={days}
                onChange={(e) => setDays(Number(e.target.value))}
              />
            </Field>
            <Button
              isDisabled={
                actions.busy ||
                !Number.isInteger(days) ||
                days < 1 ||
                days > 365
              }
              onPress={() =>
                void actions.preview({
                  kind: "ingest_key",
                  operation: "create",
                  ingest_key: {
                    endpoint_id: endpoint.id,
                    expires_at: new Date(
                      Date.now() + days * 86400000,
                    ).toISOString(),
                  },
                })
              }
            >
              {t("monitoring.ingest.issueKey")}
            </Button>
          </div>
        )}
        {(query.error || error || actions.error) && (
          <p role="alert">
            {error ||
              actions.error ||
              formatApiError(
                query.error,
                t("monitoring.errors.unavailable"),
                (requestId) => t("common.requestReference", { requestId }),
              )}
          </p>
        )}
        {query.data && (
          <DataTable
            data={query.data.items}
            getRowKey={(key) => key.id}
            columns={[
              { key: "prefix", header: t("monitoring.ingest.keyPrefix") },
              {
                key: "expires_at",
                header: t("monitoring.ingest.expires"),
                render: (key) =>
                  key.expires_at
                    ? new Date(key.expires_at).toLocaleString(i18n.language)
                    : "—",
              },
              {
                key: "revoked_at",
                header: t("monitoring.status"),
                render: (key) =>
                  t(
                    key.revoked_at
                      ? "monitoring.ingest.revoked"
                      : key.expires_at &&
                          Date.parse(key.expires_at) <= Date.now()
                        ? "monitoring.ingest.expired"
                        : "monitoring.ingest.active",
                  ),
              },
              {
                key: "actions",
                header: t("monitoring.actions"),
                render: (key) =>
                  canManage &&
                  !key.revoked_at && (
                    <div className="argus-monitoring-toolbar">
                      {!key.revealed && (
                        <Button
                          size="sm"
                          isDisabled={busy}
                          onPress={() => void reveal(key.id)}
                        >
                          {t("monitoring.ingest.reveal")}
                        </Button>
                      )}
                      <Button
                        size="sm"
                        isDisabled={actions.busy}
                        onPress={() =>
                          void actions.preview({
                            kind: "ingest_key",
                            operation: "revoke",
                            id: key.id,
                            expected_version: key.version,
                            ingest_key: {},
                          })
                        }
                      >
                        {t("monitoring.ingest.revoke")}
                      </Button>
                    </div>
                  ),
              },
            ]}
          />
        )}
        <Pagination
          page={cursors.length}
          hasNext={!!query.data?.next_cursor}
          isDisabled={query.isFetching}
          onChange={(page) =>
            setCursors((old) =>
              page <= old.length
                ? old.slice(0, page)
                : [...old, query.data?.next_cursor],
            )
          }
        />
      </Dialog>
      {value && (
        <Dialog
          open
          title={t("monitoring.ingest.reveal")}
          onOpenChange={(open) => {
            if (!open) setValue("");
          }}
        >
          <p>{t("monitoring.ingest.revealHint")}</p>
          <Field label={t("monitoring.ingest.keyValue")} requirement="required">
            <Input value={value} readOnly autoComplete="off" />
          </Field>
        </Dialog>
      )}
      {actions.confirmation}
    </>
  );
}
