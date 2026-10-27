import { useTranslation } from "react-i18next";
import type {
  MonitoringIntegration,
  MonitoringSchemas,
} from "@argus/api-client";
import { Field, Input, Select, Switch } from "@argus/ui";

export type NativeOptions = MonitoringSchemas["MonitoringCollectionConfig"];
export const splitValues = (value: string) =>
  value
    .split(/[,\n]/)
    .map((v) => v.trim())
    .filter(Boolean);

export function NativeFields({
  integration,
  endpoint,
  setEndpoint,
  database,
  setDatabase,
  members,
  setMembers,
  hostnames,
  setHostnames,
  options,
  setOptions,
}: {
  integration: MonitoringIntegration;
  endpoint: string;
  setEndpoint: (v: string) => void;
  database: string;
  setDatabase: (v: string) => void;
  members: string;
  setMembers: (v: string) => void;
  hostnames: string;
  setHostnames: (v: string) => void;
  options: NativeOptions;
  setOptions: (v: NativeOptions) => void;
}) {
  const { t } = useTranslation();
  const form = integration.form;
  if (!form) return null;
  const field = (
    key: string,
    value: string,
    update: (v: string) => void,
    required = false,
    hint?: string,
  ) => (
    <Field
      key={key}
      label={t(`monitoring.${key}`)}
      requirement={required ? "required" : "optional"}
      hint={hint}
    >
      <Input
        value={value}
        onChange={(e) => update(e.target.value)}
        autoComplete="off"
        required={required}
      />
    </Field>
  );
  const textOption = (
    key: "replica_set" | "auth_source" | "topic_match" | "group_match",
    label: string,
  ) =>
    field(label, options[key] ?? "", (value) =>
      setOptions({ ...options, [key]: value }),
    );
  const choice = (
    key: "sasl_mechanism" | "method" | "dns_network" | "record_type",
    label: string,
    values: string[],
    fallback: string,
  ) => (
    <Field label={t(`monitoring.${label}`)} requirement="required">
      <Select
        value={options[key] ?? fallback}
        options={values.map((value) => ({ value, label: value }))}
        onValueChange={(value) => setOptions({ ...options, [key]: value })}
      />
    </Field>
  );
  const hint =
    integration.key === "nginx"
      ? t("monitoring.nginxEndpointHint")
      : integration.key === "apache"
        ? t("monitoring.apacheEndpointHint")
        : t(`monitoring.endpointHints.${form.endpoint_kind}`);
  const tlsEnabled =
    form.endpoint_kind === "http_url"
      ? endpoint.startsWith("https://")
      : Boolean(options.tls?.enabled);
  return (
    <>
      {form.endpoint_kind === "local" ? (
        <p>{t("monitoring.localIISHint")}</p>
      ) : (
        field("endpoint", endpoint, setEndpoint, true, hint)
      )}
      {form.database &&
        field(
          form.database_required ? "oracleService" : "database",
          database,
          setDatabase,
          form.database_required,
        )}
      {form.members &&
        field(
          "additionalMembers",
          members,
          setMembers,
          false,
          t("monitoring.commaSeparatedHint"),
        )}
      {form.hostnames &&
        field(
          "dnsHostnames",
          hostnames,
          setHostnames,
          true,
          t("monitoring.commaSeparatedHint"),
        )}
      {form.tls && (
        <Field label={t("monitoring.tls")} requirement="optional">
          <Switch
            label={t("monitoring.tls")}
            checked={tlsEnabled}
            onChange={(enabled) => setOptions({ ...options, tls: { enabled } })}
          />
        </Field>
      )}
      {form.tls_server_name &&
        tlsEnabled &&
        field("tlsServerName", options.tls?.server_name ?? "", (value) =>
          setOptions({
            ...options,
            tls: { enabled: true, server_name: value },
          }),
        )}
      {integration.key === "mongodb" && (
        <>
          {textOption("replica_set", "replicaSet")}
          {textOption("auth_source", "authSource")}
          <Field
            label={t("monitoring.directConnection")}
            requirement="optional"
          >
            <Switch
              label={t("monitoring.directConnection")}
              checked={options.direct_connection ?? false}
              onChange={(value) =>
                setOptions({ ...options, direct_connection: value })
              }
            />
          </Field>
        </>
      )}
      {integration.key === "kafka" && (
        <>
          {textOption("topic_match", "topicMatch")}
          {textOption("group_match", "groupMatch")}
          {choice(
            "sasl_mechanism",
            "saslMechanism",
            ["SCRAM-SHA-512", "SCRAM-SHA-256", "PLAIN"],
            "SCRAM-SHA-512",
          )}
        </>
      )}
      {integration.key === "elasticsearch" && (
        <>
          {field(
            "elasticNodes",
            options.nodes?.join(", ") ?? "",
            (value) => setOptions({ ...options, nodes: splitValues(value) }),
            false,
            t("monitoring.elasticScopeHint"),
          )}
          {field(
            "elasticIndices",
            options.indices?.join(", ") ?? "",
            (value) => setOptions({ ...options, indices: splitValues(value) }),
            false,
            t("monitoring.elasticScopeHint"),
          )}
        </>
      )}
      {integration.key === "http" &&
        choice("method", "httpMethod", ["GET", "HEAD", "OPTIONS"], "GET")}
      {integration.key === "dns" && (
        <>
          {choice(
            "dns_network",
            "dnsNetwork",
            ["udp", "tcp", "tcp-tls"],
            "udp",
          )}
          {choice(
            "record_type",
            "dnsRecordType",
            ["A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "SRV"],
            "A",
          )}
        </>
      )}
      {integration.key === "icmp" && (
        <Field
          label={t("monitoring.pingCount")}
          requirement="required"
          hint={t("monitoring.icmpPermissionHint")}
        >
          <Input
            type="number"
            min={1}
            max={10}
            value={options.ping_count ?? 3}
            onChange={(e) =>
              setOptions({ ...options, ping_count: Number(e.target.value) })
            }
          />
        </Field>
      )}
    </>
  );
}
