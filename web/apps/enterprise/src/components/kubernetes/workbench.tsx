import { NamespacePicker, type NamespaceChoice } from "./namespace-picker";
import { useInventoryError } from "./inventory-error";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type KubernetesInventoryObject,
  type KubernetesInventoryScope,
} from "@argus/api-client";
import {
  Badge,
  Button,
  CodeBlock,
  DataTable,
  Dialog,
  EmptyState,
  Field,
  Input,
  KeyValueGrid,
  Select,
  Spinner,
  Switch,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";
import "../../styles/kubernetes-workbench.css";
import { useInventoryWatch } from "./inventory-watch";
import { usePermission } from "../../lib/permissions";
import { KubernetesMutationDialog } from "./mutation-dialog";
import { KubernetesOperationsDialog } from "./operations-dialog";
import { KubernetesJobRunDialog } from "./job-run-dialog";
import { KubernetesLogSessionDialog } from "./log-session-dialog";
import { KubernetesStreamSessionDialog } from "./stream-session-dialog";
import { KubernetesHelmDialog } from "./helm-dialog";

export function KubernetesWorkbench({
  clusterId,
  access,
}: {
  clusterId: string;
  access: KubernetesInventoryScope;
}) {
  const api = useApi(),
    { t, i18n } = useTranslation(),
    formatError = useInventoryError();
  const [category, setCategory] = useState("workload"),
    [type, setType] = useState("deployment"),
    [namespace, setNamespace] = useState<NamespaceChoice>(),
    [picker, setPicker] = useState(false);
  const [live, setLive] = useState(true);
  const [helmOpen, setHelmOpen] = useState(false);
  const [jobSource, setJobSource] = useState<KubernetesInventoryObject>();
  const [logSource, setLogSource] = useState<KubernetesInventoryObject>();
  const [streamSource, setStreamSource] = useState<{
    object: KubernetesInventoryObject;
    mode: "exec" | "port_forward";
  }>();
  const canExec = usePermission("kubernetes.exec"),
    canForward = usePermission("kubernetes.port_forward");
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
  const queryKey = [
    "kubernetes-workbench",
    clusterId,
    "objects",
    definition?.key,
    chosenNamespace?.id,
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
  const watch = useInventoryWatch(clusterId, queryKey, data, live, () => {
    reset();
    void list.refetch();
  });
  return (
    <div className="argus-k8s-workbench">
      {!access.cluster_wide && (
        <Badge>{t("kubernetesWorkbench.namespaceOnly")}</Badge>
      )}
      {types.data?.complete === false && (
        <Badge>{t("kubernetesWorkbench.partialTypes")}</Badge>
      )}
      <Tabs
        value={category}
        onValueChange={(value) => {
          setCategory(value);
          setType("");
          reset();
        }}
      >
        <TabsList>
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
          <div className="argus-k8s-workbench-toolbar">
            <Button onPress={() => setOperationsOpen(true)}>
              {t("kubernetesWorkbench.mutation.history")}
            </Button>
            <Button onPress={() => setHelmOpen(true)}>
              {t("kubernetesHelm.title")}
            </Button>
            <Field label={t("kubernetesWorkbench.type")} requirement="required">
              <Select
                className="argus-k8s-type-select"
                value={definition?.key ?? ""}
                options={visibleTypes.map((value) => ({
                  value: value.key,
                  label: value.kind,
                }))}
                onValueChange={(value) => {
                  setType(value);
                  reset();
                }}
              />
            </Field>
            {definition?.namespaced && (
              <Field
                label={t("kubernetesWorkbench.scope")}
                requirement="optional"
              >
                <div className="argus-k8s-workbench-controls">
                  <Button onPress={() => setPicker(true)}>
                    {namespace?.name ?? t("kubernetesWorkbench.allAuthorized")}
                  </Button>
                  {namespace && (
                    <Button
                      onPress={() => {
                        setNamespace(undefined);
                        reset();
                      }}
                    >
                      {t("kubernetesWorkbench.allAuthorized")}
                    </Button>
                  )}
                </div>
              </Field>
            )}
            <Button
              onPress={() => {
                reset();
                void list.refetch();
              }}
            >
              {t("kubernetesWorkbench.refresh")}
            </Button>
            <Switch
              label={t("kubernetesWorkbench.watch.currentPage")}
              checked={live && !!data?.watch_token}
              disabled={!data?.watch_token}
              onChange={setLive}
            />
            <span role="status">
              {t(`kubernetesWorkbench.watch.${watch.status}`)}
            </span>
            {watch.changes && (
              <Badge>{t("kubernetesWorkbench.watch.changes")}</Badge>
            )}
          </div>
          <form
            className="argus-k8s-workbench-toolbar"
            onSubmit={(event) => {
              event.preventDefault();
              setFilters({ name, labels });
              reset();
            }}
          >
            <Input
              aria-label={t("kubernetesWorkbench.search")}
              placeholder={t("kubernetesWorkbench.search")}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
            <Input
              aria-label={t("kubernetesWorkbench.labels")}
              placeholder={t("kubernetesWorkbench.labels")}
              value={labels}
              onChange={(event) => setLabels(event.target.value)}
            />
            <Button type="submit">{t("kubernetesWorkbench.refresh")}</Button>
          </form>
          {definition?.manage_permission && canManage && (
            <div className="argus-k8s-workbench-controls">
              <Button
                variant="primary"
                isDisabled={definition.namespaced && !namespace}
                onPress={() => setMutation("create")}
              >
                {t("kubernetesWorkbench.mutation.create")}
              </Button>
              {definition.namespaced && !namespace && (
                <span>{t("kubernetesWorkbench.mutation.chooseNamespace")}</span>
              )}
            </div>
          )}
          {(types.error || list.error || watch.status === "failed") && (
            <p role="alert">
              {formatError(types.error || list.error || watch.error)}
            </p>
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
                            <Button size="sm" onPress={() => setLogSource(row)}>
                              {t("kubernetesLogSession.action")}
                            </Button>
                          )}
                        {canManage && definition?.manage_permission && (
                          <Button size="sm" onPress={() => setMutation(row)}>
                            {t("kubernetesWorkbench.mutation.manage")}
                          </Button>
                        )}
                        {canExec &&
                          row.definition.key === "pod" &&
                          row.namespace_uid &&
                          data.namespace_references[row.namespace_uid] && (
                            <Button
                              size="sm"
                              onPress={() =>
                                setStreamSource({ object: row, mode: "exec" })
                              }
                            >
                              {t("kubernetesStream.exec")}
                            </Button>
                          )}
                        {canForward &&
                          ["pod", "service"].includes(row.definition.key) &&
                          row.namespace_uid &&
                          data.namespace_references[row.namespace_uid] && (
                            <Button
                              size="sm"
                              onPress={() =>
                                setStreamSource({
                                  object: row,
                                  mode: "port_forward",
                                })
                              }
                            >
                              {t("kubernetesStream.forward")}
                            </Button>
                          )}
                        {canManage &&
                          row.definition.key === "cronjob" &&
                          row.namespace_uid &&
                          data?.namespace_references[row.namespace_uid] && (
                            <Button size="sm" onPress={() => setJobSource(row)}>
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
              <div className="argus-k8s-workbench-controls">
                <Button
                  isDisabled={cursors.length === 1}
                  onPress={() => {
                    setSelected(undefined);
                    setCursors((v) => v.slice(0, -1));
                  }}
                >
                  {t("monitoring.previous")}
                </Button>
                <Button
                  isDisabled={!data.next_cursor}
                  onPress={() => {
                    setSelected(undefined);
                    setCursors((v) => [...v, data.next_cursor]);
                  }}
                >
                  {t("monitoring.next")}
                </Button>
              </div>
            </div>
          )}
          {picker && (
            <NamespacePicker
              clusterId={clusterId}
              onSelect={(choice) => {
                setNamespace(choice);
                reset();
              }}
              onClose={() => setPicker(false)}
            />
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
      {helmOpen && (
        <KubernetesHelmDialog
          clusterId={clusterId}
          onClose={() => setHelmOpen(false)}
          onSubmitted={() => {
            setHelmOpen(false);
            setOperationsOpen(true);
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
      {streamSource?.object.namespace_uid &&
        data?.namespace_references[streamSource.object.namespace_uid] && (
          <KubernetesStreamSessionDialog
            key={`${streamSource.object.uid}:${streamSource.mode}`}
            clusterId={clusterId}
            object={streamSource.object}
            mode={streamSource.mode}
            namespaceId={
              data.namespace_references[streamSource.object.namespace_uid]!
            }
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
