import { visibleReferenceNames } from "./resource-relations";
import { KubernetesResourceConfiguration } from "./resource-configuration";
import { KubernetesResourceFacts } from "./resource-facts";
import {
  KubernetesResourceMetrics,
  supportsResourceMetrics,
} from "./resource-metrics";
import { KubernetesHPANotice } from "./hpa-notice";
import { useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  ApiError,
  type KubernetesResourceReference,
} from "@argus/api-client";
import {
  DrawerPanel,
  Button,
  Badge,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Spinner,
  KeyValueGrid,
  CodeBlock,
  DataTable,
  Alert,
  Pagination,
  EmptyState,
  formatStructuredDocument,
  resourceKindName,
} from "@argus/ui";
import { useInventoryError } from "./inventory-error";
import { stateTone, formatResourceField } from "./resource-model";
import { operationResource } from "./operation-result";

export function KubernetesResourceDetail({
  clusterId,
  reference,
  full = false,
  onClose,
  onExpand,
  onOpen,
  feedback,
  onOperationDetails,
}: {
  clusterId: string;
  reference: KubernetesResourceReference;
  full?: boolean;
  feedback?: ReactNode;
  onClose: () => void;
  onExpand: () => void;
  onOpen: (ref: KubernetesResourceReference) => void;
  onOperationDetails?: (id: string) => void;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    formatError = useInventoryError();
  const [tab, setTab] = useState("overview");
  const [cursors, setCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const detail = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "object",
      reference.uid,
      reference.namespace_id,
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.object(
        clusterId,
        {
          type: reference.type,
          name: reference.name,
          uid: reference.uid,
          namespace_id: reference.namespace_id,
        },
        signal,
      ),
  });
  const related = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "relations",
      reference.uid,
      reference.namespace_id,
      tab,
      cursors.at(-1),
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.related(
        clusterId,
        {
          type: reference.type,
          name: reference.name,
          uid: reference.uid,
          namespace_id: reference.namespace_id,
          view: tab as "relations" | "events",
          limit: 30,
          cursor: cursors.at(-1),
        },
        signal,
      ),
    enabled:
      (tab === "relations" || tab === "events") &&
      !!detail.data &&
      !detail.error,
  });
  const validRelated = related.error ? undefined : related.data;
  const operation = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "result-context",
      reference.operation_id,
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.operation(
        clusterId,
        reference.operation_id!,
        signal,
      ),
    enabled: !!reference.operation_id,
    retry: false,
  });
  const recorded =
    !operation.error && operation.data
      ? operationResource(operation.data)
      : undefined;
  const recordedMatch =
    recorded?.uid === reference.uid &&
    recorded?.type === reference.type &&
    recorded?.name === reference.name &&
    recorded?.namespace_id === reference.namespace_id;
  const resultUnavailable =
    recordedMatch &&
    detail.error instanceof ApiError &&
    detail.error.code === "KUBERNETES_OBJECT_IDENTITY_CHANGED";
  const row = detail.error ? undefined : detail.data,
    title =
      (row?.definition.kind ?? resourceKindName(reference.type)) +
      " · " +
      reference.name;
  const resources = validRelated?.items ?? [];
  const content = (
    <div className="argus-k8s-resource-detail">
      {detail.isPending && <Spinner label={t("common.loading")} />}
      {detail.error && (
        <Alert
          description={
            resultUnavailable
              ? t("kubernetesResources.resultUnavailableHint")
              : ""
          }
          tone={resultUnavailable ? "warning" : "danger"}
          title={
            resultUnavailable
              ? t("kubernetesResources.resultUnavailable")
              : formatError(detail.error)
          }
        />
      )}
      {row && (
        <>
          <div className="argus-k8s-detail-state">
            <Badge tone={stateTone(row.summary?.state ?? "unknown")}>
              {t(
                "kubernetesResources.states." +
                  (row.summary?.state ?? "unknown"),
              )}
            </Badge>
            {row.summary?.reason && <span>{row.summary.reason}</span>}
            <span>{row.namespace}</span>
          </div>
          <Tabs
            value={tab}
            onValueChange={(value) => {
              setTab(value);
              setCursors([undefined]);
            }}
          >
            <TabsList ariaLabel={t("kubernetesResources.overview")}>
              {[
                "overview",
                ...(row.definition.key !== "event"
                  ? ["relations", "events", "configuration"]
                  : []),
                ...(supportsResourceMetrics(row.definition.key)
                  ? ["metrics"]
                  : []),
                "manifest",
                "technical",
              ].map((key) => (
                <TabsTrigger key={key} value={key}>
                  {t("kubernetesResources." + key)}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value="overview">
              <KubernetesHPANotice
                clusterId={clusterId}
                object={row}
                namespaceId={reference.namespace_id}
                onOpen={onOpen}
              />
              <KeyValueGrid
                columns={2}
                items={Object.entries(row.summary?.fields ?? {})
                  .filter(([key]) => key !== "involved_uid")
                  .map(([key, value]) => ({
                    label: t("kubernetesResources.fields." + key, {
                      defaultValue: key,
                    }),
                    value: formatResourceField(key, value),
                  }))}
              />
              <p className="argus-k8s-observation-time">
                {t("kubernetesResources.snapshot")} ·{" "}
                {new Date(row.observed_at).toLocaleString(i18n.language)}
              </p>
              <KubernetesResourceFacts object={row} />
            </TabsContent>
            {["relations", "events"].map((key) => (
              <TabsContent key={key} value={key}>
                {key === "relations" &&
                  visibleReferenceNames(row).length > 0 && (
                    <section>
                      <h3>{t("kubernetesResources.referenceNames")}</h3>
                      <ul>
                        {visibleReferenceNames(row).map((name) => (
                          <li key={name}>{name}</li>
                        ))}
                      </ul>
                    </section>
                  )}
                {related.isPending && <Spinner label={t("common.loading")} />}
                {related.error && (
                  <Alert
                    description=""
                    tone="danger"
                    title={formatError(related.error)}
                  />
                )}
                {validRelated && !validRelated.complete && (
                  <Alert
                    description=""
                    tone="warning"
                    title={t("kubernetesResources.partial")}
                  />
                )}
                {validRelated && resources.length > 0 && (
                  <DataTable
                    data={resources}
                    getRowKey={(item) => item.uid}
                    columns={[
                      {
                        key: "name",
                        header: t("kubernetesWorkbench.name"),
                        render: (item) => (
                          <Button variant="ghost" onPress={() => onOpen(item)}>
                            {item.name}
                          </Button>
                        ),
                      },
                      { key: "type", header: t("kubernetesWorkbench.type") },
                      {
                        key: "namespace",
                        header: t("kubernetesWorkbench.namespace"),
                      },
                      {
                        key: "state",
                        header: t("kubernetesResources.status"),
                        render: (item) => (
                          <Badge tone={stateTone(item.summary.state)}>
                            {t(
                              "kubernetesResources.states." +
                                item.summary.state,
                            )}
                          </Badge>
                        ),
                      },
                      ...(key === "events"
                        ? [
                            {
                              key: "message",
                              header: t("kubernetesResources.fields.message"),
                              render: (item: (typeof resources)[number]) =>
                                item.summary.fields.message ?? "—",
                            },
                          ]
                        : []),
                    ]}
                  />
                )}
                {validRelated && resources.length === 0 && (
                  <EmptyState
                    title={t(
                      validRelated.next_cursor
                        ? "kubernetesResources.continueRelations"
                        : "kubernetesResources.noRelated",
                    )}
                    description={t("kubernetesResources.authorizedRelations")}
                  />
                )}
                {validRelated &&
                  (cursors.length > 1 || validRelated.next_cursor) && (
                    <Pagination
                      page={cursors.length}
                      hasNext={!!validRelated.next_cursor}
                      isDisabled={related.isFetching}
                      onChange={(page) =>
                        setCursors((old) =>
                          page <= old.length
                            ? old.slice(0, page)
                            : [...old, validRelated.next_cursor],
                        )
                      }
                    />
                  )}
              </TabsContent>
            ))}
            <TabsContent value="metrics">
              <KubernetesResourceMetrics
                clusterId={clusterId}
                reference={reference}
              />
            </TabsContent>
            <TabsContent value="configuration">
              <KubernetesResourceConfiguration object={row} />
            </TabsContent>
            <TabsContent value="manifest">
              {row.redacted && <p>{t("kubernetesWorkbench.projected")}</p>}
              <CodeBlock
                code={formatStructuredDocument(row.data, "yaml")}
                language="yaml"
              />
            </TabsContent>
            <TabsContent value="technical">
              <KeyValueGrid
                columns={1}
                items={[
                  { label: "UID", value: row.uid },
                  { label: "resourceVersion", value: row.resource_version },
                  { label: "Namespace UID", value: row.namespace_uid ?? "—" },
                  { label: "generation", value: String(row.generation) },
                  {
                    label: t("kubernetesResources.age"),
                    value: new Date(row.created_at).toLocaleString(
                      i18n.language,
                    ),
                  },
                ]}
              />
            </TabsContent>
          </Tabs>
        </>
      )}
      {feedback}
    </div>
  );
  return (
    <DrawerPanel
      open
      title={title}
      expanded={full}
      onExpandedChange={() => onExpand()}
      footer={
        recordedMatch && reference.operation_id && onOperationDetails ? (
          <Button
            variant="ghost"
            onPress={() => onOperationDetails(reference.operation_id!)}
          >
            {t("kubernetesResources.originalOperation")}
          </Button>
        ) : undefined
      }
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {content}
    </DrawerPanel>
  );
}
