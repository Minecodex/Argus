import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type DataAuthorizationResource,
  type DataAuthorizationResourceType,
  type DataAuthorizationSubjectType,
} from "@argus/api-client";
import {
  Badge,
  Button,
  ConfirmDialog,
  DataTable,
  EmptyState,
  FormDrawer,
  Input,
  Spinner,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from "@argus/ui";
import "../../styles/data-authorization.css";

const resourceTypes: DataAuthorizationResourceType[] = [
  "host",
  "kubernetes_cluster",
  "dashboard",
  "application",
  "monitoring_target",
  "kubernetes_namespace",
];
type Change = {
  resourceType: DataAuthorizationResourceType;
  id: string;
  name: string;
  before: boolean;
  after: boolean;
};
const changeKey = (type: DataAuthorizationResourceType, id: string) =>
  `${type}:${id}`;

export function DataAuthorizationDialog({
  open,
  subjectLabel,
  subjectType,
  subjectId,
  onOpenChange,
}: {
  open: boolean;
  subjectLabel: string;
  subjectType: DataAuthorizationSubjectType;
  subjectId: string;
  onOpenChange: (open: boolean) => void;
}) {
  const api = useApi(),
    cache = useQueryClient(),
    { t } = useTranslation();
  const [type, setType] = useState<DataAuthorizationResourceType>("host"),
    [search, setSearch] = useState(""),
    [filter, setFilter] = useState(""),
    [cursors, setCursors] = useState<Array<string | undefined>>([undefined]);
  const [changes, setChanges] = useState<Record<string, Change>>({}),
    [draftVersion, setDraftVersion] = useState<number>(),
    [confirmOpen, setConfirmOpen] = useState(false);
  useEffect(() => {
    setChanges({});
    setDraftVersion(undefined);
    setSearch("");
    setFilter("");
    setCursors([undefined]);
    setType("host");
  }, [open, subjectType, subjectId]);
  const query = useQuery({
    queryKey: [
      "authorization",
      subjectType,
      subjectId,
      type,
      filter,
      cursors.at(-1),
    ],
    enabled: open && !!subjectId,
    queryFn: ({ signal }) =>
      api.org.listDataAuthorization(
        subjectType,
        subjectId,
        type,
        cursors.at(-1),
        30,
        filter,
        signal,
      ),
  });
  const rows = query.error ? [] : (query.data?.items ?? []),
    pending = Object.values(changes),
    stale =
      draftVersion !== undefined &&
      query.data !== undefined &&
      draftVersion !== query.data.authorization_version;
  const select = (item: DataAuthorizationResource, after: boolean) => {
    if (stale || save.isPending) return;
    setDraftVersion((version) => version ?? query.data?.authorization_version);
    setChanges((current) => {
      const next = { ...current },
        key = changeKey(type, item.resource_id),
        before = current[key]?.before ?? item.direct;
      if (after === before) delete next[key];
      else
        next[key] = {
          resourceType: type,
          id: item.resource_id,
          name: item.name,
          before,
          after,
        };
      return next;
    });
  };
  useEffect(() => {
    if (!pending.length) setDraftVersion(undefined);
  }, [pending.length]);
  const sourceLabel = (source: string) => {
    const index = source.indexOf(":"),
      kind = source.slice(0, index),
      name = source.slice(index + 1);
    if (source === "direct" || source === "直接")
      return t("authorizationCatalog.direct");
    if (index > 0 && ["role", "department", "cluster"].includes(kind))
      return t(`authorizationCatalog.sources.${kind}`, { name });
    return source;
  };
  const save = useMutation({
    mutationFn: async () => {
      if (stale || draftVersion === undefined || !pending.length) return;
      let version = draftVersion;
      for (const resourceType of resourceTypes)
        for (const remove of [false, true]) {
          const ids = pending
            .filter(
              (item) =>
                item.resourceType === resourceType && item.after === !remove,
            )
            .map((item) => item.id);
          for (let offset = 0; offset < ids.length; offset += 1000) {
            await api.org.updateDataAuthorization(
              subjectType,
              subjectId,
              resourceType,
              ids.slice(offset, offset + 1000),
              remove,
              version,
            );
            version = (
              await api.org.listDataAuthorization(
                subjectType,
                subjectId,
                resourceType,
                undefined,
                1,
              )
            ).authorization_version;
          }
        }
    },
    onSuccess: async () => {
      setChanges({});
      setDraftVersion(undefined);
      await cache.invalidateQueries({
        queryKey: ["authorization", subjectType, subjectId],
      });
      await cache.invalidateQueries({
        predicate: (query) =>
          ["monitoring", "kubernetes-workbench", "hosts"].includes(
            String(query.queryKey[0]),
          ) || String(query.queryKey[0]).startsWith("dashboard"),
      });
      onOpenChange(false);
    },
    onError: () => {
      void cache.invalidateQueries({
        queryKey: ["authorization", subjectType, subjectId],
      });
    },
  });
  const submit = () => {
    if (
      query.isFetching ||
      query.error ||
      stale ||
      !pending.length ||
      save.isPending
    )
      return;
    if (subjectType === "role") setConfirmOpen(true);
    else save.mutate();
  };
  const applySearch = () => {
    setFilter(search.trim());
    setCursors([undefined]);
  };
  const discard = () => {
    setChanges({});
    setDraftVersion(undefined);
    save.reset();
    setCursors([undefined]);
    void query.refetch();
  };
  return (
    <>
      <FormDrawer
        open={open}
        onOpenChange={onOpenChange}
        onSubmit={submit}
        loading={save.isPending}
        submitLabel={t("settings.org.dataAuthorization.save")}
        title={t("settings.org.dataAuthorization.title", {
          subject: subjectLabel,
        })}
      >
        <div className="argus-authorization-catalog">
          <Tabs
            value={type}
            onValueChange={(value) => {
              setType(value as DataAuthorizationResourceType);
              setSearch("");
              setFilter("");
              setCursors([undefined]);
            }}
          >
            <TabsList className="argus-authorization-tabs">
              {resourceTypes.map((value) => (
                <TabsTrigger key={value} value={value}>
                  {t(`authorizationCatalog.types.${value}`)}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent key={type} value={type}>
              <div className="argus-authorization-toolbar">
                <Input
                  aria-label={t("authorizationCatalog.search")}
                  placeholder={t("authorizationCatalog.search")}
                  value={search}
                  onChange={(event) => setSearch(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      event.preventDefault();
                      applySearch();
                    }
                  }}
                />
                <Button onPress={applySearch}>
                  {t("authorizationCatalog.searchButton")}
                </Button>
              </div>
              {query.isPending && <Spinner label={t("common.loading")} />}
              {query.error && (
                <p role="alert">
                  {formatApiError(
                    query.error,
                    t("settings.org.dataAuthorization.loadFailed"),
                    (id) => t("common.requestReference", { requestId: id }),
                  )}
                </p>
              )}
              {stale && (
                <p role="alert">
                  {t("authorizationCatalog.conflict")}{" "}
                  <Button onPress={discard}>
                    {t("authorizationCatalog.reload")}
                  </Button>
                </p>
              )}
              {rows.length ? (
                <DataTable
                  data={rows}
                  getRowKey={(row) => row.resource_id}
                  columns={[
                    {
                      key: "name",
                      header: t("authorizationCatalog.name"),
                      render: (row) => (
                        <div className="argus-authorization-identity">
                          <strong>{row.name}</strong>
                          {(row.parent_name || row.environment) && (
                            <small>
                              {[
                                row.parent_name,
                                row.environment
                                  ? t(`monitoring.env.${row.environment}`, {
                                      defaultValue: row.environment,
                                    })
                                  : undefined,
                              ]
                                .filter(Boolean)
                                .join(" · ")}
                            </small>
                          )}
                          {row.source_uid && <code>{row.source_uid}</code>}
                        </div>
                      ),
                    },
                    {
                      key: "sources",
                      header: t("authorizationCatalog.source"),
                      render: (row) => (
                        <div className="argus-authorization-identity">
                          {row.inherited && (
                            <Badge>{t("authorizationCatalog.inherited")}</Badge>
                          )}
                          {row.sources.map((source) => (
                            <small key={source}>{sourceLabel(source)}</small>
                          ))}
                        </div>
                      ),
                    },
                    {
                      key: "actions",
                      header: t("authorizationCatalog.direct"),
                      render: (row) => {
                        const selected =
                          changes[changeKey(type, row.resource_id)]?.after ??
                          row.direct;
                        return (
                          <Button
                            size="sm"
                            isDisabled={
                              stale ||
                              save.isPending ||
                              (row.inherited && !row.direct && !selected)
                            }
                            onPress={() => select(row, !selected)}
                          >
                            {t(
                              selected
                                ? "authorizationCatalog.revoke"
                                : "authorizationCatalog.grant",
                            )}
                          </Button>
                        );
                      },
                    },
                  ]}
                />
              ) : (
                !query.isPending &&
                !query.error && (
                  <EmptyState
                    title={t("authorizationCatalog.empty")}
                    description={t("authorizationCatalog.emptyHint")}
                  />
                )
              )}
              <div className="argus-authorization-toolbar">
                <Button
                  isDisabled={!rows.length || stale || save.isPending}
                  onPress={() =>
                    rows
                      .filter((row) => !row.inherited)
                      .forEach((row) => select(row, true))
                  }
                >
                  {t("authorizationCatalog.grantPage")}
                </Button>
                <Button
                  isDisabled={
                    !rows.some((row) => row.direct) || stale || save.isPending
                  }
                  onPress={() =>
                    rows
                      .filter((row) => row.direct)
                      .forEach((row) => select(row, false))
                  }
                >
                  {t("authorizationCatalog.revokePage")}
                </Button>
              </div>
              <div className="argus-authorization-toolbar">
                <span>
                  {t("authorizationCatalog.page", { page: cursors.length })}
                </span>
                <Button
                  isDisabled={cursors.length === 1 || query.isFetching}
                  onPress={() => setCursors((values) => values.slice(0, -1))}
                >
                  {t("settings.org.dataAuthorization.previousPage")}
                </Button>
                <Button
                  isDisabled={!query.data?.page.has_more || query.isFetching}
                  onPress={() =>
                    setCursors((values) => [
                      ...values,
                      query.data?.page.next_cursor ?? undefined,
                    ])
                  }
                >
                  {t("settings.org.dataAuthorization.nextPage")}
                </Button>
              </div>
            </TabsContent>
          </Tabs>
          <p role="status">
            {t("authorizationCatalog.pending", { count: pending.length })}
          </p>
          {save.error && (
            <p role="alert">
              {t("authorizationCatalog.saveFailed")}{" "}
              <Button onPress={discard}>
                {t("authorizationCatalog.reload")}
              </Button>
            </p>
          )}
        </div>
      </FormDrawer>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        loading={save.isPending}
        title={t("settings.org.dataAuthorization.confirmRoleTitle")}
        description={t(
          "settings.org.dataAuthorization.confirmRoleDescription",
          { count: query.data?.affected_member_count ?? 0 },
        )}
        onConfirm={() => {
          setConfirmOpen(false);
          save.mutate();
        }}
      />
    </>
  );
}
