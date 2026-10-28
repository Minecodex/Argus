import { useEffect, useState } from "react";
import { Button } from "./button";
import { ResourcePickerDialog } from "./resource-picker";
import { Tabs, TabsList, TabsTrigger } from "./primitives";
import { useUiText } from "./locale";

export type QueryResourceType =
  | "host"
  | "kubernetes_cluster"
  | "kubernetes_namespace"
  | "application"
  | "monitoring_target";
export type QueryResourceRef = { type: QueryResourceType; id: string };
export type QueryResourceSelection = {
  mode: "explicit" | "all_authorized";
  resources: QueryResourceRef[];
};
export type QueryResourceRow = QueryResourceRef & {
  name: string;
  environment: string;
  parent_name?: string;
};
export type QueryResourceLoader = (
  query: {
    kind: QueryResourceType;
    search?: string;
    cursor?: string;
    limit?: number;
  },
  signal?: AbortSignal,
) => Promise<{ items: QueryResourceRow[]; next_cursor?: string }>;
const kinds: QueryResourceType[] = [
  "host",
  "kubernetes_cluster",
  "kubernetes_namespace",
  "application",
  "monitoring_target",
];
export function ObservationScopePicker({
  value,
  onChange,
  load,
  contextKey,
  description,
  disabled,
  allowedKinds = kinds,
}: {
  value: QueryResourceSelection;
  onChange: (value: QueryResourceSelection) => void;
  load: QueryResourceLoader;
  contextKey: string;
  description?: string;
  disabled?: boolean;
  allowedKinds?: QueryResourceType[];
}) {
  const text = useUiText(),
    [open, setOpen] = useState(false);
  return (
    <>
      <Button
        aria-haspopup="dialog"
        isDisabled={disabled}
        onPress={() => setOpen(true)}
        className="argus-observe-scope-button"
      >
        {text("资源范围", "Resources")} ·{" "}
        {value.mode === "all_authorized"
          ? text("全部有权资源", "All authorized")
          : text(
              `已选 ${value.resources.length} 项`,
              `${value.resources.length} selected`,
            )}
      </Button>
      {open && (
        <ScopeDialog
          key={contextKey}
          {...{ value, onChange, load, description, allowedKinds }}
          onClose={() => setOpen(false)}
        />
      )}
    </>
  );
}
function ScopeDialog({
  value,
  onChange,
  load,
  description,
  allowedKinds,
  onClose,
}: {
  value: QueryResourceSelection;
  onChange: (value: QueryResourceSelection) => void;
  load: QueryResourceLoader;
  description?: string;
  allowedKinds: QueryResourceType[];
  onClose: () => void;
}) {
  const text = useUiText();
  const [draft, setDraft] = useState(() => structuredClone(value));
  const [kind, setKind] = useState(
    allowedKinds.includes(value.resources[0]?.type ?? "host")
      ? (value.resources[0]?.type ?? "host")
      : (allowedKinds[0] ?? "host"),
  );
  const [search, setSearch] = useState(""),
    [cursors, setCursors] = useState<(string | undefined)[]>([undefined]);
  const [data, setData] = useState<QueryResourceRow[]>([]),
    [next, setNext] = useState<string>(),
    [loading, setLoading] = useState(true),
    [error, setError] = useState(false),
    [refresh, setRefresh] = useState(0);
  const cursor = cursors.at(-1);
  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError(false);
    setData([]);
    setNext(undefined);
    const timer = setTimeout(() => {
      void load(
        { kind, search: search.trim(), cursor, limit: 30 },
        controller.signal,
      )
        .then((page) => {
          if (!controller.signal.aborted) {
            setData(page.items);
            setNext(page.next_cursor);
          }
        })
        .catch(() => {
          if (!controller.signal.aborted) setError(true);
        })
        .finally(() => {
          if (!controller.signal.aborted) setLoading(false);
        });
    }, 150);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [load, kind, search, cursor, refresh]);
  const names: Record<QueryResourceType, string> = {
    host: text("主机", "Hosts"),
    kubernetes_cluster: text("集群", "Clusters"),
    kubernetes_namespace: "Namespace",
    application: text("应用", "Applications"),
    monitoring_target: text("监控目标", "Monitoring targets"),
  };
  return (
    <ResourcePickerDialog<QueryResourceRow>
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={text("选择资源范围", "Select resources")}
      description={description}
      data={data}
      columns={[
        { key: "name", header: text("名称", "Name") },
        { key: "environment", header: text("环境", "Environment") },
        {
          key: "parent_name",
          header: text("所属资源", "Parent resource"),
          render: (r) => r.parent_name || "—",
        },
      ]}
      getRowKey={(r) => `${r.type}:${r.id}`}
      getRowLabel={(r) => r.name}
      search={search}
      onSearch={(s) => {
        setSearch(s);
        setCursors([undefined]);
      }}
      loading={loading}
      error={
        error ? (
          <div>
            {text("资源目录读取失败。", "Resource directory failed.")}
            <Button onPress={() => setRefresh((n) => n + 1)}>
              {text("重试", "Retry")}
            </Button>
          </div>
        ) : undefined
      }
      page={cursors.length}
      hasPrevious={cursors.length > 1}
      hasNext={Boolean(next)}
      onPrevious={() => setCursors((xs) => xs.slice(0, -1))}
      onNext={() => setCursors((xs) => [...xs, next])}
      isDisabled={(row) =>
        draft.mode === "explicit" &&
        draft.resources.length >= 1000 &&
        !draft.resources.some((r) => r.id === row.id && r.type === row.type)
      }
      selectedKeys={
        draft.mode === "explicit"
          ? draft.resources.map((r) => `${r.type}:${r.id}`)
          : []
      }
      onToggle={(row) =>
        setDraft((old) => ({
          mode: "explicit",
          resources:
            old.mode !== "explicit"
              ? [{ type: row.type, id: row.id }]
              : old.resources.some(
                    (r) => r.type === row.type && r.id === row.id,
                  )
                ? old.resources.filter((r) => r.id !== row.id)
                : [...old.resources, { type: row.type, id: row.id }],
        }))
      }
      onConfirm={() => onChange(draft)}
      confirmDisabled={loading || error}
      filters={
        <>
          <Tabs
            value={kind}
            onValueChange={(v) => {
              setKind(v as QueryResourceType);
              setSearch("");
              setCursors([undefined]);
            }}
          >
            <TabsList>
              {allowedKinds.map((k) => (
                <TabsTrigger value={k} key={k}>
                  {names[k]}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <div className="argus-observe-scope-actions">
            <Button
              onPress={() =>
                setDraft({ mode: "all_authorized", resources: [] })
              }
            >
              {text("全部有权资源", "All authorized resources")}
            </Button>
            <Button
              onPress={() => setDraft({ mode: "explicit", resources: [] })}
            >
              {text("清空选择", "Clear selection")}
            </Button>
            <span role="status">
              {draft.mode === "all_authorized"
                ? text("全部有权资源", "All authorized resources")
                : text(
                    `已选 ${draft.resources.length} 项资源`,
                    `${draft.resources.length} resources selected`,
                  )}
            </span>
          </div>
        </>
      }
    />
  );
}
