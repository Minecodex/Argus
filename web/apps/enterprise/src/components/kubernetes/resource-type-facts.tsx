import { useState } from "react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import type { KubernetesInventoryObject } from "@argus/api-client";
import {
  DataTable,
  Pagination,
  KeyValueGrid,
  formatResourceQuantity,
  type Column,
} from "@argus/ui";
import { asRecord } from "./resource-patch";
type Row = Record<string, unknown>;
const array = (value: unknown): Row[] =>
  Array.isArray(value) ? value.map(asRecord) : [];
const text = (value: unknown): string =>
  value === undefined || value === null || value === ""
    ? "—"
    : Array.isArray(value)
      ? value.map(text).join(", ")
      : typeof value === "object"
        ? Object.entries(asRecord(value))
            .map(([key, v]) => key + ": " + text(v))
            .join(" · ")
        : String(value);
function FactsTable({
  title,
  data,
  columns,
}: {
  title: string;
  data: Row[];
  columns: Column<Row>[];
}) {
  const [page, setPage] = useState(1),
    { t } = useTranslation(),
    current = Math.min(page, Math.max(1, Math.ceil(data.length / 20)));
  return (
    <section className="argus-k8s-facts">
      <h3>{title}</h3>
      {data.length ? (
        <DataTable
          data={data.slice((current - 1) * 20, current * 20)}
          getRowKey={(row) => String(data.indexOf(row))}
          columns={columns}
        />
      ) : (
        <p>{t("kubernetesResources.facts.empty")}</p>
      )}
      {data.length > 20 && (
        <Pagination
          page={current}
          totalPages={Math.ceil(data.length / 20)}
          onChange={setPage}
        />
      )}
    </section>
  );
}
export function KubernetesResourceTypeFacts({
  object,
}: {
  object: KubernetesInventoryObject;
}) {
  const { t } = useTranslation(),
    r = object.data,
    spec = asRecord(r.spec),
    status = asRecord(r.status),
    kind = object.definition.key;
  const label = (key: string) => t("kubernetesResources.facts." + key),
    column = (key: string, render?: (row: Row) => string): Column<Row> => ({
      key,
      header: label(key),
      render: (row) => (render ? render(row) : text(row[key])),
    });
  const table = (key: string, data: Row[], columns: Column<Row>[]) => (
    <FactsTable key={key} title={label(key)} data={data} columns={columns} />
  );
  const selector = (value: unknown) => {
    if (value === undefined) return "—";
    const source = asRecord(value),
      labels = asRecord(source.matchLabels),
      expressions = array(source.matchExpressions);
    const parts = [
      ...Object.entries(labels).map(([k, v]) => k + "=" + text(v)),
      ...expressions.map(
        (row) =>
          text(row.key) +
          " " +
          text(row.operator) +
          " (" +
          text(row.values) +
          ")",
      ),
    ];
    return parts.join(" · ") || label("all");
  };
  const conditions = Array.isArray(status.conditions)
    ? table("conditions", array(status.conditions), [
        column("type"),
        column("status"),
        column("reason"),
        column("message"),
        column("lastTransitionTime"),
        column("lastHeartbeatTime"),
      ])
    : null;
  let content: ReactNode = null;
  if (["role", "clusterrole"].includes(kind))
    content = table("rules", array(r.rules), [
      column("apiGroups", (row) =>
        Array.isArray(row.apiGroups) && row.apiGroups.includes("")
          ? text(
              (row.apiGroups as unknown[]).map((value) =>
                value === "" ? "core" : value,
              ),
            )
          : text(row.apiGroups),
      ),
      column("resources"),
      column("verbs"),
      column("resourceNames"),
      column("nonResourceURLs"),
    ]);
  if (["rolebinding", "clusterrolebinding"].includes(kind))
    content = (
      <>
        <KeyValueGrid
          columns={2}
          items={[{ label: label("roleRef"), value: text(r.roleRef) }]}
        />
        {table("subjects", array(r.subjects), [
          column("kind"),
          column("name"),
          column("namespace"),
          column("apiGroup"),
        ])}
      </>
    );
  if (kind === "serviceaccount")
    content = (
      <>
        {table(
          "references",
          [
            ...array(r.imagePullSecrets).map((row) => ({
              ...row,
              kind: "imagePullSecret",
            })),
            ...array(r.secrets).map((row) => ({ ...row, kind: "Secret" })),
          ],
          [column("kind"), column("name")],
        )}
      </>
    );
  if (kind === "node") {
    const labels = asRecord(asRecord(r.metadata).labels),
      roles = Object.keys(labels)
        .filter((key) => key.startsWith("node-role.kubernetes.io/"))
        .map((key) => key.slice("node-role.kubernetes.io/".length));
    content = (
      <>
        <KeyValueGrid
          columns={2}
          items={[
            { label: label("roles"), value: roles.join(", ") || "—" },
            {
              label: label("taints"),
              value:
                array(spec.taints)
                  .map(
                    (row) =>
                      text(row.key) +
                      (row.value ? "=" + text(row.value) : "") +
                      ":" +
                      text(row.effect),
                  )
                  .join(" · ") || "—",
            },
          ]}
        />
        {table("addresses", array(status.addresses), [
          column("type"),
          column("address"),
        ])}
        {table(
          "capacity",
          [
            ...new Set([
              ...Object.keys(asRecord(status.capacity)),
              ...Object.keys(asRecord(status.allocatable)),
            ]),
          ].map((name) => ({
            name,
            capacity: asRecord(status.capacity)[name],
            allocatable: asRecord(status.allocatable)[name],
          })),
          [
            column("name"),
            column("capacity", (row) =>
              row.name === "cpu" ||
              row.name === "memory" ||
              row.name === "ephemeral-storage"
                ? formatResourceQuantity(
                    row.capacity,
                    row.name === "cpu" ? "cpu" : "bytes",
                  )
                : text(row.capacity),
            ),
            column("allocatable", (row) =>
              row.name === "cpu" ||
              row.name === "memory" ||
              row.name === "ephemeral-storage"
                ? formatResourceQuantity(
                    row.allocatable,
                    row.name === "cpu" ? "cpu" : "bytes",
                  )
                : text(row.allocatable),
            ),
          ],
        )}
      </>
    );
  }
  if (kind === "service")
    content = (
      <>
        <KeyValueGrid
          columns={2}
          items={[
            { label: label("selector"), value: text(spec.selector) },
            { label: label("externalIPs"), value: text(spec.externalIPs) },
            {
              label: label("loadBalancer"),
              value:
                array(asRecord(status.loadBalancer).ingress)
                  .map((row) => text(row.ip ?? row.hostname))
                  .join(", ") || "—",
            },
          ]}
        />
        {table("ports", array(spec.ports), [
          column("name"),
          column("port"),
          column("targetPort"),
          column("protocol"),
          column("nodePort"),
        ])}
      </>
    );
  if (kind === "ingress") {
    const routes: Row[] = [];
    if (spec.defaultBackend)
      routes.push({
        host: "*",
        path: "*",
        pathType: label("default"),
        backend: text(
          asRecord(spec.defaultBackend).service ??
            asRecord(spec.defaultBackend).resource,
        ),
      });
    for (const rule of array(spec.rules))
      for (const path of array(asRecord(rule.http).paths))
        routes.push({
          host: rule.host ?? "*",
          path: path.path,
          pathType: path.pathType,
          backend: text(
            asRecord(path.backend).service ?? asRecord(path.backend).resource,
          ),
        });
    content = (
      <>
        <KeyValueGrid
          columns={2}
          items={[
            {
              label: label("ingressClass"),
              value: text(spec.ingressClassName),
            },
            {
              label: label("loadBalancer"),
              value:
                array(asRecord(status.loadBalancer).ingress)
                  .map((row) => text(row.ip ?? row.hostname))
                  .join(", ") || "—",
            },
          ]}
        />
        {table("routes", routes, [
          column("host"),
          column("path"),
          column("pathType"),
          column("backend"),
        ])}
        {table("tls", array(spec.tls), [column("hosts"), column("secretName")])}
      </>
    );
  }
  if (kind === "ingressclass")
    content = (
      <KeyValueGrid
        columns={2}
        items={[
          { label: label("controller"), value: text(spec.controller) },
          { label: label("parameters"), value: text(spec.parameters) },
        ]}
      />
    );
  if (kind === "networkpolicy") {
    const peers = (value: unknown) =>
      array(value)
        .map(
          (peer) =>
            [
              peer.podSelector !== undefined
                ? "Pod: " + selector(peer.podSelector)
                : "",
              peer.namespaceSelector !== undefined
                ? "Namespace: " + selector(peer.namespaceSelector)
                : "",
              peer.ipBlock
                ? text(asRecord(peer.ipBlock).cidr) +
                  (asRecord(peer.ipBlock).except
                    ? " except " + text(asRecord(peer.ipBlock).except)
                    : "")
                : "",
            ]
              .filter(Boolean)
              .join(" · ") || label("all"),
        )
        .join("; ") || label("all");
    content = (
      <>
        <KeyValueGrid
          columns={2}
          items={[
            { label: label("selector"), value: selector(spec.podSelector) },
            { label: label("policyTypes"), value: text(spec.policyTypes) },
          ]}
        />
        {table(
          "networkRules",
          [
            ...array(spec.ingress).map((row) => ({
              direction: "Ingress",
              peers: peers(row.from),
              ports: row.ports ? text(row.ports) : label("all"),
            })),
            ...array(spec.egress).map((row) => ({
              direction: "Egress",
              peers: peers(row.to),
              ports: row.ports ? text(row.ports) : label("all"),
            })),
          ],
          [column("direction"), column("peers"), column("ports")],
        )}
      </>
    );
  }
  if (kind === "pdb")
    content = (
      <KeyValueGrid
        columns={2}
        items={[
          { label: label("selector"), value: selector(spec.selector) },
          { label: label("expectedPods"), value: text(status.expectedPods) },
          {
            label: label("unhealthyPodEvictionPolicy"),
            value: text(spec.unhealthyPodEvictionPolicy),
          },
        ]}
      />
    );
  if (kind === "resourcequota")
    content = table(
      "quota",
      [
        ...new Set([
          ...Object.keys(asRecord(spec.hard)),
          ...Object.keys(asRecord(status.used)),
        ]),
      ].map((name) => ({
        name,
        used: asRecord(status.used)[name],
        hard: asRecord(spec.hard)[name],
        observedHard: asRecord(status.hard)[name],
      })),
      [column("name"), column("used"), column("hard"), column("observedHard")],
    );
  if (kind === "storageclass")
    content = (
      <KeyValueGrid
        columns={2}
        items={[
          { label: label("parameters"), value: text(r.parameters) },
          { label: label("mountOptions"), value: text(r.mountOptions) },
          {
            label: label("allowedTopologies"),
            value: text(r.allowedTopologies),
          },
        ]}
      />
    );
  if (["pv", "pvc"].includes(kind))
    content = (
      <KeyValueGrid
        columns={2}
        items={[
          { label: label("accessModes"), value: text(spec.accessModes) },
          { label: label("volumeMode"), value: text(spec.volumeMode) },
          {
            label: label("claimRef"),
            value:
              kind === "pv"
                ? text(asRecord(spec.claimRef).namespace) +
                  " / " +
                  text(asRecord(spec.claimRef).name)
                : text(spec.volumeName),
          },
          { label: label("mountOptions"), value: text(spec.mountOptions) },
        ]}
      />
    );
  if (kind === "statefulset")
    content = (
      <>
        <KeyValueGrid
          columns={2}
          items={[
            {
              label: label("podManagementPolicy"),
              value: text(spec.podManagementPolicy),
            },
            {
              label: label("partition"),
              value: text(
                asRecord(asRecord(spec.updateStrategy).rollingUpdate).partition,
              ),
            },
            {
              label: label("retentionPolicy"),
              value: text(spec.persistentVolumeClaimRetentionPolicy),
            },
          ]}
        />
        {table(
          "volumeTemplates",
          array(spec.volumeClaimTemplates).map((row) => ({
            name: asRecord(row.metadata).name,
            storageClassName: asRecord(row.spec).storageClassName,
            accessModes: asRecord(row.spec).accessModes,
            storage: asRecord(asRecord(asRecord(row.spec).resources).requests)
              .storage,
          })),
          [
            column("name"),
            column("storageClassName"),
            column("accessModes"),
            column("storage"),
          ],
        )}
      </>
    );
  if (kind === "cronjob")
    content = (
      <KeyValueGrid
        columns={2}
        items={[
          {
            label: label("successfulJobsHistoryLimit"),
            value: text(spec.successfulJobsHistoryLimit),
          },
          {
            label: label("failedJobsHistoryLimit"),
            value: text(spec.failedJobsHistoryLimit),
          },
          {
            label: label("startingDeadlineSeconds"),
            value: text(spec.startingDeadlineSeconds),
          },
          {
            label: label("activeJobs"),
            value:
              array(status.active)
                .map((row) => text(row.name))
                .join(", ") || "—",
          },
        ]}
      />
    );
  if (kind === "job")
    content = (
      <KeyValueGrid
        columns={2}
        items={[
          { label: label("parallelism"), value: text(spec.parallelism) },
          { label: label("completions"), value: text(spec.completions) },
          {
            label: label("duration"),
            value:
              status.startTime && status.completionTime
                ? Math.max(
                    0,
                    (Date.parse(String(status.completionTime)) -
                      Date.parse(String(status.startTime))) /
                      1000,
                  ) + " s"
                : "—",
          },
        ]}
      />
    );
  if (kind === "pod") {
    const containers: Row[] = [
      ...array(status.initContainerStatuses).map((row) => ({
        ...row,
        group: "init",
      })),
      ...array(status.containerStatuses).map((row) => ({
        ...row,
        group: "container",
      })),
      ...array(status.ephemeralContainerStatuses).map((row) => ({
        ...row,
        group: "ephemeral",
      })),
    ];
    content = (
      <>
        {table(
          "containerStates",
          containers.map((row) => {
            const state = asRecord(row.state),
              waiting = asRecord(state.waiting),
              terminated = asRecord(state.terminated),
              running = asRecord(state.running);
            return {
              ...row,
              state: Object.keys(state).join(", "),
              reason: waiting.reason ?? terminated.reason,
              message: waiting.message ?? terminated.message,
              startedAt: running.startedAt ?? terminated.startedAt,
              finishedAt: terminated.finishedAt,
              ready: row.ready === undefined ? "—" : String(row.ready),
            };
          }),
          [
            column("name"),
            column("group"),
            column("ready"),
            column("restartCount"),
            column("state"),
            column("reason"),
            column("message"),
            column("startedAt"),
            column("finishedAt"),
          ],
        )}
        {table(
          "volumes",
          array(spec.volumes).map((row) => ({
            name: row.name,
            source: Object.keys(row)
              .filter((key) => key !== "name")
              .join(", "),
            references: Object.entries(row)
              .filter(([key]) => key !== "name")
              .map(([key, value]) => key + ": " + text(value))
              .join(" · "),
          })),
          [column("name"), column("source"), column("references")],
        )}
      </>
    );
  }
  return content || conditions ? (
    <div className="argus-k8s-resource-detail">
      {content}
      {conditions}
    </div>
  ) : null;
}
