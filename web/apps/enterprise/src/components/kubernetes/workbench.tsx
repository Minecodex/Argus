import type { NamespaceChoice } from "./namespace-picker";
import { NamespaceSelect } from "./namespace-select";
import { useInventoryError } from "./inventory-error";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import {
  useApi,
  type KubernetesInventoryObject,
  type KubernetesInventoryScope,
} from "@argus/api-client";
import {
  Badge,
  Button,
  Pagination,
  CodeBlock,
  DataTable,
  Dialog,
  EmptyState,
  FilterBar,
  Input,
  SearchInput,
  KeyValueGrid,
  Spinner,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";
import "../../styles/kubernetes-workbench.css";
import { usePermission } from "../../lib/permissions";
import { KubernetesMutationDialog } from "./mutation-dialog";
import { KubernetesOperationsDialog } from "./operations-dialog";
import { KubernetesJobRunDialog } from "./job-run-dialog";
import { KubernetesLogSessionDialog } from "./log-session-dialog";
import { KubernetesStreamSessionDialog } from "./stream-session-dialog";

export function KubernetesWorkbench({
  clusterId,
  access,
}: {
  clusterId: string;
  access: KubernetesInventoryScope;
}) {
  const api = useApi(),
    cache = useQueryClient(),
    { t, i18n } = useTranslation(),
    formatError = useInventoryError();
  const [category, setCategory] = useState("workload"),
    [type, setType] = useState("deployment"),
    [namespace, setNamespace] = useState<NamespaceChoice>();
  const [jobSource, setJobSource] = useState<KubernetesInventoryObject>();
  const [logSource, setLogSource] = useState<KubernetesInventoryObject>();
  const [streamSource, setStreamSource] = useState<KubernetesInventoryObject>();
  const canExec = usePermission("kubernetes.exec");
  const canReadLogs = usePermission("kubernetes.operational_logs.read");
  const [name, setName] = useState(""),
    [labels, setLabels] = useState(""),
    [filters, setFilters] = useState({ name: "", labels: "" });
  const [cursors, setCursors] = useState<Array<string | undefined>>([
      undefined,
    ]),
    [selected, setSelected] = useState<KubernetesInventoryObject>();
  const types = useQuery({
    queryKey: ["kubernetes-workbench", clusterId, "types"],
    queryFn: ({ signal }) => api.kubernetesWorkbench.types(clusterId, signal),
  });
  const definitions = types.error ? [] : (types.data?.items ?? []);
  const categories = [...new Set(definitions.map((v) => v.category))];
  const visibleTypes = definitions.filter((v) => v.category === category);
  const definition =
    visibleTypes.find((v) => v.key === type) ?? visibleTypes[0];
  const canManage = usePermission(definition?.manage_permission ?? "");
  const [mutation, setMutation] = useState<
    "create" | KubernetesInventoryObject
  >();
  const [operationsOpen, setOperationsOpen] = useState(false);
  const chosenNamespace = definition?.namespaced ? namespace : undefined;
  const objectScopeKey = [
    "kubernetes-workbench",
    clusterId,
    "objects",
    definition?.key,
    chosenNamespace?.id,
  ];
  const queryKey = [
    ...objectScopeKey,
    filters,
    cursors.at(-1),
    access.cluster_version,
  ];
  const list = useQuery({
    queryKey,
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.list(
        clusterId,
        {
          type: definition!.key,
          namespace_mode: chosenNamespace ? "explicit" : "all_authorized",
          namespace_ids: chosenNamespace ? [chosenNamespace.id] : undefined,
          ...filters,
          limit: 30,
          cursor: cursors.at(-1),
        },
        signal,
      ),
    enabled: !!definition,
  });
  const data = list.error ? undefined : list.data;
  const reset = () => {
    setCursors([undefined]);
    setSelected(undefined);
  };
  const refreshFirstPage = (nextFilters = filters) => {
    setFilters(nextFilters);
    reset();
    void cache.invalidateQueries({
      queryKey: [
        ...objectScopeKey,
        nextFilters,
        undefined,
        access.cluster_version,
      ],
      exact: true,
    });
  };
  return (
    <div className="argus-k8s-workbench">
      {!access.cluster_wide && (
        <Badge>{t("kubernetesWorkbench.namespaceOnly")}</Badge>
      )}
      <Tabs
        value={category}
        onValueChange={(value) => {
          setCategory(value);
          setType("");
          reset();
        }}
      >
        <TabsList ariaLabel={t("kubernetesWorkbench.category")}>
          {categories.map((value) => (
            <TabsTrigger key={value} value={value}>
              {t(`kubernetesWorkbench.categories.${value}`, {
                defaultValue: value,
              })}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent
          key={category}
          value={category}
          className="argus-k8s-workbench-panel"
        >
          <Tabs
            value={definition?.key ?? ""}
            onValueChange={(value) => {
              setType(value);
              reset();
            }}
          >
            <TabsList ariaLabel={t("kubernetesWorkbench.type")}>
              {visibleTypes.map((value) => (
                <TabsTrigger key={value.key} value={value.key}>
                  {value.kind}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent
              key={definition?.key}
              value={definition?.key ?? ""}
              className="argus-k8s-workbench-panel"
            >
              <form
                aria-label={t("kubernetesWorkbench.filters")}
                onSubmit={(event) => {
                  event.preventDefault();
                  refreshFirstPage({ name, labels });
                }}
              >
                <FilterBar
                  className="argus-k8s-resource-filters"
                  inlineActions={
                    <>
                      {definition?.namespaced && (
                        <NamespaceSelect
                          clusterId={clusterId}
                          scopeVersion={access.cluster_version}
                          value={namespace}
                          onChange={(value) => {
                            setNamespace(value);
                            reset();
                          }}
                        />
                      )}
                      <SearchInput
                        className="argus-k8s-resource-name"
                        aria-label={t("kubernetesWorkbench.search")}
                        placeholder={t("kubernetesWorkbench.search")}
                        value={name}
                        onChange={(event) => setName(event.target.value)}
                      />
                      <Input
                        className="argus-k8s-resource-labels"
                        aria-label={t("kubernetesWorkbench.labels")}
                        placeholder={t("kubernetesWorkbench.labels")}
                        value={labels}
                        onChange={(event) => setLabels(event.target.value)}
                      />
                      <Button
                        type="submit"
                        variant="ghost"
                        isIconOnly
                        aria-label={t("kubernetesWorkbench.refresh")}
                        title={t("kubernetesWorkbench.refresh")}
                        isDisabled={list.isFetching}
                      >
                        <RefreshCw aria-hidden size={15} />
                      </Button>
                    </>
                  }
                  actions={
                    <>
                      <Button onPress={() => setOperationsOpen(true)}>
                        {t("kubernetesWorkbench.mutation.history")}
                      </Button>
                      {definition?.manage_permission && canManage && (
                        <Button
                          variant="primary"
                          isDisabled={definition.namespaced && !namespace}
                          title={
                            definition.namespaced && !namespace
                              ? t(
                                  "kubernetesWorkbench.mutation.chooseNamespace",
                                )
                              : undefined
                          }
                          onPress={() => setMutation("create")}
                        >
                          {t("kubernetesWorkbench.mutation.create")}
                        </Button>
                      )}
                    </>
                  }
                />
              </form>
              {(types.error || list.error) && (
                <p role="alert">{formatError(types.error || list.error)}</p>
              )}
              {(types.isPending || list.isFetching) && (
                <Spinner label={t("common.loading")} />
              )}
              {data &&
                (data.items.length ? (
                  <DataTable
                    data={data.items}
                    getRowKey={(row) => row.uid}
                    columns={[
                      { key: "name", header: t("kubernetesWorkbench.name") },
                      {
                        key: "namespace",
                        header: t("kubernetesWorkbench.namespace"),
                      },
                      {
                        key: "owners",
                        header: t("kubernetesWorkbench.owner"),
                        render: (row) =>
                          row.owners
                            .map((owner) => `${owner.kind}/${owner.name}`)
                            .join(", ") || "—",
                      },
                      {
                        key: "resource_version",
                        header: t("kubernetesWorkbench.version"),
                      },
                      {
                        key: "actions",
                        header: t("monitoring.actions"),
                        render: (row) => (
                          <div className="argus-k8s-workbench-controls">
                            <Button size="sm" onPress={() => setSelected(row)}>
                              {t("kubernetesWorkbench.details")}
                            </Button>
                            {canReadLogs &&
                              row.definition.key === "pod" &&
                              row.namespace_uid &&
                              data.namespace_references[row.namespace_uid] && (
                                <Button
                                  size="sm"
                                  onPress={() => setLogSource(row)}
                                >
                                  {t("kubernetesLogSession.action")}
                                </Button>
                              )}
                            {canManage && definition?.manage_permission && (
                              <Button
                                size="sm"
                                onPress={() => setMutation(row)}
                              >
                                {t("kubernetesWorkbench.mutation.manage")}
                              </Button>
                            )}
                            {canExec &&
                              row.definition.key === "pod" &&
                              row.namespace_uid &&
                              data.namespace_references[row.namespace_uid] && (
                                <Button
                                  size="sm"
                                  onPress={() => setStreamSource(row)}
                                >
                                  {t("kubernetesStream.exec")}
                                </Button>
                              )}
                            {canManage &&
                              row.definition.key === "cronjob" &&
                              row.namespace_uid &&
                              data?.namespace_references[row.namespace_uid] && (
                                <Button
                                  size="sm"
                                  onPress={() => setJobSource(row)}
                                >
                                  {t("kubernetesWorkbench.jobRun.action")}
                                </Button>
                              )}
                          </div>
                        ),
                      },
                    ]}
                  />
                ) : (
                  <EmptyState
                    title={t("kubernetesWorkbench.empty")}
                    description={t("kubernetesWorkbench.emptyHint")}
                  />
                ))}
              {data && (
                <div className="argus-k8s-workbench-footer">
                  <span>
                    {t("kubernetesWorkbench.observed")}{" "}
                    {new Date(data.observed_at).toLocaleString(i18n.language)} ·{" "}
                    {t(
                      data.complete
                        ? "kubernetesWorkbench.complete"
                        : "kubernetesWorkbench.more",
                    )}
                  </span>
                  <Pagination
                    page={cursors.length}
                    hasNext={!!data.next_cursor}
                    isDisabled={list.isFetching}
                    onChange={(page) => {
                      setSelected(undefined);
                      setCursors((old) =>
                        page <= old.length
                          ? old.slice(0, page)
                          : [...old, data.next_cursor],
                      );
                    }}
                  />
                </div>
              )}
              {selected && data && (
                <InventoryObjectDialog
                  key={selected.uid}
                  clusterId={clusterId}
                  object={selected}
                  namespaceId={
                    selected.namespace_uid
                      ? data.namespace_references[selected.namespace_uid]
                      : undefined
                  }
                  onClose={() => setSelected(undefined)}
                />
              )}
            </TabsContent>
          </Tabs>
        </TabsContent>
      </Tabs>
      {mutation && definition && (
        <KubernetesMutationDialog
          key={mutation === "create" ? "create" : mutation.uid}
          clusterId={clusterId}
          definition={definition}
          object={mutation === "create" ? undefined : mutation}
          namespace={
            mutation === "create"
              ? chosenNamespace
              : mutation.namespace_uid &&
                  data?.namespace_references[mutation.namespace_uid]
                ? {
                    id: data.namespace_references[mutation.namespace_uid]!,
                    name: mutation.namespace,
                  }
                : undefined
          }
          onClose={() => setMutation(undefined)}
          onSubmitted={() => {
            setOperationsOpen(true);
            void list.refetch();
          }}
        />
      )}
      {jobSource &&
        jobSource.namespace_uid &&
        data?.namespace_references[jobSource.namespace_uid] && (
          <KubernetesJobRunDialog
            clusterId={clusterId}
            source={jobSource}
            namespaceId={data.namespace_references[jobSource.namespace_uid]!}
            onClose={() => setJobSource(undefined)}
            onSubmitted={() => {
              setOperationsOpen(true);
              void list.refetch();
            }}
          />
        )}
      {operationsOpen && (
        <KubernetesOperationsDialog
          clusterId={clusterId}
          onClose={() => {
            setOperationsOpen(false);
            void list.refetch();
          }}
        />
      )}
      {logSource?.namespace_uid &&
        data?.namespace_references[logSource.namespace_uid] && (
          <KubernetesLogSessionDialog
            key={logSource.uid}
            clusterId={clusterId}
            object={logSource}
            namespaceId={data.namespace_references[logSource.namespace_uid]!}
            onClose={() => setLogSource(undefined)}
          />
        )}
      {streamSource?.namespace_uid &&
        data?.namespace_references[streamSource.namespace_uid] && (
          <KubernetesStreamSessionDialog
            key={streamSource.uid}
            clusterId={clusterId}
            object={streamSource}
            mode="exec"
            namespaceId={data.namespace_references[streamSource.namespace_uid]!}
            onClose={() => setStreamSource(undefined)}
          />
        )}
    </div>
  );
}

function InventoryObjectDialog({
  clusterId,
  object,
  namespaceId,
  onClose,
}: {
  clusterId: string;
  object: KubernetesInventoryObject;
  namespaceId?: string;
  onClose: () => void;
}) {
  const api = useApi(),
    { t } = useTranslation(),
    formatError = useInventoryError();
  const query = useQuery({
    queryKey: [
      "kubernetes-workbench",
      clusterId,
      "object",
      object.uid,
      namespaceId,
    ],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.object(
        clusterId,
        {
          type: object.definition.key,
          name: object.name,
          uid: object.uid,
          namespace_id: namespaceId,
        },
        signal,
      ),
  });
  const data = query.error ? undefined : query.data;
  return (
    <Dialog
      open
      size="lg"
      title={`${object.definition.kind} · ${object.name}`}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      {query.isPending && <Spinner label={t("common.loading")} />}
      {query.error && <p role="alert">{formatError(query.error)}</p>}
      {data && (
        <>
          <KeyValueGrid
            columns={2}
            items={[
              { label: t("kubernetesWorkbench.uid"), value: data.uid },
              {
                label: t("kubernetesWorkbench.version"),
                value: data.resource_version,
              },
              {
                label: t("kubernetesWorkbench.namespace"),
                value: data.namespace || "—",
              },
            ]}
          />
          {data.redacted && <p>{t("kubernetesWorkbench.projected")}</p>}
          <CodeBlock
            code={JSON.stringify(data.data, null, 2)}
            language="json"
          />
        </>
      )}
    </Dialog>
  );
}
