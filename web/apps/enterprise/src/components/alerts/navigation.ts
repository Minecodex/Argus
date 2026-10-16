import { useEffect, useRef, useState } from "react";
import { useEnterpriseAuthStore } from "@argus/auth";
import type { AlertFilter } from "@argus/api-client";

export const alertTabs = [
  "active",
  "history",
  "deliveries",
  "policies",
  "noise",
  "channels",
] as const;
type AlertTab = (typeof alertTabs)[number];
export type AlertDetail = {
  type: "instance" | "delivery" | "policy";
  id: string;
};
const filterNames = [
  "query",
  "severity",
  "lifecycle",
  "notification_state",
  "policy_id",
  "resource_id",
  "rule_id",
  "metric",
  "channel_id",
  "from",
  "to",
  "cursor",
] as const;
function scrollPosition() {
  return document.querySelector("main")?.scrollTop ?? window.scrollY;
}
function restoreScroll(top: number) {
  const main = document.querySelector("main");
  if (main) main.scrollTo({ top });
  else window.scrollTo(0, top);
}
function readLocation() {
  const search = new URLSearchParams(location.search);
  const tab =
    alertTabs.find(
      (v) => v === (search.get("tab") ?? location.hash.slice(1)),
    ) ?? "active";
  const filter: AlertFilter = { limit: 5 };
  for (const key of filterNames) {
    const value = search.get(key);
    if (value) filter[key] = value;
  }
  if (search.has("acknowledged"))
    filter.acknowledged = search.get("acknowledged") === "true";
  let detail: AlertDetail | null = null;
  for (const type of ["instance", "delivery", "policy"] as const) {
    const id = search.get(type);
    if (id) detail = { type, id };
  }
  return { tab, filter, detail };
}

export function useAlertNavigation() {
  const user = useEnterpriseAuthStore((s) => s.session?.user.id);
  const key = `argus.alertNavigation.${user ?? "anonymous"}`;
  const initial = useRef(readLocation());
  const [tab, setTab] = useState<AlertTab>(initial.current.tab);
  const [filter, setFilter] = useState<AlertFilter>(initial.current.filter);
  const [detail, setDetail] = useState<AlertDetail | null>(
    initial.current.detail,
  );
  const views = useRef<
    Partial<
      Record<
        AlertTab,
        { filter: AlertFilter; cursors: string[]; scroll: number }
      >
    >
  >({});
  const [cursors, setCursors] = useState<string[]>([]);
  useEffect(() => {
    try {
      const saved = JSON.parse(sessionStorage.getItem(key) ?? "null") as {
        tab: AlertTab;
        filter: AlertFilter;
        cursors: string[];
        scroll: number;
      } | null;
      if (
        saved &&
        !location.search &&
        !location.hash &&
        alertTabs.includes(saved.tab)
      ) {
        setTab(saved.tab);
        setFilter(saved.filter);
        setCursors(saved.cursors);
        requestAnimationFrame(() => restoreScroll(saved.scroll));
      }
    } catch {
      /* Invalid browser preferences do not replace valid URL state. */
    }
    const restore = () => {
      const state = readLocation();
      setTab(state.tab);
      setFilter(state.filter);
      setDetail(state.detail);
      setCursors([]);
    };
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, [key]);
  useEffect(() => {
    const url = new URL(location.href);
    url.hash = "";
    url.search = "";
    url.searchParams.set("tab", tab);
    for (const [name, value] of Object.entries(filter)) {
      if (name !== "limit" && value !== undefined && value !== "")
        url.searchParams.set(name, String(value));
    }
    if (detail) url.searchParams.set(detail.type, detail.id);
    history.replaceState(history.state, "", url);
    const save = () =>
      sessionStorage.setItem(
        key,
        JSON.stringify({ tab, filter, cursors, scroll: scrollPosition() }),
      );
    save();
    const scrollTarget = document.querySelector("main") ?? window;
    scrollTarget.addEventListener("scroll", save, { passive: true });
    return () => {
      save();
      scrollTarget.removeEventListener("scroll", save);
    };
  }, [key, tab, filter, detail, cursors]);
  const changeTab = (value: string) => {
    const next = alertTabs.find((v) => v === value);
    if (!next) return;
    views.current[tab] = { filter, cursors, scroll: scrollPosition() };
    const saved = views.current[next];
    setTab(next);
    setFilter(saved?.filter ?? { limit: 5 });
    setCursors(saved?.cursors ?? []);
    requestAnimationFrame(() => restoreScroll(saved?.scroll ?? 0));
  };
  return {
    tab,
    changeTab,
    filter,
    setFilter,
    detail,
    setDetail,
    cursors,
    setCursors,
  };
}
