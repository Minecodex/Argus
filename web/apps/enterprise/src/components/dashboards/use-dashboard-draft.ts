import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError, useApi, type DashboardDraft } from "@argus/api-client";
import { draftInput } from "./model";
import { mergeDraftMutation } from "./merge-draft-mutation";

export function useDashboardDraft(id: string) {
  const api = useApi();
  const [draft, setDraft] = useState<DashboardDraft | null>(null),
    [loading, setLoading] = useState(true),
    [loadAttempt, setLoadAttempt] = useState(0),
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
    setLoading(true);
    setError(undefined);
    void api.dashboards
      .draft(id)
      .then((value) => {
        if (active) install(value);
      })
      .catch((e) => {
        if (active) setError(e);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [api, id, install, loadAttempt]);
  // Initial-read retry only: never replace an editor's buffered changes.
  const reload = useCallback(() => {
    if (!latest.current) setLoadAttempt((attempt) => attempt + 1);
  }, []);
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
  const mutate = useCallback(
    async (operation: (stored: DashboardDraft) => Promise<DashboardDraft>) => {
      await save();
      while (running.current) await running.current;
      if (!latest.current || !baseline.current)
        throw new Error("DASHBOARD_NOT_FOUND");
      const before = structuredClone(baseline.current),
        generation = saved.current;
      const perform = async () => {
        setSaving(true);
        try {
          const stored = await operation(before);
          baseline.current = stored;
          saved.current = generation;
          latest.current = mergeDraftMutation(before, stored, latest.current!);
          setDraft(latest.current);
          setDirty(changed.current !== saved.current);
          setError(undefined);
          return stored;
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
    },
    [save],
  );
  useEffect(() => {
    if (!dirty || conflict || error) return;
    const timer = setTimeout(() => {
      void save().catch(() => undefined);
    }, 900);
    return () => clearTimeout(timer);
  }, [draft, dirty, conflict, error, save]);
  return {
    draft,
    loading,
    reload,
    update,
    save,
    mutate,
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
