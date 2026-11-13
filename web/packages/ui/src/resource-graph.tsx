import {
  useCallback,
  useId,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { Button } from "./button";
import { EmptyState } from "./display";
import { useUiText } from "./locale";
import {
  ResourceState,
  type ResourceVisualSnapshot,
} from "./resource-visualizations";
import { resourceKindName } from "./resource-kind-name";

const levels: Record<string, number> = {
  node: 0,
  hpa: 0,
  ingress: 0,
  cronjob: 0,
  pdb: 0,
  service: 1,
  networkpolicy: 1,
  deployment: 1,
  statefulset: 1,
  daemonset: 1,
  job: 1,
  replicaset: 2,
  controllerrevision: 2,
  endpointslice: 2,
  pod: 3,
  pvc: 4,
  secret: 4,
  configmap: 4,
  serviceaccount: 4,
  pv: 5,
  storageclass: 5,
};
type Edge = { key: string; path: string; label: string };

// CSS lays out cards without scaling their text. Connectors follow measured card
// positions; dense graphs scroll instead of compressing into overlapping labels.
export function ResourceGraph({
  snapshot,
  onSelect,
}: {
  snapshot: ResourceVisualSnapshot;
  onSelect?: (uid: string) => void;
}) {
  const text = useUiText(),
    world = useRef<HTMLDivElement>(null);
  const arrowId = useId().replace(/:/g, "");
  const [showInactive, setShowInactive] = useState(false),
    [hovered, setHovered] = useState<string>();
  const [edges, setEdges] = useState<Edge[]>([]);
  const inactive = useMemo(
    () =>
      new Set(
        snapshot.resources
          .filter(
            (node) =>
              node.uid !== snapshot.focus_uid &&
              ((node.type === "replicaset" &&
                node.summary.state === "scaled_down") ||
                (["pod", "job"].includes(node.type) &&
                  node.summary.state === "completed")),
          )
          .map((node) => node.uid),
      ),
    [snapshot],
  );
  const nodes = useMemo(
    () =>
      snapshot.resources.filter(
        (node) => showInactive || !inactive.has(node.uid),
      ),
    [snapshot, inactive, showInactive],
  );
  const columns = useMemo(() => {
    const result = new Map<number, typeof nodes>();
    for (const node of nodes) {
      const level = levels[node.type] ?? 2;
      result.set(level, [...(result.get(level) ?? []), node]);
    }
    return [...result.entries()].sort(([a], [b]) => a - b);
  }, [nodes]);
  const relationLabel = useCallback(
    (kind: string) =>
      ({
        owns: text("拥有", "Owns"),
        selects: text("选择", "Selects"),
        schedules: text("调度", "Schedules"),
        mounts: text("挂载", "Mounts"),
        binds: text("绑定", "Binds"),
        uses: text("使用", "Uses"),
        references: text("引用", "References"),
        routes: text("路由至", "Routes to"),
        scales: text("伸缩", "Scales"),
      })[kind] ?? kind,
    [text],
  );
  useLayoutEffect(() => {
    const root = world.current;
    if (!root) return;
    const measure = () => {
      const origin = root.getBoundingClientRect(),
        boxes = new Map<string, DOMRect>();
      root
        .querySelectorAll<HTMLElement>("[data-resource-uid]")
        .forEach((node) =>
          boxes.set(node.dataset.resourceUid!, node.getBoundingClientRect()),
        );
      setEdges(
        snapshot.relations.flatMap((edge) => {
          const a = boxes.get(edge.from),
            b = boxes.get(edge.to);
          if (
            !a ||
            !b ||
            (hovered && edge.from !== hovered && edge.to !== hovered)
          )
            return [];
          const forward = b.left >= a.left,
            x1 = (forward ? a.right : a.left) - origin.left,
            x2 = (forward ? b.left : b.right) - origin.left;
          const y1 = a.top + a.height / 2 - origin.top,
            y2 = b.top + b.height / 2 - origin.top;
          const mid = (x1 + x2) / 2;
          return [
            {
              key: edge.from + edge.to + edge.kind,
              path: `M${x1},${y1} C${mid},${y1} ${mid},${y2} ${x2},${y2}`,
              label: relationLabel(edge.kind),
            },
          ];
        }),
      );
    };
    const observer = new ResizeObserver(measure);
    observer.observe(root);
    root
      .querySelectorAll("[data-resource-uid]")
      .forEach((node) => observer.observe(node));
    measure();
    return () => observer.disconnect();
  }, [nodes, snapshot.relations, hovered, relationLabel]);
  if (!nodes.length)
    return (
      <EmptyState
        title={text("选择一个工作负载", "Select a workload")}
        description={text(
          "查看其拥有、选择、挂载和引用的资源。",
          "Explore ownership, selection, mounts and references.",
        )}
      />
    );
  return (
    <div className="argus-resource-graph">
      <div className="argus-resource-graph__toolbar">
        <span>
          {text(
            "资源关系 · 点击卡片查看详情",
            "Resource relationships · Select a card for details",
          )}
        </span>
        {hovered && (
          <span>
            {[
              ...new Set(
                snapshot.relations
                  .filter(
                    (edge) => edge.from === hovered || edge.to === hovered,
                  )
                  .map((edge) => relationLabel(edge.kind)),
              ),
            ].join(" · ")}
          </span>
        )}
        {inactive.size > 0 && (
          <Button
            size="sm"
            variant="ghost"
            onPress={() => setShowInactive(!showInactive)}
          >
            {showInactive
              ? text("隐藏历史副本", "Hide inactive resources")
              : text("显示历史副本", "Show inactive resources")}{" "}
            ({inactive.size})
          </Button>
        )}
      </div>
      <div
        className="argus-resource-graph__viewport"
        tabIndex={0}
        role="region"
        aria-label={text(
          "可滚动的资源关系图",
          "Scrollable resource relationships",
        )}
      >
        <div className="argus-resource-graph__world" ref={world}>
          <svg className="argus-resource-graph__edges" aria-hidden="true">
            <defs>
              <marker
                id={arrowId}
                markerWidth="6"
                markerHeight="6"
                refX="6"
                refY="3"
                orient="auto"
              >
                <path d="M0,0 L6,3 L0,6 Z" />
              </marker>
            </defs>
            {edges.map((edge) => (
              <path key={edge.key} d={edge.path} markerEnd={`url(#${arrowId})`}>
                <title>{edge.label}</title>
              </path>
            ))}
          </svg>
          {columns.map(([level, rows]) => (
            <div className="argus-resource-graph__column" key={level}>
              {rows.map((node) => (
                <Button
                  key={node.uid}
                  layout="content"
                  variant="ghost"
                  className="argus-resource-graph__node"
                  data-resource-uid={node.uid}
                  data-graph-root={node.uid === snapshot.focus_uid || undefined}
                  onHoverStart={() => setHovered(node.uid)}
                  onHoverEnd={() => setHovered(undefined)}
                  onFocus={() => setHovered(node.uid)}
                  onBlur={() => setHovered(undefined)}
                  onPress={() => onSelect?.(node.uid)}
                  title={`${resourceKindName(node.type)} · ${node.namespace ? node.namespace + " / " : ""}${node.name}`}
                >
                  <span className="argus-resource-graph__kind">
                    {resourceKindName(node.type)}
                  </span>
                  <strong>{node.name}</strong>
                  <small>
                    {node.namespace || text("集群资源", "Cluster resource")}
                  </small>
                  <ResourceState state={node.summary.state} />
                </Button>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
