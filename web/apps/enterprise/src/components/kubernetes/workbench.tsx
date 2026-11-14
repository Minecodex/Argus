import { useEnterpriseAuthStore } from "@argus/auth";
import { useInventoryPagination } from "./use-inventory-pagination";
import { ResourceActions } from "./resource-actions";
import { KubernetesResourceDetail } from "./resource-detail";
import { KubernetesOperationFeedback } from "./operation-feedback";
import {
  resourceColumns,
  formatResourceField,
  stateTone,
  readonlyKinds,
  referenceOf,
} from "./resource-model";
import { useResourceLocation } from "./resource-location";
import type { KubernetesResourceReference } from "@argus/api-client";
import { NamespacePicker, type NamespaceChoice } from "./namespace-picker";
import { NamespaceSelect } from "./namespace-select";
import { useInventoryError } from "./inventory-error";
import { supportsListMetrics, useListMetrics } from "./use-list-metrics";
import { ListMetricCell } from "./list-metric-cell";
import { ResourceNameSummary } from "./resource-name-summary";
import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { RefreshCw } from "lucide-react";
import {
  useApi,
  apiErrorPresentation,
  type KubernetesInventoryObject,
  type ConfirmActionResult,
  type KubernetesInventoryScope,
} from "@argus/api-client";
import {
  Badge,
  Select,
  Button,
  Pagination,
  DataTable,
  EmptyState,
  FilterBar,
  Input,
  SearchInput,
  Spinner,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@argus/ui";
import "../../styles/kubernetes-workbench.css";
import { usePermission } from "../../lib/permissions";
import {
  KubernetesMutationDialog,
  type KubernetesMutationEdit,
} from "./mutation-dialog";
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
  const location = useResourceLocation(clusterId);
  const listScroll = useRef<number | undefined>(undefined);
  const restoreList = useCallback(() => {
    const top = listScroll.current;
    if (top !== undefined)
      requestAnimationFrame(() =>
        document.querySelector("main")?.scrollTo({ top }),
      );
  }, []);
  const [submission, setSubmission] = useState<ConfirmActionResult>();
  const [initialEdit, setInitialEdit] = useState<KubernetesMutationEdit>();
  const [reviewInput, setReviewInput] = useState<() => void>();
  const [jobName, setJobName] = useState<string>();
  const [mutationNamespace, setMutationNamespace] = useState<NamespaceChoice>();
  const [mutationIntent, setMutationIntent] = useState<string>();
  const [sessionNamespaceId, setSessionNamespaceId] = useState<string>();
  const [category, setCategory] = useState("workload"),
    [type, setType] = useState(location.search.kind ?? "deployment"),
    [namespace, setNamespace] = useState<NamespaceChoice | undefined>(
      location.search.namespace_id
        ? {
            id: location.search.namespace_id,
            name: location.search.namespace_name ?? "",
          }
        : undefined,
    );
  const [jobSource, setJobSource] = useState<KubernetesInventoryObject>();
  const [logSource, setLogSource] = useState<KubernetesInventoryObject>();
  const [streamSource, setStreamSource] = useState<KubernetesInventoryObject>();
  const [name, setName] = useState(location.search.name_filter ?? ""),
    [labels, setLabels] = useState(location.search.label_filter ?? ""),
    [filters, setFilters] = useState({
      name: location.search.name_filter ?? "",
      labels: location.search.label_filter ?? "",
    });
  const [selected, setSelected] = useState<KubernetesResourceReference>();
  const types = useQuery({
    queryKey: ["kubernetes-workbench", clusterId, "types"],
    queryFn: ({ signal }) => api.kubernetesWorkbench.types(clusterId, signal),
  });
  const definitions = useMemo(
    () => (types.error ? [] : (types.data?.items ?? [])),
    [types.error, types.data?.items],
  );
  const categories = [...new Set(definitions.map((v) => v.category))];
  const visibleTypes = definitions.filter((v) => v.category === category);
  const definition = type
    ? definitions.find((v) => v.key === type)
    : visibleTypes[0];
  const canManage = usePermission(definition?.manage_permission ?? "");
  const [mutation, setMutation] = useState<
    "create" | KubernetesInventoryObject
  >();
  const [operationsOpen, setOperationsOpen] = useState(false);
  const [namespacePickerOpen, setNamespacePickerOpen] = useState(false);
  const [operationId, setOperationId] = useState<string>();
  const showOperation = (id: string) => {
    setOperationId(id);
    setOperationsOpen(true);
  };
  const chosenNamespace = definition?.namespaced ? namespace : undefined;
  const objectScopeKey = [
    "kubernetes-workbench",
    clusterId,
    "objects",
    definition?.key,
    chosenNamespace?.id,
  ];
  const actorId = useEnterpriseAuthStore((state) => state.session?.user.id);
  const pagination = useInventoryPagination({
    scopeKey: JSON.stringify([
      actorId,
      objectScopeKey,
      filters,
      location.search.state_filter,
      access,
    ]),
    location: location.search,
    update: location.update,
    fetchPage: (cursor, signal) =>
      api.kubernetesWorkbench.list(
        clusterId,
        {
          type: definition!.key,
          namespace_mode: chosenNamespace ? "explicit" : "all_authorized",
          namespace_ids: chosenNamespace ? [chosenNamespace.id] : undefined,
          ...filters,
          state: location.search.state_filter,
          limit: 30,
          cursor,
        },
        signal,
      ),
  });
  const queryKey = [
    ...objectScopeKey,
    filters,
    location.search.state_filter,
    pagination.cursor,
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
          state: location.search.state_filter,
          limit: 30,
          cursor: pagination.cursor,
        },
        signal,
      ),
    enabled: !!definition,
  });
  const data = list.error ? undefined : list.data;
  const metricGroups = useListMetrics({
    clusterId,
    input: {
      type: definition?.key ?? "",
      namespace_mode: chosenNamespace ? "explicit" : "all_authorized",
      namespace_ids: chosenNamespace ? [chosenNamespace.id] : undefined,
      ...filters,
      state: location.search.state_filter,
      limit: 30,
      cursor: pagination.cursor,
    },
    uids: data?.items.map((row) => row.uid) ?? [],
    updatedAt: list.dataUpdatedAt,
    fetching: list.isFetching || !!list.error,
    scopeKey: JSON.stringify(access),
  });
  const metricColumns: ("cpu" | "memory" | "storage")[] = supportsListMetrics(
    definition?.key ?? "",
  )
    ? definition?.key === "pvc"
      ? ["storage"]
      : ["cpu", "memory", "storage"]
    : [];
  const namespaceReadDenied =
    !types.error &&
    definition?.namespaced &&
    !chosenNamespace &&
    apiErrorPresentation(list.error)?.code === "KUBERNETES_API_FORBIDDEN";
  const namespaceIdFor = (row: KubernetesInventoryObject) =>
    row.namespace_uid
      ? (data?.namespace_references[row.namespace_uid] ??
        (selected?.uid === row.uid ? selected.namespace_id : undefined))
      : undefined;
  const openResource = (ref: KubernetesResourceReference) => {
    if (!selected)
      listScroll.current = document.querySelector("main")?.scrollTop ?? 0;
    setSelected(ref);
    location.update(
      {
        resource_type: ref.type,
        resource_name: ref.name,
        resource_uid: ref.uid,
        resource_namespace: ref.namespace,
        resource_namespace_id: ref.namespace_id,
        resource_operation: ref.operation_id,
        full: false,
      },
      false,
    );
  };
  const closeResource = () => {
    restoreList();
    setSelected(undefined);
    location.update({
      resource_type: undefined,
      resource_name: undefined,
      resource_uid: undefined,
      resource_namespace: undefined,
      resource_namespace_id: undefined,
      resource_operation: undefined,
      full: false,
    });
  };
  const onAction = (intent: string, row: KubernetesInventoryObject) => {
    setInitialEdit(undefined);
    setJobName(undefined);
    setMutationNamespace(
      namespaceIdFor(row)
        ? { id: namespaceIdFor(row)!, name: row.namespace }
        : undefined,
    );
    setSessionNamespaceId(namespaceIdFor(row));
    if (intent === "logs") {
      setLogSource(row);
      return;
    }
    if (intent === "exec") {
      setStreamSource(row);
      return;
    }
    if (intent === "run") {
      setJobSource(row);
      return;
    }
    setMutationIntent(intent);
    setMutation(row);
  };
  const settled = useCallback(() => {
    void cache.invalidateQueries({
      queryKey: ["kubernetes-workbench", clusterId],
    });
  }, [cache, clusterId]);
  useEffect(() => {
    const q = location.search;
    if (q.resource_uid && q.resource_type && q.resource_name)
      setSelected({
        uid: q.resource_uid,
        type: q.resource_type,
        name: q.resource_name,
        namespace: q.resource_namespace ?? "",
        namespace_id: q.resource_namespace_id,
        operation_id: q.resource_operation,
      });
    else {
      setSelected(undefined);
      restoreList();
    }
  }, [location.search, restoreList]);
  useEffect(() => {
    if (location.search.kind) {
      setType(location.search.kind);
      const found = definitions.find((row) => row.key === location.search.kind);
      if (found) setCategory(found.category);
    }
  }, [location.search.kind, definitions]);
  useEffect(() => {
    const q = location.search;
    setName(q.name_filter ?? "");
    setLabels(q.label_filter ?? "");
    setFilters({ name: q.name_filter ?? "", labels: q.label_filter ?? "" });
    setNamespace(
      q.namespace_id
        ? { id: q.namespace_id, name: q.namespace_name ?? "" }
        : undefined,
    );
  }, [location.search]);
  const reset = () => {
    pagination.cancel();
    setSelected(undefined);
  };
  const selectNamespace = (value: NamespaceChoice | undefined) => {
    setNamespacePickerOpen(false);
    setNamespace(value);
    location.update({
      namespace_id: value?.id,
      namespace_name: value?.name,
      page_cursor: undefined,
      page_index: undefined,
      resource_uid: undefined,
    });
    reset();
  };
  const refreshFirstPage = (nextFilters = filters) => {
    setFilters(nextFilters);
    location.update({
      name_filter: nextFilters.name || undefined,
      label_filter: nextFilters.labels || undefined,
      page_cursor: undefined,
      page_index: undefined,
      resource_uid: undefined,
    });
    reset();
    void cache.invalidateQueries({
      queryKey: [
        ...objectScopeKey,
        nextFilters,
        location.search.state_filter,
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
      {submission && !selected && (
        <KubernetesOperationFeedback
          key={submission.pending_action.action_ref}
          clusterId={clusterId}
          submission={submission}
          onReview={reviewInput}
          onSettled={settled}
          onDetails={showOperation}
          onOpen={openResource}
        />
      )}
      {(pagination.unknown ||
        pagination.busy ||
        pagination.recovery ||
        !!pagination.error) && (
        <section role="status">
          <p>
            {t(
              pagination.unknown
                ? "kubernetesResources.pageUnknown"
                : pagination.error
                  ? "kubernetesResources.pageRestoreFailed"
                  : "kubernetesResources.pageRestoring",
              { page: pagination.recovery?.reached ?? pagination.page },
            )}
          </p>
          {!!pagination.error && (
            <Button onPress={() => refreshFirstPage()}>
              {t("kubernetesResources.firstPage")}
            </Button>
          )}
          {pagination.recovery && (
            <Button onPress={pagination.continueRecovery}>
              {t("kubernetesResources.continueRestore")}
            </Button>
          )}
          {(pagination.busy || pagination.recovery) && (
            <Button onPress={pagination.cancel}>{t("common.cancel")}</Button>
          )}
        </section>
      )}
      <div>
        <Tabs
          value={category}
          onValueChange={(value) => {
            setCategory(value);
            setType("");
            location.update({
              kind: definitions.find((row) => row.category === value)?.key,
              page_cursor: undefined,
              page_index: undefined,
              resource_uid: undefined,
            });
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
                location.update({
                  kind: value,
                  page_cursor: undefined,
                  page_index: undefined,
                  resource_uid: undefined,
                });
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
                            onChange={selectNamespace}
                          />
                        )}
                        <Select
                          ariaLabel={t("kubernetesResources.status")}
                          sizing="content"
                          className="argus-k8s-status-select"
                          value={location.search.state_filter ?? ""}
                          onValueChange={(state) => {
                            location.update({
                              state_filter: state || undefined,
                              page_cursor: undefined,
                              page_index: undefined,
                              resource_uid: undefined,
                            });
                            reset();
                          }}
                          options={[
                            "",
                            "healthy",
                            "configured",
                            "progressing",
                            "pending",
                            "warning",
                            "failed",
                            "completed",
                            "paused",
                            "scaled_down",
                            "terminating",
                            "unknown",
                            "attention",
                          ].map((value) => ({
                            value,
                            label: value
                              ? t("kubernetesResources.states." + value)
                              : t("common.all"),
                          }))}
                        />
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
                        {definition?.manage_permission &&
                          canManage &&
                          !readonlyKinds.has(definition.key) && (
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
                              onPress={() => {
                                setMutationIntent("configure");
                                setMutationNamespace(chosenNamespace);
                                setMutation("create");
                              }}
                            >
                              {t("kubernetesWorkbench.mutation.create")}
                            </Button>
                          )}
                      </>
                    }
                  />
                </form>
                {(types.error || list.error) && (
                  <div role="alert" className="argus-k8s-inventory-error">
                    <p>{formatError(types.error || list.error)}</p>
                    {namespaceReadDenied && (
                      <>
                        <p>
                          {t("kubernetesWorkbench.namespaceReadDeniedHint")}
                        </p>
                        <Button onPress={() => setNamespacePickerOpen(true)}>
                          {t("kubernetesWorkbench.chooseNamespace")}
                        </Button>
                      </>
                    )}
                  </div>
                )}
                {(types.isPending || list.isFetching) && (
                  <Spinner label={t("common.loading")} />
                )}
                {data &&
                  (data.items.length ? (
                    <DataTable
                      className="argus-k8s-inventory-wrap"
                      tableClassName="argus-k8s-inventory-table"
                      data={data.items}
                      getRowKey={(row) => row.uid}
                      columns={[
                        {
                          key: "name",
                          header: t("kubernetesWorkbench.name"),
                          render: (row) => (
                            <ResourceNameSummary
                              object={row}
                              isSummaryDisabled={
                                !!selected ||
                                !!mutation ||
                                !!jobSource ||
                                !!logSource ||
                                !!streamSource ||
                                operationsOpen ||
                                namespacePickerOpen
                              }
                              onPress={() =>
                                openResource(
                                  referenceOf(row, namespaceIdFor(row)),
                                )
                              }
                            />
                          ),
                        },
                        ...(definition?.namespaced
                          ? [
                              {
                                key: "namespace",
                                header: t("kubernetesWorkbench.namespace"),
                                render: (row: KubernetesInventoryObject) => (
                                  <span
                                    className="argus-k8s-resource-value"
                                    title={row.namespace}
                                  >
                                    {row.namespace}
                                  </span>
                                ),
                              },
                            ]
                          : []),
                        {
                          key: "status",
                          header: t("kubernetesResources.status"),
                          render: (row) => (
                            <div>
                              <Badge
                                tone={stateTone(
                                  row.summary?.state ?? "unknown",
                                )}
                              >
                                {t(
                                  "kubernetesResources.states." +
                                    (row.summary?.state ?? "unknown"),
                                )}
                              </Badge>
                              {row.summary?.reason && (
                                <small>{row.summary.reason}</small>
                              )}
                            </div>
                          ),
                        },
                        ...(
                          resourceColumns[definition?.key ?? ""] ?? ["phase"]
                        ).map((key) => ({
                          key,
                          header: t("kubernetesResources.fields." + key, {
                            defaultValue: key,
                          }),
                          render: (row: KubernetesInventoryObject) => (
                            <span
                              className="argus-k8s-resource-value"
                              title={row.summary?.fields[key]}
                            >
                              {row.summary?.fields[key] !== undefined
                                ? formatResourceField(
                                    key,
                                    row.summary.fields[key]!,
                                  )
                                : "—"}
                            </span>
                          ),
                        })),
                        ...metricColumns.map((metric) => ({
                          key: `metric_${metric}`,
                          header: t("kubernetesTrends." + metric),
                          render: (row: KubernetesInventoryObject) => (
                            <ListMetricCell
                              uid={row.uid}
                              metric={metric}
                              group={metricGroups[metric]}
                            />
                          ),
                        })),
                        ...(metricColumns.length
                          ? []
                          : [
                              {
                                key: "created",
                                header: t("kubernetesResources.age"),
                                render: (row: KubernetesInventoryObject) =>
                                  new Date(row.created_at).toLocaleString(
                                    i18n.language,
                                  ),
                              },
                            ]),
                        {
                          key: "actions",
                          header: t("monitoring.actions"),
                          render: (row) => (
                            <ResourceActions
                              object={row}
                              onAction={onAction}
                              onCopyLink={() => {
                                const reference = referenceOf(
                                  row,
                                  namespaceIdFor(row),
                                );
                                const params = new URLSearchParams();
                                const search = {
                                  ...location.search,
                                  resource_type: reference.type,
                                  resource_name: reference.name,
                                  resource_uid: reference.uid,
                                  resource_namespace: reference.namespace,
                                  resource_namespace_id: reference.namespace_id,
                                  resource_operation: undefined,
                                  full: undefined,
                                };
                                for (const [key, value] of Object.entries(
                                  search,
                                ))
                                  if (value !== undefined)
                                    params.set(key, String(value));
                                void navigator.clipboard.writeText(
                                  window.location.origin +
                                    "/kubernetes/" +
                                    encodeURIComponent(clusterId) +
                                    "?" +
                                    params,
                                );
                              }}
                            />
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
                      {new Date(data.observed_at).toLocaleString(i18n.language)}{" "}
                      ·{" "}
                      {t(
                        data.complete
                          ? "kubernetesWorkbench.complete"
                          : "kubernetesWorkbench.more",
                      )}
                    </span>
                    <Pagination
                      page={pagination.page}
                      summary={
                        pagination.unknown
                          ? t("kubernetesResources.pageUnknown")
                          : undefined
                      }
                      hasPrevious={!pagination.unknown && pagination.page > 1}
                      hasNext={!!data.next_cursor}
                      isDisabled={list.isFetching || pagination.busy}
                      onChange={(page) => {
                        setSelected(undefined);
                        pagination.go(page, data.next_cursor);
                      }}
                    />
                  </div>
                )}
              </TabsContent>
            </Tabs>
          </TabsContent>
        </Tabs>
      </div>
      {selected && (
        <KubernetesResourceDetail
          key={selected.uid}
          clusterId={clusterId}
          reference={selected}
          onOperationDetails={showOperation}
          feedback={
            submission ? (
              <KubernetesOperationFeedback
                key={submission.pending_action.action_ref}
                clusterId={clusterId}
                submission={submission}
                onSettled={settled}
                onDetails={showOperation}
                onOpen={openResource}
                onReview={reviewInput}
              />
            ) : undefined
          }
          full={location.search.full}
          onClose={closeResource}
          onExpand={() => {
            listScroll.current ??=
              document.querySelector("main")?.scrollTop ?? 0;
            location.update({ full: !location.search.full });
          }}
          onOpen={openResource}
        />
      )}
      {namespacePickerOpen && namespaceReadDenied && (
        <NamespacePicker
          clusterId={clusterId}
          onSelect={selectNamespace}
          onClose={() => setNamespacePickerOpen(false)}
        />
      )}
      {mutation && definition && (
        <KubernetesMutationDialog
          key={
            (mutation === "create" ? "create" : mutation.uid) +
            ":" +
            mutationIntent
          }
          clusterId={clusterId}
          definition={mutation === "create" ? definition : mutation.definition}
          intent={mutationIntent}
          initialEdit={initialEdit}
          object={mutation === "create" ? undefined : mutation}
          namespace={mutationNamespace}
          onClose={() => setMutation(undefined)}
          onSubmitted={(result, edit) => {
            setSubmission(result);
            setReviewInput(() => () => {
              setInitialEdit(edit);
              setMutationIntent(mutationIntent);
              setMutationNamespace(mutationNamespace);
              setMutation(mutation);
            });
            settled();
          }}
        />
      )}
      {jobSource && jobSource.namespace_uid && sessionNamespaceId && (
        <KubernetesJobRunDialog
          clusterId={clusterId}
          source={jobSource}
          initialName={jobName}
          namespaceId={sessionNamespaceId!}
          onClose={() => setJobSource(undefined)}
          onSubmitted={(result, name) => {
            setSubmission(result);
            setReviewInput(() => () => {
              setJobName(name);
              setJobSource(jobSource);
              setSessionNamespaceId(sessionNamespaceId);
            });
            settled();
          }}
        />
      )}
      {operationsOpen && (
        <KubernetesOperationsDialog
          clusterId={clusterId}
          operationId={operationId}
          onOpen={(ref) => {
            setOperationsOpen(false);
            setOperationId(undefined);
            openResource(ref);
          }}
          onClose={() => {
            setOperationsOpen(false);
            setOperationId(undefined);
            void list.refetch();
          }}
        />
      )}
      {logSource?.namespace_uid && sessionNamespaceId && (
        <KubernetesLogSessionDialog
          key={logSource.uid}
          clusterId={clusterId}
          object={logSource}
          namespaceId={sessionNamespaceId!}
          onClose={() => setLogSource(undefined)}
        />
      )}
      {streamSource?.namespace_uid && sessionNamespaceId && (
        <KubernetesStreamSessionDialog
          key={streamSource.uid}
          clusterId={clusterId}
          object={streamSource}
          mode="exec"
          namespaceId={sessionNamespaceId!}
          onClose={() => setStreamSource(undefined)}
        />
      )}
    </div>
  );
}
