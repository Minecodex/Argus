import { FilterPopover } from "./filter-popover";
import { useState } from "react";
import { resourceKindName } from "./resource-kind-name";
import { EmptyState } from "./display";
export { ResourceGraph } from "./resource-graph";
import { Badge } from "./badge";
import { Button } from "./button";
import { DataTable } from "./table";
import { useUiText } from "./locale";
export type ResourceVisualNode = {
  uid: string;
  type: string;
  name: string;
  namespace: string;
  summary: { state: string; reason?: string; fields: Record<string, string> };
  created_at: string;
};
export type ResourceVisualRelation = { from: string; to: string; kind: string };
export type ResourceVisualSnapshot = {
  focus_uid?: string;
  cluster_wide: boolean;
  resources: ResourceVisualNode[];
  relations: ResourceVisualRelation[];
  counts: Record<string, Record<string, number>>;
  errors: Record<string, string>;
  complete: boolean;
  observed_at: string;
};
const tone = (state: string) =>
  state === "healthy"
    ? "success"
    : state === "failed"
      ? "danger"
      : ["pending", "warning"].includes(state)
        ? "warning"
        : state === "progressing"
          ? "accent"
          : "neutral";
const names: Record<string, [string, string]> = {
  healthy: ["就绪", "Ready"],
  unknown: ["未知", "Unknown"],
  configured: ["已配置", "Configured"],
  pending: ["等待中", "Pending"],
  progressing: ["等待生效", "Progressing"],
  warning: ["需要关注", "Needs attention"],
  failed: ["失败", "Failed"],
  completed: ["已完成", "Completed"],
  paused: ["已暂停", "Suspended"],
  scaled_down: ["已缩容至零", "Scaled to zero"],
  terminating: ["删除中", "Terminating"],
};
export function ResourceState({ state }: { state: string }) {
  const text = useUiText(),
    label = names[state];
  return <Badge tone={tone(state)}>{label ? text(...label) : state}</Badge>;
}
export function ResourceSummary({
  snapshot,
  onFilter,
  types,
}: {
  snapshot: ResourceVisualSnapshot;
  types?: string[];
  onFilter?: (type: string, state?: string) => void;
}) {
  const text = useUiText(),
    counts = snapshot.counts;
  const [workloadsOpen, setWorkloadsOpen] = useState(false);
  const workloads = ["deployment", "statefulset", "daemonset", "job"].filter(
    (kind) => !types?.length || types.includes(kind),
  );
  const attention = (key: string) =>
    ["failed", "warning", "pending", "unknown"].reduce(
      (sum, state) => sum + (counts[key]?.[state] ?? 0),
      0,
    );
  const cards = [
    ...(snapshot.cluster_wide
      ? [
          {
            key: "node",
            title: text("就绪节点", "Ready nodes"),
            value: counts.node
              ? `${counts.node.healthy ?? 0} / ${counts.node.total ?? 0}`
              : "—",
            state: undefined,
          },
        ]
      : []),
    {
      key: "pod",
      title: text("就绪 Pod", "Ready Pods"),
      value: counts.pod
        ? `${counts.pod.healthy ?? 0} / ${(counts.pod.total ?? 0) - (counts.pod.completed ?? 0)}`
        : "—",
      state: undefined,
    },
    {
      key: "deployment",
      title: text("异常工作负载", "Workloads needing attention"),
      value: ["deployment", "statefulset", "daemonset", "job"].some(
        (key) => snapshot.errors[key],
      )
        ? "—"
        : String(
            ["deployment", "statefulset", "daemonset", "job"].reduce(
              (sum, key) => sum + attention(key),
              0,
            ),
          ),
      state: "attention",
    },
    {
      key: "pvc",
      title: text("已绑定 PVC", "Bound PVCs"),
      value: counts.pvc
        ? `${counts.pvc.healthy ?? 0} / ${counts.pvc.total ?? 0}`
        : "—",
      state: undefined,
    },
    {
      key: "event",
      title: text("可见警告事件", "Visible warning events"),
      value: snapshot.errors.event ? "—" : String(counts.event?.warning ?? 0),
      state: "warning",
    },
  ];
  const shown = types?.length
    ? cards.filter((card) =>
        card.key === "deployment"
          ? workloads.length > 0
          : types.includes(card.key),
      )
    : cards;
  if (types?.length)
    for (const kind of types) {
      if (
        ![
          "node",
          "pod",
          "deployment",
          "statefulset",
          "daemonset",
          "job",
          "pvc",
          "event",
        ].includes(kind)
      )
        shown.push({
          key: kind,
          title: resourceKindName(kind),
          value: counts[kind] ? String(counts[kind]!.total ?? 0) : "—",
          state: undefined,
        });
    }
  return (
    <div className="argus-resource-summary">
      {shown.map((card) =>
        card.key === "deployment" ? (
          <FilterPopover
            key={card.key}
            open={workloadsOpen}
            onOpenChange={setWorkloadsOpen}
            title={card.title}
            trigger={
              <Button variant="ghost" className="argus-resource-summary__item">
                <span>{card.title}</span>
                <strong>{card.value}</strong>
              </Button>
            }
          >
            {workloads.map((kind) => (
              <Button
                key={kind}
                variant="ghost"
                onPress={() => {
                  setWorkloadsOpen(false);
                  onFilter?.(kind, "attention");
                }}
              >
                {kind} · {snapshot.errors[kind] ? "—" : attention(kind)}
              </Button>
            ))}
          </FilterPopover>
        ) : (
          <Button
            key={card.key}
            variant="ghost"
            className="argus-resource-summary__item"
            onPress={() => onFilter?.(card.key, card.state)}
          >
            <span>{card.title}</span>
            <strong>{snapshot.errors[card.key] ? "—" : card.value}</strong>
          </Button>
        ),
      )}
    </div>
  );
}
function memoryQuantity(value?: string) {
  if (!value) return "—";
  const match = /^([0-9.]+)([KMGTPE])i$/.exec(value);
  if (!match) return value;
  const bytes = Number(match[1]) * 1024 ** ("KMGTPE".indexOf(match[2]!) + 1);
  const exponent = Math.min(
    6,
    Math.max(0, Math.floor(Math.log(bytes || 1) / Math.log(1024))),
  );
  return (
    (bytes / 1024 ** exponent).toLocaleString(undefined, {
      maximumFractionDigits: 1,
    }) +
    " " +
    ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"][exponent]
  );
}
export function NodeAllocation({
  snapshot,
  onSelect,
}: {
  snapshot: ResourceVisualSnapshot;
  onSelect?: (uid: string) => void;
}) {
  const text = useUiText(),
    [page, setPage] = useState(0);
  const pods = snapshot.resources.filter((item) => item.type === "pod"),
    nodes = snapshot.resources.filter((item) => item.type === "node");
  const groups = [
    ...nodes.map((node) => ({ name: node.name, node })),
    ...Array.from(
      new Set(
        pods.map(
          (pod) => pod.summary.fields.node || text("尚未调度", "Unscheduled"),
        ),
      ),
    )
      .filter((name) => !nodes.some((node) => node.name === name))
      .map((name) => ({ name, node: undefined })),
  ];
  const visible = groups.slice(page * 8, page * 8 + 8);
  return (
    <div
      className="argus-node-allocation"
      style={{
        gridTemplateColumns: `repeat(${Math.min(2, visible.length) || 1}, minmax(0, 1fr))`,
      }}
    >
      {visible.map((group) => (
        <section className="argus-node-allocation__node" key={group.name}>
          <div className="argus-node-allocation__heading">
            <Button
              variant="ghost"
              onPress={() => group.node && onSelect?.(group.node.uid)}
              isDisabled={!group.node}
            >
              {group.name}
            </Button>
            {group.node && <ResourceState state={group.node.summary.state} />}
          </div>
          {group.node && (
            <p>
              {text("可分配", "Allocatable")}:{" "}
              {group.node.summary.fields.cpu_allocatable ?? "—"}{" "}
              {text("CPU ·", "CPU ·")}{" "}
              {memoryQuantity(group.node.summary.fields.memory_allocatable)}
            </p>
          )}
          <p>
            {text("Pod 请求（当前快照）", "Pod requests (current snapshot)")}:{" "}
            {snapshot.errors.pod
              ? "—"
              : (
                  pods
                    .filter(
                      (pod) =>
                        (pod.summary.fields.node ||
                          text("尚未调度", "Unscheduled")) === group.name &&
                        !["Succeeded", "Failed"].includes(
                          pod.summary.fields.phase ?? "",
                        ),
                    )
                    .reduce(
                      (sum, pod) =>
                        sum +
                        Number(pod.summary.fields.cpu_request_millicores ?? 0),
                      0,
                    ) / 1000
                ).toLocaleString()}{" "}
            {text("CPU ·", "CPU ·")}{" "}
            {snapshot.errors.pod
              ? "—"
              : (
                  pods
                    .filter(
                      (pod) =>
                        (pod.summary.fields.node ||
                          text("尚未调度", "Unscheduled")) === group.name &&
                        !["Succeeded", "Failed"].includes(
                          pod.summary.fields.phase ?? "",
                        ),
                    )
                    .reduce(
                      (sum, pod) =>
                        sum +
                        Number(pod.summary.fields.memory_request_bytes ?? 0),
                      0,
                    ) / 1048576
                ).toLocaleString(undefined, { maximumFractionDigits: 1 })}{" "}
            {text("MiB", "MiB")}
          </p>
          <div className="argus-node-allocation__pods">
            {pods
              .filter(
                (pod) =>
                  (pod.summary.fields.node ??
                    text("尚未调度", "Unscheduled")) === group.name,
              )
              .map((pod) => (
                <Button
                  key={pod.uid}
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  className={
                    "argus-resource-cell argus-resource-cell--" +
                    pod.summary.state
                  }
                  aria-label={
                    pod.namespace +
                    " / " +
                    pod.name +
                    " · " +
                    (names[pod.summary.state]
                      ? text(...names[pod.summary.state]!)
                      : pod.summary.state)
                  }
                  title={pod.name + " · " + pod.summary.state}
                  onPress={() => onSelect?.(pod.uid)}
                >
                  ●
                </Button>
              ))}
          </div>
        </section>
      ))}
      {groups.length > 8 && (
        <div className="argus-resource-pagination">
          <Button isDisabled={page === 0} onPress={() => setPage(page - 1)}>
            {text("上一页", "Previous")}
          </Button>
          <span>
            {page + 1} / {Math.ceil(groups.length / 8)}
          </span>
          <Button
            isDisabled={(page + 1) * 8 >= groups.length}
            onPress={() => setPage(page + 1)}
          >
            {text("下一页", "Next")}
          </Button>
        </div>
      )}
      <div className="argus-resource-legend">
        {[...new Set(pods.map((pod) => pod.summary.state))].map((state) => (
          <ResourceState key={state} state={state} />
        ))}
      </div>
      {!groups.length && (
        <p>
          {text(
            "当前范围没有节点或 Pod 数据",
            "No node or Pod data in this scope",
          )}
        </p>
      )}
    </div>
  );
}
export function ResourceDistribution({
  snapshot,
  onFilter,
}: {
  snapshot: ResourceVisualSnapshot;
  onFilter?: (type: string, state?: string) => void;
}) {
  const text = useUiText();
  return (
    <div className="argus-resource-distribution">
      {Object.entries(snapshot.counts)
        .filter(([kind]) => kind !== "event")
        .map(([kind, counts]) => (
          <section key={kind}>
            <strong>{resourceKindName(kind)}</strong>
            <div>
              {Object.entries(counts)
                .filter(([state, value]) => state !== "total" && value > 0)
                .map(([state, value]) => (
                  <Button
                    key={state}
                    size="sm"
                    variant="ghost"
                    onPress={() => onFilter?.(kind, state)}
                  >
                    <ResourceState state={state} />
                    <span>
                      {snapshot.errors[kind] ? "≥ " : ""}
                      {value}
                    </span>
                  </Button>
                ))}
            </div>
            {snapshot.errors[kind] && (
              <span>{text("结果不完整", "Incomplete")}</span>
            )}
          </section>
        ))}
    </div>
  );
}
export function ResourceTable({
  snapshot,
  events = false,
  onSelect,
  pageSize = 20,
}: {
  snapshot: ResourceVisualSnapshot;
  pageSize?: number;
  events?: boolean;
  onSelect?: (uid: string) => void;
}) {
  const text = useUiText(),
    [page, setPage] = useState(0);
  const rows = snapshot.resources;
  const current = Math.min(
    page,
    Math.max(0, Math.ceil(rows.length / pageSize) - 1),
  );
  if (!rows.length)
    return (
      <EmptyState
        title={text(
          events ? "当前没有警告事件" : "当前没有需要关注的资源",
          events ? "No warning events" : "No resources need attention",
        )}
        description={text(
          "当前授权范围内没有匹配项。",
          "There are no matching items in the authorized scope.",
        )}
      />
    );
  return (
    <div className="argus-resource-results">
      <DataTable
        data={rows.slice(current * pageSize, (current + 1) * pageSize)}
        getRowKey={(row) => row.uid}
        density="default"
        columns={[
          {
            key: "name",
            header: text("名称", "Name"),
            render: (row) => (
              <Button variant="ghost" onPress={() => onSelect?.(row.uid)}>
                {row.name}
              </Button>
            ),
          },
          { key: "namespace", header: text("命名空间", "Namespace") },
          {
            key: "state",
            header: text("状态", "State"),
            render: (row) => <ResourceState state={row.summary.state} />,
          },
          ...(events
            ? [
                {
                  key: "message",
                  header: text("原因 / 消息", "Reason / message"),
                  render: (row: ResourceVisualNode) =>
                    row.summary.fields.message ?? row.summary.reason ?? "—",
                },
              ]
            : [
                {
                  key: "type",
                  header: text("类型", "Type"),
                  render: (row: ResourceVisualNode) =>
                    resourceKindName(row.type),
                },
              ]),
        ]}
      />
      {!rows.length && (
        <p>
          {text("当前范围没有匹配资源", "No matching resources in this scope")}
        </p>
      )}
      {rows.length > pageSize && (
        <div className="argus-resource-pagination">
          <Button isDisabled={page === 0} onPress={() => setPage(page - 1)}>
            {text("上一页", "Previous")}
          </Button>
          <span>
            {page + 1} / {Math.ceil(rows.length / pageSize)}
          </span>
          <Button
            isDisabled={(page + 1) * pageSize >= rows.length}
            onPress={() => setPage(page + 1)}
          >
            {text("下一页", "Next")}
          </Button>
        </div>
      )}
    </div>
  );
}
