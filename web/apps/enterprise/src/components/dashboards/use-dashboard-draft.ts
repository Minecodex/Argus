import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, useApi, type DashboardDraft } from "@argus/api-client";
import { draftInput } from "./model";

export function useDashboardDraft(id: string) {
  const api = useApi();
  const [draft, setDraft] = useState<DashboardDraft | null>(null),
    [error, setError] = useState<unknown>(),
    [saving, setSaving] = useState(false),
    [dirty, setDirty] = useState(false),
    [conflict, setConflict] = useState(false);
  const latest = useRef<DashboardDraft | null>(null),
    baseline = useRef<DashboardDraft | null>(null),
    changed = useRef(0),
    saved = useRef(0),
    running = useRef<Promise<DashboardDraft> | null>(null);
  const install = useCallback((value: DashboardDraft) => {
    latest.current = structuredClone(value);
    baseline.current = structuredClone(value);
    changed.current = 0;
    saved.current = 0;
    setDraft(latest.current);
    setDirty(false);
    setError(undefined);
    setConflict(false);
  }, []);
  useEffect(() => {
    let active = true;
    void api.dashboards
      .draft(id)
      .then((value) => {
        if (active) install(value);
      })
      .catch((e) => {
        if (active) setError(e);
      });
    return () => {
      active = false;
    };
  }, [api, id, install]);
  const update = useCallback((value: DashboardDraft) => {
    latest.current = value;
    changed.current++;
    setDraft(value);
    setDirty(true);
    setError(undefined);
  }, []);
  const save = useCallback(async (): Promise<DashboardDraft> => {
    while (running.current) await running.current;
    if (!latest.current || !baseline.current)
      throw new Error("DASHBOARD_NOT_FOUND");
    if (changed.current === saved.current) return baseline.current;
    const perform = async () => {
      setSaving(true);
      try {
        while (changed.current !== saved.current) {
          const generation = changed.current;
          const value = structuredClone(latest.current!);
          const stored = await api.dashboards.saveDraft(id, {
            ...draftInput(value),
            expected_version: baseline.current!.draft_version,
          });
          baseline.current = stored;
          saved.current = generation;
          const next =
            generation === changed.current
              ? stored
              : { ...latest.current!, draft_version: stored.draft_version };
          latest.current = next;
          setDraft(next);
        }
        setDirty(false);
        setError(undefined);
        return baseline.current!;
      } catch (e) {
        setError(e);
        if (e instanceof ApiError && e.code === "DASHBOARD_VERSION_CONFLICT")
          setConflict(true);
        throw e;
      } finally {
        setSaving(false);
        running.current = null;
      }
    };
    running.current = perform();
    return running.current;
  }, [api, id]);
  useEffect(() => {
    if (!dirty || conflict || error) return;
    const timer = setTimeout(() => {
      void save().catch(() => undefined);
    }, 900);
    return () => clearTimeout(timer);
  }, [draft, dirty, conflict, error, save]);
  return {
    draft,
    update,
    save,
    install,
    error,
    setError,
    saving,
    dirty,
    conflict,
    setConflict,
    current: latest,
    baseline,
  };
}
