import { formatResourceField } from "./resource-model";
import { useTranslation } from "react-i18next";
import type { KubernetesInventoryObject } from "@argus/api-client";
import { CodeBlock, DataTable, KeyValueGrid } from "@argus/ui";
import { asRecord } from "./resource-patch";
import { KubernetesCRDSchema } from "./resource-facts";
import { KubernetesResourceTypeFacts } from "./resource-type-facts";
export function KubernetesResourceConfiguration({
  object,
}: {
  object: KubernetesInventoryObject;
}) {
  const { t } = useTranslation(),
    data = object.data,
    kind = object.definition.key,
    metadata = asRecord(data.metadata),
    spec = asRecord(data.spec);
  const podSpec =
    kind === "pod"
      ? spec
      : kind === "cronjob"
        ? asRecord(
            asRecord(asRecord(asRecord(spec.jobTemplate).spec).template).spec,
          )
        : asRecord(asRecord(spec.template).spec);
  const containers = [
    ...(Array.isArray(podSpec.containers) ? podSpec.containers : []),
    ...(Array.isArray(podSpec.initContainers) ? podSpec.initContainers : []),
  ].map(asRecord);
  const pairs = (value: unknown) =>
    Object.entries(asRecord(value)).map(([label, item]) => ({
      label,
      value: String(item ?? "—"),
    }));
  return (
    <div className="argus-k8s-resource-detail">
      <KubernetesCRDSchema object={object} />
      <KubernetesResourceTypeFacts object={object} />
      {object.redacted && <p>{t("kubernetesWorkbench.projected")}</p>}
      {containers.length > 0 && (
        <section>
          <h3>{t("kubernetesResources.containers")}</h3>
          <DataTable
            data={containers}
            getRowKey={(row) => String(row.name)}
            columns={[
              { key: "name", header: t("kubernetesWorkbench.name") },
              { key: "image", header: t("kubernetesResources.image") },
              {
                key: "requests",
                header: t("kubernetesResources.schema.requests"),
                render: (row) =>
                  Object.entries(asRecord(asRecord(row.resources).requests))
                    .map(([key, value]) => key + ": " + String(value))
                    .join(" · ") || "—",
              },
              {
                key: "limits",
                header: t("kubernetesResources.schema.limits"),
                render: (row) =>
                  Object.entries(asRecord(asRecord(row.resources).limits))
                    .map(([key, value]) => key + ": " + String(value))
                    .join(" · ") || "—",
              },
            ]}
          />
        </section>
      )}
      {["configmap", "secret"].includes(kind) && (
        <section>
          <h3>{t("kubernetesResources.data")}</h3>
          {Object.entries(asRecord(data.data)).map(([key, value]) => (
            <div key={key}>
              <strong>{key}</strong>
              <CodeBlock code={String(value)} language="text" />
            </div>
          ))}
        </section>
      )}
      <section>
        <h3>{t("kubernetesResources.labels")}</h3>
        <KeyValueGrid columns={2} items={pairs(metadata.labels)} />
      </section>
      <section>
        <h3>{t("kubernetesResources.annotations")}</h3>
        <KeyValueGrid columns={1} items={pairs(metadata.annotations)} />
      </section>
      <KeyValueGrid
        columns={2}
        items={Object.entries(object.summary?.fields ?? {})
          .filter(([key]) => !["involved_uid", "event_time"].includes(key))
          .map(([key, value]) => ({
            label: t("kubernetesResources.fields." + key, {
              defaultValue: key,
            }),
            value: formatResourceField(key, value),
          }))}
      />
    </div>
  );
}
