import { useMemo, useState } from "react";
import { useSearch } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Plus, Play } from "lucide-react";
import { useEnterpriseAuthStore } from "@argus/auth";
import type {
  ObservationResourceRef,
  ObservationSignal,
} from "@argus/api-client";
import {
  Button,
  Card,
  CardContent,
  Checkbox,
  EmptyState,
  PageShell,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  type QueryResourceType,
} from "@argus/ui";
import { useMyPermissions } from "../lib/permissions";
import { makeQuery } from "../components/observe/model";
import { useObserveWorkspace } from "../components/observe/use-workspace";
import { ObserveQueryCard } from "../components/observe/query-card";
import { ObserveCatalog } from "../components/observe/catalog";
import { TimeRangePicker } from "../components/dashboards/time-range-picker";
import {
  freezeObservationTime,
  type ObservationTimeRange,
} from "../components/observe/time-range";
import "../styles/dashboards.css";
import "../styles/observe.css";

export function ObservePage() {
  const session = useEnterpriseAuthStore((s) => s.session),
    permissions = useMyPermissions(),
    { t } = useTranslation();
  const allowedKinds = useMemo<QueryResourceType[]>(
    () =>
      Object.entries({
        host: "host.read",
        kubernetes_cluster: "kubernetes.read",
        kubernetes_namespace: "kubernetes.read",
        application: "application.read",
        monitoring_target: "monitoring.read",
      })
        .filter(
          ([, permission]) =>
            permissions.has("*") || permissions.has(permission),
        )
        .map(([kind]) => kind as QueryResourceType),
    [permissions],
  );
  const actorKey = JSON.stringify([
    session?.user.id,
    session?.session.enterprise_id,
    session?.session.id,
    session?.session.authorization_version,
    [...permissions].sort(),
  ]);
  if (!allowedKinds.length)
    return (
      <PageShell title={t("observe.title")}>
        <EmptyState
          title={t("observe.forbidden")}
          description={t("observe.forbiddenHint")}
        />
      </PageShell>
    );
  return (
    <Workspace key={actorKey} actorKey={actorKey} allowedKinds={allowedKinds} />
  );
}
function Workspace({
  actorKey,
  allowedKinds,
}: {
  actorKey: string;
  allowedKinds: QueryResourceType[];
}) {
  const search = useSearch({ strict: false }) as {
    resource_type?: ObservationResourceRef["type"];
    resource_id?: string;
    signal?: ObservationSignal;
  };
  const { t } = useTranslation();
  const [initial] = useState(() =>
    makeQuery(search.signal ?? "metrics", {
      mode: "explicit",
      resources:
        search.resource_type && search.resource_id
          ? [{ type: search.resource_type, id: search.resource_id }]
          : [],
    }),
  );
  const workspace = useObserveWorkspace(initial);
  const [tab, setTab] = useState("query"),
    [selection, setSelection] = useState<ObservationTimeRange>(() => ({
      kind: "absolute",
      ...freezeObservationTime({ kind: "relative", seconds: 3600 }).range,
    })),
    [linked, setLinked] = useState(true);
  const [group] = useState(() => crypto.randomUUID());
  const time = useMemo(
    () => freezeObservationTime(selection).range,
    [selection],
  );
  const add = (q = makeQuery()) => {
    workspace.add(q);
    setTab("query");
  };
  const timeControl = (
    <TimeRangePicker
      label={t("telemetry.timeRange")}
      value={selection}
      onChange={(value) => {
        if (JSON.stringify(value) === JSON.stringify(selection)) return;
        setSelection(value);
        workspace.invalidateAll();
      }}
    />
  );
  return (
    <PageShell
      title={t("observe.title")}
      description={t("observe.description")}
      actions={
        <Button variant="primary" onPress={() => add()}>
          <Plus />
          {t("observe.newQuery")}
        </Button>
      }
      className="argus-observe-page"
    >
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="query">
            {t("observe.workspace")} · {workspace.queries.length}
          </TabsTrigger>
          <TabsTrigger value="catalog">{t("observe.catalog")}</TabsTrigger>
        </TabsList>
        {tab === "query" && (
          <Card className="argus-observe-global">
            <CardContent className="argus-observe-toolbar">
              <div>
                {tab === "query" && (
                  <Checkbox isSelected={linked} onChange={setLinked}>
                    {t("observe.linked")}
                  </Checkbox>
                )}
              </div>
              <div className="argus-observe-actions">
                {timeControl}
                {tab === "query" && (
                  <Button
                    variant="primary"
                    onPress={() =>
                      workspace.run(
                        workspace.queries.map((q) => q.id),
                        selection,
                      )
                    }
                  >
                    <Play />
                    {t("observe.runAll")}
                  </Button>
                )}
              </div>
            </CardContent>
          </Card>
        )}
        <TabsContent value="query" keepMounted>
          <div className="argus-observe-queries">
            {workspace.queries.map((q, index) => (
              <ObserveQueryCard
                key={q.id}
                query={q}
                index={index}
                total={workspace.queries.length}
                workspace={workspace}
                timeRange={selection}
                time={time}
                actorKey={actorKey}
                allowedKinds={allowedKinds}
                linkGroup={linked ? group : undefined}
              />
            ))}
            <Button
              className="argus-observe-add"
              variant="tertiary"
              onPress={() => add()}
            >
              <Plus />
              {t("observe.newQuery")}
            </Button>
            <p className="argus-observe-hint">{t("observe.temporary")}</p>
          </div>
        </TabsContent>
        <TabsContent value="catalog" keepMounted>
          <ObserveCatalog
            time={time}
            timeControl={timeControl}
            actorKey={actorKey}
            allowedKinds={allowedKinds}
            onNew={add}
          />
        </TabsContent>
      </Tabs>
    </PageShell>
  );
}
