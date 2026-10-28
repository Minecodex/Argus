import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { freezeObservationTime, type ObservationTimeRange } from "./time-range";
import {
  ApiError,
  formatApiError,
  useApi,
  type ObservationSignal,
  type DashboardPanel,
} from "@argus/api-client";
import {
  makeQuery,
  hasQueryResources,
  ObserveScheduler,
  ObservationConversionError,
  type ObserveQuery,
  type QueryResult,
  switchQuerySignal,
} from "./model";

export function useObserveWorkspace(initial: ObserveQuery) {
  const api = useApi(),
    { t } = useTranslation();
  const [queries, setQueries] = useState<ObserveQuery[]>([initial]);
  const current = useRef(queries);
  const scheduler = useRef(new ObserveScheduler());
  const validators = useRef(new Map<string, () => boolean>());
  const registerValidation = useCallback(
    (id: string, validate: () => boolean) => {
      validators.current.set(id, validate);
      return () => {
        validators.current.delete(id);
      };
    },
    [],
  );
  const alive = useRef(true);
  useEffect(() => {
    const activeScheduler = scheduler.current;
    alive.current = true;
    return () => {
      alive.current = false;
      activeScheduler.cancelAll();
    };
  }, []);
  const commit = (next: ObserveQuery[]) => {
    current.current = next;
    if (alive.current) setQueries(next);
  };
  const set = (id: string, fn: (q: ObserveQuery) => ObserveQuery) =>
    commit(current.current.map((q) => (q.id === id ? fn(q) : q)));
  const update = (id: string, fn: (q: ObserveQuery) => ObserveQuery) => {
    scheduler.current.cancel(id);
    set(id, (q) => ({
      ...fn(q),
      revision: q.revision + 1,
      status: "dirty",
      result: undefined,
      error: undefined,
      conversionError: undefined,
      compiled: undefined,
    }));
  };
  const add = (query = makeQuery()) => commit([...current.current, query]);
  const cancel = (id: string) => {
    scheduler.current.cancel(id);
    set(id, (q) => ({
      ...q,
      revision: q.revision + 1,
      status: "cancelled",
      result: undefined,
      error: undefined,
    }));
  };
  const remove = (id: string) => {
    scheduler.current.cancel(id);
    commit(current.current.filter((q) => q.id !== id));
  };
  const duplicate = (id: string) => {
    const index = current.current.findIndex((q) => q.id === id);
    if (index < 0) return;
    const copy = structuredClone(current.current[index]!);
    copy.id = crypto.randomUUID();
    copy.panel.id = copy.id;
    copy.status = "idle";
    copy.result = undefined;
    copy.error = undefined;
    copy.revision = 0;
    commit([
      ...current.current.slice(0, index + 1),
      copy,
      ...current.current.slice(index + 1),
    ]);
  };
  const move = (id: string, delta: number) => {
    const items = [...current.current],
      i = items.findIndex((q) => q.id === id),
      n = i + delta;
    if (i < 0 || n < 0 || n >= items.length) return;
    [items[i], items[n]] = [items[n]!, items[i]!];
    commit(items);
  };
  const invalidateAll = () => {
    scheduler.current.cancelAll();
    commit(
      current.current.map((q) => ({
        ...q,
        revision: q.revision + 1,
        status: "dirty",
        result: undefined,
        error: undefined,
        compiled: undefined,
      })),
    );
  };
  const valid = (id: string, revision: number, signal: AbortSignal) =>
    alive.current &&
    !signal.aborted &&
    current.current.some((q) => q.id === id && q.revision === revision);
  const convert = async (
    panel: DashboardPanel,
    mode: "builder" | "dsl",
    signal: AbortSignal,
  ) => {
    const result = await api.dashboards.convertPanel({ panel, mode }, signal);
    if (!result.converted)
      throw new ObservationConversionError(
        result.issues.map((i) => i.message).join("; ") ||
          t("observe.conversionFailed"),
      );
    return result.panel;
  };
  const switchMode = (id: string, mode: "builder" | "dsl") => {
    const old = current.current.find((q) => q.id === id);
    if (!old || old.panel.authoring_mode === mode) return;
    if (validators.current.get(id)?.() === false) return;
    const target = old.panel.targets[0]!,
      definition = target.source_definition;
    const canonical = (value: object) =>
      JSON.stringify(
        Object.fromEntries(
          Object.entries(value)
            .filter(([, v]) => v !== undefined)
            .sort(([a], [b]) => a.localeCompare(b)),
        ),
      );
    const emptyBuilder =
      old.panel.signal === "metrics" &&
      definition.builder &&
      canonical(definition.builder) ===
        canonical(makeQuery().panel.targets[0]!.source_definition.builder!);
    const emptyCode =
      definition.dsl &&
      !definition.dsl.expression.trim() &&
      !definition.dsl.pipeline &&
      !definition.dsl.operation &&
      !Object.keys(definition.dsl.variables ?? {}).length;
    if ((mode === "dsl" && emptyBuilder) || (mode === "builder" && emptyCode)) {
      update(id, (q) => ({
        ...q,
        panel: {
          ...q.panel,
          authoring_mode: mode,
          targets: [
            {
              ...target,
              source_definition:
                mode === "dsl"
                  ? { dsl: { expression: "" } }
                  : makeQuery(q.panel.signal).panel.targets[0]!
                      .source_definition,
            },
          ],
        },
      }));
      return;
    }
    scheduler.current.cancel(id);
    const revision = old.revision + 1;
    set(id, (q) => ({
      ...q,
      revision,
      status: "queued",
      conversionError: undefined,
    }));
    scheduler.current.schedule(id, async (signal) => {
      try {
        const panel = await convert(structuredClone(old.panel), mode, signal);
        if (valid(id, revision, signal))
          set(id, (q) => ({
            ...q,
            panel,
            status: q.result ? "success" : "dirty",
          }));
      } catch (error) {
        if (valid(id, revision, signal))
          set(id, (q) => ({
            ...q,
            status: q.result ? "success" : "dirty",
            conversionError:
              error instanceof Error
                ? error.message
                : t("observe.conversionFailed"),
          }));
      }
    });
  };
  const run = (ids: string[], selection: ObservationTimeRange) => {
    const { range, seconds } = freezeObservationTime(selection);
    for (const id of ids) {
      const original = current.current.find((q) => q.id === id);
      if (!original) continue;
      if (validators.current.get(id)?.() === false) {
        scheduler.current.cancel(id);
        set(id, (q) => ({
          ...q,
          revision: q.revision + 1,
          status: "error",
          result: undefined,
          error: t("observe.invalidFields"),
        }));
        continue;
      }
      if (!hasQueryResources(original.scope) || !original.scope.source_type) {
        set(id, (q) => ({
          ...q,
          status: "empty",
          result: undefined,
          error: t("observe.selectResourcesSource"),
        }));
        continue;
      }
      if (
        original.panel.authoring_mode === "builder" &&
        original.panel.signal === "metrics" &&
        !original.panel.targets[0]?.source_definition.builder?.metric
      ) {
        set(id, (q) => ({
          ...q,
          status: "error",
          error: t("observe.selectMetric"),
        }));
        continue;
      }
      if (
        original.panel.authoring_mode === "dsl" &&
        !original.panel.targets[0]?.source_definition.dsl?.expression.trim()
      ) {
        set(id, (q) => ({
          ...q,
          status: "error",
          error: t("observe.enterQuery"),
        }));
        continue;
      }
      scheduler.current.cancel(id);
      const snapshot = structuredClone(original),
        revision = original.revision + 1;
      set(id, (q) => ({
        ...q,
        revision,
        status: "queued",
        error: undefined,
        result: undefined,
        range,
      }));
      scheduler.current.schedule(id, async (signal) => {
        if (!valid(id, revision, signal)) return;
        set(id, (q) => ({ ...q, status: "running" }));
        try {
          const panel =
            snapshot.panel.authoring_mode === "builder"
              ? await convert(snapshot.panel, "dsl", signal)
              : snapshot.panel;
          if (!valid(id, revision, signal)) return;
          const target = panel.targets[0]!,
            dsl = target.source_definition.dsl!;
          const input = {
            scope: snapshot.scope,
            query: dsl.expression,
            time_range: range,
            budget: {
              max_scan_bytes: 268435456,
              max_rows: 50000,
              max_samples: 5_000_000,
              max_series: 100_000,
              timeout_ms: 10000,
              max_result_bytes: 8388608,
            },
          };
          let result: QueryResult;
          if (panel.signal === "metrics") {
            const response =
              target.query_mode === "instant"
                ? await api.telemetry.queryMetrics(input, signal)
                : await api.telemetry.queryMetricsRange(
                    {
                      ...input,
                      step_seconds:
                        target.range_step_policy.kind === "fixed"
                          ? (target.range_step_policy.seconds ?? 60)
                          : Math.max(
                              target.range_step_policy.min_step_seconds ?? 1,
                              Math.ceil(
                                seconds /
                                  (target.range_step_policy.target_points ??
                                    300),
                              ),
                            ),
                    },
                    signal,
                  );
            result = {
              result_type: response.data.resultType,
              data: response.data.result,
              warnings: response.warnings,
              partial: response.argus_meta.partial,
              elapsed_ms: response.argus_meta.elapsed_ms,
            };
          } else if (panel.signal === "logs") {
            const response = await api.telemetry.queryLogs(
              { ...input, pipeline: dsl.pipeline },
              signal,
            );
            result = { ...response, elapsed_ms: response.meta.elapsed_ms };
          } else {
            const response = await api.telemetry.queryTraces(
              {
                ...input,
                variables: dsl.variables,
                operation_name: dsl.operation,
              },
              signal,
            );
            if (response.errors?.length && !response.data)
              throw new Error(response.errors.map((e) => e.message).join("; "));
            result = {
              result_type: "traces",
              data: response.data,
              warnings: response.errors?.map((e) => e.message),
              partial: response.extensions.argus.partial,
              elapsed_ms: response.extensions.argus.elapsed_ms,
            };
          }
          if (valid(id, revision, signal))
            set(id, (q) => ({
              ...q,
              status: "success",
              result,
              compiled: [dsl.expression, dsl.pipeline]
                .filter(Boolean)
                .join("\n"),
              range,
            }));
        } catch (error) {
          if (valid(id, revision, signal))
            set(id, (q) => ({
              ...q,
              status:
                error instanceof ApiError &&
                error.code === "QUERY_SOURCES_EMPTY"
                  ? "empty"
                  : "error",
              error:
                error instanceof ObservationConversionError
                  ? error.message
                  : formatApiError(error, t("observe.failed"), (id) => id),
            }));
        }
      });
    }
  };
  const signal = (id: string, value: ObservationSignal) =>
    update(id, (q) => switchQuerySignal(q, value));
  return {
    queries,
    registerValidation,
    update,
    add,
    cancel,
    remove,
    duplicate,
    move,
    invalidateAll,
    switchMode,
    run,
    signal,
    annotate: set,
    setView: (id: string, view: "graph" | "table") =>
      set(id, (q) => ({ ...q, view })),
  };
}
export type ObserveWorkspace = ReturnType<typeof useObserveWorkspace>;
