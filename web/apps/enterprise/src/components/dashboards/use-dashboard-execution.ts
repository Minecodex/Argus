import { useCallback, useEffect, useRef, useState } from "react";
import type {
  DashboardExecution,
  DashboardRevision,
  DashboardSchemas,
} from "@argus/api-client";
import { changedVariables, dependentPanels } from "./execution-dependencies";
import { hasUnverifiedCandidates } from "./execution-status";

export type ViewInput = Pick<
  DashboardSchemas["DashboardExecutionInput"],
  "variables" | "local_values" | "resource_ids"
>;
type Range = DashboardSchemas["DashboardTimeRange"];
type Options = {
  dashboardId: string;
  revision?: DashboardRevision;
  resourceId?: string;
  enabled: boolean;
  execute: (
    id: string,
    input: DashboardSchemas["DashboardExecutionInput"],
    signal: AbortSignal,
  ) => Promise<DashboardExecution>;
  onRevisionChanged: () => Promise<unknown>;
  onReset: () => void;
  formatError: (error: unknown) => string;
};

export function useDashboardExecution(options: Options) {
  const current = useRef(options);
  current.current = options;
  const [execution, setExecution] = useState<DashboardExecution>();
  const [input, setInput] = useState<ViewInput>({
    resource_ids: [],
    variables: {},
    local_values: {},
  });
  const [timeRange, setTimeRange] = useState<Range>({
    kind: "relative",
    seconds: 3600,
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [refreshSeconds, setRefreshSeconds] = useState(0);
  const latest = useRef(input),
    rangeRef = useRef(timeRange);
  const bounds = useRef<{ from: string; to: string } | undefined>(undefined);
  const tokens = useRef(new Map<string, string>());
  const snapshot = useRef<DashboardExecution | undefined>(undefined);
  const generation = useRef(0);
  const active = useRef<
    { controller: AbortController; panels?: string[] } | undefined
  >(undefined);

  const run = useCallback(
    async (
      next: ViewInput,
      panels?: string[],
      range?: Range,
      automatic = false,
    ) => {
      const opts = current.current;
      if (
        !opts.enabled ||
        !opts.revision ||
        (automatic && (active.current || document.visibilityState === "hidden"))
      )
        return;
      const spec = opts.revision.spec;
      // A condition change supersedes the active request, but must also finish
      // every invalidated panel from that request. Never drop a local change.
      let requested = panels;
      if (active.current) {
        requested =
          !requested || !active.current.panels
            ? undefined
            : [...new Set([...requested, ...active.current.panels])];
        active.current.controller.abort();
      }
      if (!snapshot.current) requested = undefined;
      const ticket = ++generation.current;
      const controller = new AbortController();
      active.current = { controller, panels: requested };
      if (panels === undefined || !bounds.current) {
        const selected = range ?? rangeRef.current;
        const to = new Date();
        bounds.current =
          selected.kind === "absolute"
            ? { from: selected.from!, to: selected.to! }
            : {
                from: new Date(
                  to.getTime() - (selected.seconds ?? 3600) * 1000,
                ).toISOString(),
                to: to.toISOString(),
              };
      }
      if (range) {
        rangeRef.current = range;
        setTimeRange(range);
      }
      latest.current = next;
      setInput(next);
      opts.onReset();
      if (requested) requested.forEach((id) => tokens.current.delete(id));
      else tokens.current.clear();
      setBusy(true);
      setError("");
      try {
        // Server-proven fallback can invalidate additional panels. Reconcile
        // those in the same frozen interval without querying unrelated panels.
        while (true) {
          const result = await opts.execute(
            opts.dashboardId,
            {
              ...next,
              ...bounds.current,
              panel_ids: requested,
              candidates_only: requested?.length === 0,
            },
            controller.signal,
          );
          if (controller.signal.aborted || ticket !== generation.current)
            return;
          if (result.revision_id !== opts.revision.id) {
            snapshot.current = undefined;
            setExecution(undefined);
            tokens.current.clear();
            await opts.onRevisionChanged();
            return;
          }
          const returned = new Set(result.panels.map((p) => p.id));
          const effective: ViewInput = {
            ...next,
            variables: result.variables,
            local_values: requested
              ? {
                  ...next.local_values,
                  ...Object.fromEntries(
                    result.panels.map((p) => [
                      p.id,
                      result.local_values[p.id] ?? {},
                    ]),
                  ),
                }
              : result.local_values,
          };
          const previous = snapshot.current;
          const merged =
            requested && previous
              ? {
                  ...previous,
                  variables: result.variables,
                  variable_candidates: result.variable_candidates,
                  local_values: effective.local_values ?? {},
                  local_candidates: {
                    ...previous.local_candidates,
                    ...Object.fromEntries(
                      result.panels.map((p) => [
                        p.id,
                        result.local_candidates[p.id] ?? {},
                      ]),
                    ),
                  },
                  panels: previous.panels.map(
                    (p) => result.panels.find((v) => v.id === p.id) ?? p,
                  ),
                }
              : result;
          merged.partial =
            hasUnverifiedCandidates(merged) ||
            merged.panels.some((p) =>
              p.targets.some((v) =>
                ["partial", "error", "skipped_budget"].includes(v.status),
              ),
            );
          snapshot.current = merged;
          setExecution(merged);
          for (const panel of result.panels)
            if (result.context_token)
              tokens.current.set(panel.id, result.context_token);
          const remaining = requested
            ? dependentPanels(
                spec,
                changedVariables(next.variables, result.variables),
              ).filter((id) => !returned.has(id))
            : [];
          latest.current = next = effective;
          setInput(effective);
          if (!remaining.length) break;
          remaining.forEach((id) => tokens.current.delete(id));
          requested = remaining;
          active.current = { controller, panels: remaining };
        }
      } catch (e) {
        if (!controller.signal.aborted && ticket === generation.current)
          setError(opts.formatError(e));
      } finally {
        if (ticket === generation.current) {
          active.current = undefined;
          setBusy(false);
        }
      }
    },
    [],
  );

  useEffect(() => {
    const epoch = generation;
    generation.current++;
    active.current?.controller.abort();
    active.current = undefined;
    snapshot.current = undefined;
    bounds.current = undefined;
    tokens.current.clear();
    setExecution(undefined);
    setBusy(false);
    const opts = current.current;
    if (opts.enabled && opts.revision) {
      setRefreshSeconds(opts.revision.spec.default_refresh_seconds);
      const initial: ViewInput = {
        resource_ids: opts.resourceId ? [opts.resourceId] : [],
        variables: Object.fromEntries(
          opts.revision.spec.variables.map((v) => [v.name, v.default]),
        ),
        local_values: {},
      };
      void run(initial, undefined, opts.revision.spec.default_time_range);
    }
    return () => {
      epoch.current++;
      active.current?.controller.abort();
      active.current = undefined;
    };
  }, [
    options.dashboardId,
    options.revision?.id,
    options.resourceId,
    options.enabled,
    run,
  ]);

  useEffect(() => {
    const seconds = refreshSeconds;
    if (!options.enabled || seconds < 5) return;
    let timer: ReturnType<typeof setInterval> | undefined;
    const schedule = () => {
      clearInterval(timer);
      timer = undefined;
      if (document.visibilityState !== "hidden")
        timer = setInterval(
          () => void run(latest.current, undefined, undefined, true),
          seconds * 1000,
        );
    };
    schedule();
    document.addEventListener("visibilitychange", schedule);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", schedule);
    };
  }, [options.enabled, refreshSeconds, run]);

  const apply = useCallback(
    (next: ViewInput, panels?: string[]) => {
      void run(next, panels);
    },
    [run],
  );
  const applyVariable = useCallback(
    (name: string, value: DashboardSchemas["DashboardSelection"]) => {
      const spec = current.current.revision?.spec;
      if (!spec) return;
      const next = {
        ...latest.current,
        variables: { ...latest.current.variables, [name]: value },
      };
      void run(next, dependentPanels(spec, [name]));
    },
    [run],
  );
  return {
    refreshSeconds,
    setRefreshSeconds,
    execution,
    input,
    timeRange,
    error,
    setError,
    busy,
    bounds,
    latest,
    tokens,
    run,
    apply,
    applyVariable,
  };
}
