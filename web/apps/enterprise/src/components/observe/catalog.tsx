import { useMemo, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type ObservationScope,
  type ObservationMetric,
  type ObservationSignal,
} from "@argus/api-client";
import {
  Alert,
  Badge,
  Button,
  Checkbox,
  DataTable,
  DrawerPanel,
  EmptyState,
  FilterBar,
  Pagination,
  ObservationScopePicker,
  Select,
  Spinner,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  TelemetryFieldExplorer,
  type QueryResourceType,
} from "@argus/ui";
import {
  hasQueryResources,
  makeQuery,
  queryFromMetric,
  type ObserveQuery,
} from "./model";

export function ObserveCatalog({
  time,
  timeControl,
  actorKey,
  allowedKinds,
  onNew,
}: {
  time: { from: string; to: string };
  timeControl: ReactNode;
  actorKey: string;
  allowedKinds: QueryResourceType[];
  onNew: (q: ObserveQuery) => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const [signal, setSignal] = useState<ObservationSignal>("metrics"),
    [scope, setScope] = useState<ObservationScope>({
      mode: "explicit",
      resources: [],
    });
  const [kind, setKind] = useState(""),
    [search, setSearch] = useState(""),
    [observed, setObserved] = useState(true),
    [pagination, setPagination] = useState<{
      key: string;
      cursors: (string | undefined)[];
    }>({ key: "", cursors: [undefined] }),
    [selected, setSelected] = useState<ObservationMetric>();
  const reset = () => setPagination({ key: "", cursors: [undefined] });
  const effective = useMemo<ObservationScope>(
    () =>
      !kind
        ? scope
        : scope.mode === "explicit"
          ? {
              ...scope,
              resources: scope.resources.filter((r) => r.type === kind),
            }
          : { ...scope, resource_types: [kind as QueryResourceType] },
    [scope, kind],
  );
  const active = hasQueryResources(effective),
    key = JSON.stringify([actorKey, effective, signal, time, search, observed]);
  // Signed cursors belong to one exact scope, search and absolute time window.
  const cursors = pagination.key === key ? pagination.cursors : [undefined];
  const sourceKey = JSON.stringify([
    actorKey,
    effective.mode,
    effective.resources,
    effective.resource_types,
    signal,
    time,
  ]);
  const sourceQuery = useQuery({
    queryKey: ["observe-catalog-sources", sourceKey],
    enabled: active,
    gcTime: 0,
    queryFn: ({ signal: cancel }) =>
      api.telemetry.catalog(
        {
          scope: {
            ...effective,
            source_type: undefined,
            source_keys: undefined,
          },
          signal,
          kind: "sources",
          ...time,
          limit: 100,
        },
        cancel,
      ),
  });
  const query = useQuery({
    queryKey: ["observe-catalog", key, cursors.at(-1)],
    enabled: active,
    gcTime: 0,
    queryFn: ({ signal: cancel }) =>
      api.telemetry.catalog(
        {
          scope: effective,
          signal,
          kind: signal === "metrics" ? "metrics" : "fields",
          search,
          observed_only: observed,
          ...time,
          limit: 30,
          cursor: cursors.at(-1),
        },
        cancel,
      ),
  });
  const newSignalQuery = () => onNew(makeQuery(signal, effective));
  const chooseMetric = (
    m: ObservationMetric,
    expression?: string,
    unit?: string,
  ) => {
    const q = queryFromMetric(m, effective, expression);
    if (unit !== undefined) q.panel.unit = unit;
    setSelected(undefined);
    onNew(q);
  };
  return (
    <div className="argus-observe-catalog">
      <Tabs
        value={signal}
        onValueChange={(value) => {
          setSignal(value as ObservationSignal);
          setScope((old) => ({
            ...old,
            source_type: undefined,
            source_keys: undefined,
          }));
          setSearch("");
          reset();
        }}
      >
        <TabsList>
          {(["metrics", "logs", "traces"] as const).map((s) => (
            <TabsTrigger value={s} key={s}>
              {t(`telemetry.${s}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        {(["metrics", "logs", "traces"] as const).map((s) => (
          <TabsContent value={s} key={s}>
            {s === signal && (
              <>
                <FilterBar
                  search={{
                    value: search,
                    onChange: (v) => {
                      setSearch(v);
                      reset();
                    },
                    placeholder: t("observe.searchCatalog"),
                  }}
                  filters={[
                    {
                      key: "type",
                      value: kind,
                      onChange: (v) => {
                        setKind(v);
                        setScope((old) => ({
                          ...old,
                          source_type: undefined,
                          source_keys: undefined,
                        }));
                        reset();
                      },
                      allLabel: t("observe.allTypes"),
                      ariaLabel: t("observe.resourceType"),
                      options: allowedKinds.map((value) => ({
                        value,
                        label: t(`dashboardControls.resourceKinds.${value}`),
                      })),
                    },
                  ]}
                  onRefresh={() => {
                    if (active) {
                      void query.refetch();
                      void sourceQuery.refetch();
                    }
                  }}
                  refreshing={query.isFetching}
                  inlineActions={
                    <>
                      <ObservationScopePicker
                        value={scope}
                        contextKey={actorKey}
                        allowedKinds={allowedKinds}
                        load={api.telemetry.resources}
                        description={t("observe.catalogScopeHint")}
                        onChange={(selection) => {
                          setScope(selection);
                          reset();
                        }}
                      />
                      <Select
                        sizing="content"
                        ariaLabel={t("observe.source")}
                        value={scope.source_type ?? ""}
                        options={[
                          { value: "", label: t("observe.allSources") },
                          ...(sourceQuery.data?.sources ?? []).map((s) => ({
                            value: s.source_type,
                            label: s.source_type,
                          })),
                        ]}
                        onValueChange={(v) => {
                          setScope((old) => ({
                            ...old,
                            source_type: v || undefined,
                            source_keys: undefined,
                          }));
                          reset();
                        }}
                      />
                      {signal === "metrics" && (
                        <Checkbox
                          isSelected={observed}
                          onChange={(v) => {
                            setObserved(v);
                            reset();
                          }}
                        >
                          {t("observe.observedOnly")}
                        </Checkbox>
                      )}
                    </>
                  }
                  actions={timeControl}
                />
                {!active && (
                  <EmptyState
                    title={t(
                      hasQueryResources(scope)
                        ? "observe.resourceIntersectionEmpty"
                        : "observe.selectResources",
                    )}
                    description={t("observe.catalogScopeHint")}
                  />
                )}
                {query.isFetching && (
                  <Spinner label={t("observe.catalogLoading")} />
                )}
                {query.isError && (
                  <Alert
                    title={t("observe.catalogFailed")}
                    description={formatApiError(
                      query.error,
                      t("observe.failed"),
                      (id) => id,
                    )}
                    tone="danger"
                  />
                )}
                {active && query.data && (
                  <>
                    {!query.data.complete && (
                      <p className="argus-observe-hint">
                        {t("observe.catalogPartial")}
                      </p>
                    )}
                    {signal === "metrics" ? (
                      <DataTable<ObservationMetric>
                        getRowKey={(m) =>
                          JSON.stringify([
                            m.name,
                            m.source_type,
                            m.source_keys,
                            m.type,
                            m.unit,
                          ])
                        }
                        columns={[
                          {
                            key: "name",
                            header: t("observe.metric"),
                            render: (m) => (
                              <>
                                <Button
                                  variant="ghost"
                                  className="argus-observe-metric-name"
                                  onPress={() => setSelected(m)}
                                >
                                  {m.name}
                                </Button>
                                <small className="argus-observe-description">
                                  {m.description ||
                                    t("observe.unknownDescription")}
                                </small>
                              </>
                            ),
                          },
                          {
                            key: "type",
                            header: t("observe.metricType"),
                            render: (m) => (
                              <Badge>{m.type || t("observe.unknown")}</Badge>
                            ),
                          },
                          {
                            key: "unit",
                            header: t("observe.unit"),
                            render: (m) => m.unit || t("observe.unknown"),
                          },
                          {
                            key: "source_type",
                            header: t("observe.source"),
                            render: (m) => (
                              <span title={m.source_keys.join("\n")}>
                                {m.source_type}
                                <small className="argus-observe-description">
                                  {m.source_keys.length
                                    ? t("observe.sourceCount", {
                                        count: m.source_keys.length,
                                      })
                                    : ""}
                                </small>
                              </span>
                            ),
                          },
                          {
                            key: "observed",
                            header: t("observe.observation"),
                            render: (m) => (
                              <Badge tone={m.observed ? "success" : "warning"}>
                                {t(
                                  m.observed
                                    ? "observe.observed"
                                    : "observe.unobserved",
                                )}
                              </Badge>
                            ),
                          },
                          {
                            key: "actions",
                            header: t("observe.actions"),
                            render: (m) => (
                              <Button
                                variant="ghost"
                                onPress={() => chooseMetric(m)}
                              >
                                {t("observe.newQuery")}
                              </Button>
                            ),
                          },
                        ]}
                        data={query.data.metrics}
                      />
                    ) : (
                      <>
                        <DataTable
                          getRowKey={(f) => f.name}
                          columns={[
                            { key: "name", header: t("observe.field") },
                            { key: "type", header: t("observe.metricType") },
                          ]}
                          data={query.data.fields}
                        />
                        <Button onPress={newSignalQuery}>
                          {t("observe.newQuery")}
                        </Button>
                        <TelemetryFieldExplorer
                          key={key}
                          scope={t("observe.scopedFields")}
                          load={(request, cancel) =>
                            api.telemetry.catalog(
                              {
                                scope: effective,
                                signal,
                                ...time,
                                ...request,
                                limit: 30,
                              },
                              cancel,
                            )
                          }
                        />
                      </>
                    )}
                    {!query.data.metrics.length &&
                      !query.data.fields.length && (
                        <EmptyState
                          title={t(
                            query.data.has_more || cursors.length > 1
                              ? "observe.pageEmpty"
                              : "observe.noData",
                          )}
                          description={t(
                            query.data.has_more || cursors.length > 1
                              ? "observe.pageEmptyHint"
                              : "observe.noDataHint",
                          )}
                        />
                      )}
                    <Pagination
                      page={cursors.length}
                      hasPrevious={cursors.length > 1}
                      hasNext={query.data.has_more}
                      isDisabled={query.isFetching}
                      onPrevious={() =>
                        setPagination({ key, cursors: cursors.slice(0, -1) })
                      }
                      onNext={() =>
                        setPagination({
                          key,
                          cursors: [...cursors, query.data?.next_cursor],
                        })
                      }
                    />
                  </>
                )}
              </>
            )}
          </TabsContent>
        ))}
      </Tabs>
      {selected && (
        <DrawerPanel
          open
          title={selected.name}
          description={selected.description || t("observe.unknownDescription")}
          onOpenChange={(open) => {
            if (!open) setSelected(undefined);
          }}
        >
          <div className="argus-observe-detail">
            <p>
              {t("observe.source")}: {selected.source_type}
            </p>
            <p>
              {t("observe.metricType")}: {selected.type || t("observe.unknown")}{" "}
              · {t("observe.unit")}: {selected.unit || t("observe.unknown")}
            </p>
            <div className="argus-observe-actions">
              {selected.labels.map((label) => (
                <Badge key={label}>{label}</Badge>
              ))}
            </div>
            <h3>{t("observe.recipes")}</h3>
            {selected.recipes.length ? (
              selected.recipes.map((recipe) => (
                <section className="argus-observe-recipe" key={recipe.id}>
                  <strong>{recipe.title}</strong>
                  <p>{recipe.description}</p>
                  <pre>{recipe.expression}</pre>
                  <small>
                    {recipe.integration_key} · v{recipe.version} ·{" "}
                    {t("observe.requiredMetrics")}: {recipe.metrics.join(", ")}
                  </small>
                  <div className="argus-observe-actions">
                    <Button
                      onPress={() =>
                        void navigator.clipboard.writeText(recipe.expression)
                      }
                    >
                      {t("observe.copyExpression")}
                    </Button>
                    <Button
                      variant="primary"
                      onPress={() =>
                        chooseMetric(selected, recipe.expression, recipe.unit)
                      }
                    >
                      {t("observe.newQuery")}
                    </Button>
                  </div>
                </section>
              ))
            ) : (
              <EmptyState
                title={t("observe.noRecipe")}
                description={t("observe.noRecipeHint")}
              />
            )}
            <Button onPress={() => chooseMetric(selected)}>
              {t("observe.rawQuery")}
            </Button>
            <TelemetryFieldExplorer
              key={JSON.stringify([key, selected])}
              scope={t("observe.scopedFields")}
              load={(request, cancel) =>
                api.telemetry.catalog(
                  {
                    scope: {
                      ...effective,
                      source_type: selected.source_type,
                      ...(selected.source_keys.length
                        ? { source_keys: selected.source_keys }
                        : {}),
                    },
                    signal: "metrics",
                    metric: selected.name,
                    ...time,
                    ...request,
                    limit: 30,
                  },
                  cancel,
                )
              }
            />
          </div>
        </DrawerPanel>
      )}
    </div>
  );
}
