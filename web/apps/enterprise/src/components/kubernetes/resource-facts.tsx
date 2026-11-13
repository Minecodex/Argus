import { useTranslation } from "react-i18next";
import type { KubernetesInventoryObject } from "@argus/api-client";
import { Badge, CodeBlock, DataTable, KeyValueGrid } from "@argus/ui";
import { asRecord } from "./resource-patch";
import { KubernetesResourceTypeFacts } from "./resource-type-facts";

const rows = (value: unknown) =>
  Array.isArray(value) ? value.map(asRecord) : [];
const pairs = (value: unknown) =>
  Object.entries(asRecord(value))
    .map(([key, val]) => `${key}: ${String(val)}`)
    .join(" · ") || "—";
const metricKey = (metric: Record<string, unknown>) => {
  const content = asRecord(
    metric[String(metric.type).replace(/^./, (c) => c.toLowerCase())],
  );
  return [
    metric.type,
    content.name,
    asRecord(content.metric).name,
    content.container,
    asRecord(content.describedObject).name,
  ]
    .filter(Boolean)
    .join(" · ");
};
const metricValue = (
  metric: Record<string, unknown>,
  field: "current" | "target",
) => {
  const content = asRecord(
      metric[String(metric.type).replace(/^./, (c) => c.toLowerCase())],
    ),
    value = asRecord(content[field]);
  if (value.averageUtilization !== undefined)
    return `${value.averageUtilization}%`;
  return String(value.averageValue ?? value.value ?? "—");
};

export function KubernetesResourceFacts({
  object,
}: {
  object: KubernetesInventoryObject;
}) {
  const { t } = useTranslation(),
    spec = asRecord(object.data.spec),
    status = asRecord(object.data.status),
    kind = object.definition.key;
  const heading = (key: string) => t("kubernetesResources." + key);
  if (kind === "hpa") {
    const current = rows(status.currentMetrics),
      metrics = rows(spec.metrics);
    return (
      <section className="argus-k8s-facts">
        <h3>{heading("autoscalingMetrics")}</h3>
        <DataTable
          data={metrics}
          getRowKey={metricKey}
          columns={[
            { key: "metric", header: heading("metric"), render: metricKey },
            {
              key: "current",
              header: heading("currentValue"),
              render: (row) => {
                const observed = current.find(
                  (item) => metricKey(item) === metricKey(row),
                );
                return observed
                  ? metricValue(observed, "current")
                  : heading("notObserved");
              },
            },
            {
              key: "target",
              header: heading("targetValue"),
              render: (row) => metricValue(row, "target"),
            },
          ]}
        />
      </section>
    );
  }
  if (kind === "limitrange")
    return (
      <section className="argus-k8s-facts">
        <h3>{heading("resourceConstraints")}</h3>
        <DataTable
          data={rows(spec.limits)}
          getRowKey={(row) => String(row.type)}
          columns={[
            { key: "type", header: t("kubernetesWorkbench.type") },
            ...[
              "min",
              "max",
              "default",
              "defaultRequest",
              "maxLimitRequestRatio",
            ].map((key) => ({
              key,
              header: heading("constraints." + key),
              render: (row: Record<string, unknown>) => pairs(row[key]),
            })),
          ]}
        />
      </section>
    );
  if (kind === "endpointslice")
    return (
      <section className="argus-k8s-facts">
        <h3>{heading("endpointReadiness")}</h3>
        <DataTable
          data={rows(object.data.endpoints).map(
            (row, index): Record<string, unknown> & { index: number } => ({
              ...row,
              index,
            }),
          )}
          getRowKey={(row) => String(row.index)}
          columns={[
            {
              key: "addresses",
              header: heading("addresses"),
              render: (row) =>
                Array.isArray(row.addresses) ? row.addresses.join(", ") : "—",
            },
            { key: "nodeName", header: t("kubernetesResources.fields.node") },
            ...["ready", "serving", "terminating"].map((key) => ({
              key,
              header: heading("endpointStates." + key),
              render: (row: Record<string, unknown>) => (
                <Badge
                  tone={
                    asRecord(row.conditions)[key] === true
                      ? "success"
                      : "neutral"
                  }
                >
                  {asRecord(row.conditions)[key] === undefined
                    ? heading("notObserved")
                    : asRecord(row.conditions)[key]
                      ? t("kubernetesResources.yes")
                      : t("kubernetesResources.no")}
                </Badge>
              ),
            })),
          ]}
        />
      </section>
    );
  if (kind === "crd")
    return (
      <section className="argus-k8s-facts">
        <h3>{heading("apiVersions")}</h3>
        <DataTable
          data={rows(spec.versions)}
          getRowKey={(row) => String(row.name)}
          columns={[
            { key: "name", header: heading("version") },
            ...["served", "storage"].map((key) => ({
              key,
              header: heading("crd." + key),
              render: (row: Record<string, unknown>) =>
                row[key]
                  ? t("kubernetesResources.yes")
                  : t("kubernetesResources.no"),
            })),
          ]}
        />
        <p>{heading("crd.instancesHint")}</p>
      </section>
    );
  return <KubernetesResourceTypeFacts object={object} />;
}

export function KubernetesCRDSchema({
  object,
}: {
  object: KubernetesInventoryObject;
}) {
  const { t } = useTranslation();
  if (object.definition.key !== "crd") return null;
  return (
    <section className="argus-k8s-facts">
      <h3>{t("kubernetesResources.crd.schema")}</h3>
      {rows(asRecord(object.data.spec).versions).map((version) => (
        <details key={String(version.name)}>
          <summary>{String(version.name)}</summary>
          <KeyValueGrid
            columns={1}
            items={[
              {
                label: t("kubernetesResources.crd.required"),
                value: String(
                  asRecord(asRecord(version.schema).openAPIV3Schema).required ??
                    "—",
                ),
              },
            ]}
          />
          <CodeBlock
            language="json"
            code={JSON.stringify(
              asRecord(version.schema).openAPIV3Schema ?? {},
              null,
              2,
            )}
          />
        </details>
      ))}
    </section>
  );
}
