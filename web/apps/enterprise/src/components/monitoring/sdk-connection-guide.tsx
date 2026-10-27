import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type MonitoringSchemas,
} from "@argus/api-client";
import {
  Alert,
  Button,
  CodeBlock,
  Dialog,
  Field,
  Input,
  SegmentedControl,
} from "@argus/ui";

type Endpoint = MonitoringSchemas["MonitoringIngestEndpoint"];
type Connection = MonitoringSchemas["MonitoringIngestConnection"];

const shellQuote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;

export function sdkConnectionExample(
  connection: Connection,
  transport: string,
  serviceName: string,
): string {
  const listener = connection.listeners.find(
    (item) => item.transport === transport,
  );
  if (!listener) return "";
  if (connection.protocol === "otlp") {
    const address = listener.endpoint.replace(/^grpcs:/, "https:");
    return [
      `export OTEL_SERVICE_NAME=${shellQuote(serviceName || "<service-name>")}`,
      `export OTEL_EXPORTER_OTLP_PROTOCOL=${shellQuote(transport === "grpc" ? "grpc" : "http/protobuf")}`,
      `export OTEL_EXPORTER_OTLP_ENDPOINT=${shellQuote(address)}`,
      'export OTEL_EXPORTER_OTLP_HEADERS="authorization=Bearer%20${ARGUS_APPLICATION_INGEST_KEY}"',
      "export OTEL_EXPORTER_OTLP_CERTIFICATE='argus-sdk-ca.pem'",
    ].join("\n");
  }
  return [
    `endpoint: ${listener.endpoint}`,
    `transport: ${listener.transport}`,
    `service.name: ${serviceName || "<service-name>"}`,
    "authorization: Bearer <application-ingest-key>",
    `tls.server_name: ${connection.tls.server_name}`,
    "tls.ca_file: argus-sdk-ca.pem",
  ].join("\n");
}

export function SDKConnectionGuide({
  endpoint,
  onClose,
}: {
  endpoint: Endpoint;
  onClose: () => void;
}) {
  const api = useApi();
  const { t, i18n } = useTranslation();
  const [transport, setTransport] = useState(""),
    [service, setService] = useState("");
  const query = useQuery({
    queryKey: [
      "monitoring",
      "ingest-connection",
      endpoint.id,
      endpoint.version,
    ],
    queryFn: () => api.monitoring.ingestConnection(endpoint.id),
    refetchInterval: 5000,
  });
  const connection = query.data;
  const active = connection?.listeners.some(
    (item) => item.transport === transport,
  )
    ? transport
    : (connection?.listeners[0]?.transport ?? "");
  const listener = connection?.listeners.find(
    (item) => item.transport === active,
  );
  const date = (value?: string) =>
    value
      ? new Date(value).toLocaleString(i18n.language)
      : t("monitoring.freshnessStates.no_data");
  const downloadCA = () => {
    if (!connection) return;
    const url = URL.createObjectURL(
      new Blob([connection.tls.ca_bundle_pem], {
        type: "application/x-pem-file",
      }),
    );
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "argus-sdk-ca.pem";
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  return (
    <Dialog
      open
      title={`${endpoint.name} · ${t("monitoring.ingest.guide.title")}`}
      size="lg"
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {query.isPending && <p role="status">{t("common.loading")}</p>}
      {query.error && (
        <p role="alert">
          {formatApiError(
            query.error,
            t("monitoring.errors.unavailable"),
            (requestId) => t("common.requestReference", { requestId }),
          )}
        </p>
      )}
      {connection && (
        <>
          <div className="argus-monitoring-form">
            <Field
              requirement="none"
              label={t("monitoring.ingest.guide.environment")}
            >
              <Input readOnly value={connection.environment} />
            </Field>
            <Field
              requirement="none"
              label={t("monitoring.ingest.guide.configuration")}
            >
              <Input
                readOnly
                value={t(
                  `monitoring.ingest.guide.configStates.${connection.configuration_status}`,
                )}
              />
            </Field>
            <Field
              requirement="none"
              label={t("monitoring.ingest.guide.observation")}
            >
              <Input
                readOnly
                value={t(
                  `monitoring.ingest.guide.observedStates.${connection.observation_state}`,
                )}
              />
            </Field>
            <Field requirement="none" label={t("monitoring.lastSample")}>
              <Input readOnly value={date(connection.last_received_at)} />
            </Field>
          </div>
          {connection.last_error_code && (
            <Alert
              tone="danger"
              title={t("monitoring.ingest.guide.rejected")}
              description={connection.last_error_code}
            />
          )}
          <Alert
            title={t("monitoring.ingest.guide.networkTitle")}
            description={t("monitoring.ingest.guide.networkHint")}
          />
          <SegmentedControl
            label={t("monitoring.ingest.protocol")}
            value={active}
            options={connection.listeners.map((item) => ({
              value: item.transport,
              label: item.transport.toUpperCase(),
            }))}
            onChange={setTransport}
          />
          <Field
            requirement="none"
            label={t("monitoring.ingest.guide.endpoint")}
          >
            <Input readOnly value={listener?.endpoint ?? ""} />
          </Field>
          <Field
            requirement="none"
            label={t("monitoring.ingest.guide.serverName")}
          >
            <Input readOnly value={connection.tls.server_name} />
          </Field>
          <Field
            requirement="optional"
            label={t("monitoring.ingest.guide.service")}
            hint={t("monitoring.ingest.guide.serviceHint")}
          >
            <Input
              value={service}
              onChange={(event) => setService(event.target.value)}
              maxLength={200}
            />
          </Field>
          <CodeBlock
            language={connection.protocol === "otlp" ? "sh" : "text"}
            code={sdkConnectionExample(connection, active, service)}
            label={t("monitoring.ingest.guide.example")}
          />
          <p>{t("monitoring.ingest.guide.keyHint")}</p>
          <div className="argus-monitoring-toolbar">
            <Button onPress={downloadCA}>
              {t("monitoring.ingest.guide.downloadCA")}
            </Button>
            <span>
              {t("monitoring.ingest.guide.trustEpoch", {
                epoch: connection.tls.trust_bundle_epoch,
              })}
            </span>
          </div>
          <Field
            requirement="none"
            label={t("monitoring.ingest.guide.caFingerprint")}
          >
            <Input readOnly value={connection.tls.ca_bundle_sha256} />
          </Field>
          <CodeBlock
            code={connection.tls.ca_bundle_pem}
            label={t("monitoring.ingest.guide.publicCA")}
          />
        </>
      )}
    </Dialog>
  );
}
