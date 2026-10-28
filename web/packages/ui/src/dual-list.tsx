import { Pagination } from "./pagination";
import { Checkbox } from "./checkbox";
import { useEffect, useMemo, useState } from "react";
import { Button } from "./button";

export type DualListItem = {
  id: string;
  label: string;
  inherited?: boolean;
  source?: string;
};

export type ResourceAuthorizationDualListLabels = {
  host: string;
  kubernetes: string;
  dashboard?: string;
  searchPlaceholder: string;
  available: string;
  authorized: string;
  inherited: string;
  moveSelected: string;
  moveAll: string;
  removeSelected: string;
  removeAll: string;
  previousPage: string;
  nextPage: string;
};

type ResourceType = "host" | "kubernetes_cluster" | "dashboard";
type ResourceValues = {
  host: string[];
  kubernetes_cluster: string[];
  dashboard?: string[];
};
type Side = "available" | "authorized";
const PAGE_SIZE = 20;

export function ResourceAuthorizationDualList({
  hosts,
  clusters,
  dashboards,
  value,
  labels,
  onChange,
}: {
  hosts: DualListItem[];
  clusters: DualListItem[];
  dashboards?: DualListItem[];
  value: ResourceValues;
  labels: ResourceAuthorizationDualListLabels;
  onChange: (next: ResourceValues) => void;
}) {
  const [tab, setTab] = useState<ResourceType>("host");
  const [query, setQuery] = useState("");
  const [pages, setPages] = useState({ available: 1, authorized: 1 });
  const [selected, setSelected] = useState<
    Record<ResourceType, Record<Side, string[]>>
  >({
    host: { available: [], authorized: [] },
    kubernetes_cluster: { available: [], authorized: [] },
    dashboard: { available: [], authorized: [] },
  });
  const items = useMemo(
    () =>
      tab === "host"
        ? hosts
        : tab === "kubernetes_cluster"
          ? clusters
          : (dashboards ?? []),
    [tab, hosts, clusters, dashboards],
  );
  const authorized = useMemo(() => new Set(value[tab] ?? []), [value, tab]);
  const filtered = useMemo(
    () =>
      items.filter((item) =>
        item.label.toLowerCase().includes(query.trim().toLowerCase()),
      ),
    [items, query],
  );
  const available = useMemo(
    () => filtered.filter((item) => !authorized.has(item.id)),
    [filtered, authorized],
  );
  const granted = useMemo(
    () => filtered.filter((item) => authorized.has(item.id)),
    [filtered, authorized],
  );
  const pageCount = (count: number) =>
    Math.max(1, Math.ceil(count / PAGE_SIZE));
  const availablePageCount = pageCount(available.length);
  const grantedPageCount = pageCount(granted.length);
  const pageItems = (list: DualListItem[], side: Side) =>
    list.slice((pages[side] - 1) * PAGE_SIZE, pages[side] * PAGE_SIZE);

  useEffect(() => {
    setPages({ available: 1, authorized: 1 });
    setSelected((current) => ({
      ...current,
      [tab]: { available: [], authorized: [] },
    }));
  }, [tab, query]);

  useEffect(() => {
    setPages((old) => ({
      available: Math.min(old.available, availablePageCount),
      authorized: Math.min(old.authorized, grantedPageCount),
    }));
  }, [availablePageCount, grantedPageCount]);

  const updateSelection = (side: Side, id: string, checked: boolean) =>
    setSelected((current) => {
      const ids = new Set(current[tab][side]);
      if (checked) ids.add(id);
      else ids.delete(id);
      return { ...current, [tab]: { ...current[tab], [side]: [...ids] } };
    });

  const move = (ids: string[], add: boolean) => {
    if (!ids.length) return;
    const next = new Set(value[tab] ?? []);
    ids.forEach((id) => {
      const item = items.find((entry) => entry.id === id);
      if (!item || item.inherited) return;
      if (add) next.add(id);
      else next.delete(id);
    });
    onChange({ ...value, [tab]: [...next] });
    setSelected((current) => ({
      ...current,
      [tab]: { available: [], authorized: [] },
    }));
  };

  const moveAll = (add: boolean) =>
    move(
      (add ? available : granted)
        .filter((item) => !item.inherited)
        .map((item) => item.id),
      add,
    );

  const renderItem = (item: DualListItem, side: Side) => (
    <Checkbox
      className="argus-dual-list__item"
      key={item.id}
      isSelected={selected[tab][side].includes(item.id)}
      isDisabled={item.inherited}
      onChange={(selected) => updateSelection(side, item.id, selected)}
    >
      <span className="argus-dual-list__item-label">{item.label}</span>
      {item.inherited ? (
        <small className="argus-dual-list__item-source">
          {item.source || labels.inherited}
        </small>
      ) : null}
    </Checkbox>
  );

  const pager = (count: number, side: Side) => (
    <Pagination
      page={pages[side]}
      totalPages={count}
      onChange={(page) => setPages((old) => ({ ...old, [side]: page }))}
    />
  );

  const column = (side: Side, list: DualListItem[], count: number) => {
    const add = side === "available";
    return (
      <section>
        <header>
          <span>
            {add ? labels.available : labels.authorized} ({list.length})
          </span>
          <span>
            <Button
              isDisabled={!selected[tab][side].length}
              onPress={() => move(selected[tab][side], add)}
              size="sm"
              variant="ghost"
            >
              {add ? labels.moveSelected : labels.removeSelected}
            </Button>
            <Button
              isDisabled={!list.some((item) => !item.inherited)}
              onPress={() => moveAll(add)}
              size="sm"
              variant="ghost"
            >
              {add ? labels.moveAll : labels.removeAll}
            </Button>
          </span>
        </header>
        {pageItems(list, side).map((item) => renderItem(item, side))}
        {pager(count, side)}
      </section>
    );
  };

  return (
    <div className="argus-dual-list" data-resource-type={tab}>
      <div className="argus-dual-list__tabs">
        <Button
          onPress={() => setTab("host")}
          size="sm"
          variant={tab === "host" ? "secondary" : "ghost"}
        >
          {labels.host}
        </Button>
        <Button
          onPress={() => setTab("kubernetes_cluster")}
          size="sm"
          variant={tab === "kubernetes_cluster" ? "secondary" : "ghost"}
        >
          {labels.kubernetes}
        </Button>
        {dashboards !== undefined && (
          <Button
            onPress={() => setTab("dashboard")}
            size="sm"
            variant={tab === "dashboard" ? "secondary" : "ghost"}
          >
            {labels.dashboard}
          </Button>
        )}
      </div>
      <input
        className="argus-input"
        onChange={(event) => setQuery(event.target.value)}
        placeholder={labels.searchPlaceholder}
        value={query}
      />
      <div className="argus-dual-list__columns">
        {column("available", available, availablePageCount)}
        {column("authorized", granted, grantedPageCount)}
      </div>
    </div>
  );
}
