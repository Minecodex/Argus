import { useEffect, useRef, useState } from "react";
export type InventoryPageLocation = {
  page_cursor?: string;
  page_index?: number;
};
type Options = {
  scopeKey: string;
  location: InventoryPageLocation;
  update: (value: InventoryPageLocation & { resource_uid?: string }) => void;
  fetchPage: (
    cursor: string | undefined,
    signal: AbortSignal,
  ) => Promise<{ next_cursor?: string }>;
};
// Native Kubernetes lists have forward-only cursors. A copied URL preserves its
// page number. Missing predecessors are rebuilt in explicit, cancellable batches
// of eight reads instead of serializing an unbounded cursor chain into the URL.
export function useInventoryPagination(options: Options) {
  const known = useRef(new Map<number, string | undefined>([[1, undefined]]));
  const controller = useRef<AbortController | undefined>(undefined);
  const identity = useRef(options.scopeKey);
  const [busy, setBusy] = useState(false),
    [recovery, setRecovery] = useState<{ target: number; reached: number }>(),
    [error, setError] = useState<unknown>();
  if (identity.current !== options.scopeKey) {
    controller.current?.abort();
    known.current = new Map([[1, undefined]]);
    identity.current = options.scopeKey;
  }
  const page = options.location.page_index ?? 1,
    unknown =
      (!!options.location.page_cursor && !options.location.page_index) ||
      (!options.location.page_cursor && page > 1);
  if (!unknown) known.current.set(page, options.location.page_cursor);
  useEffect(() => {
    setRecovery(undefined);
    setError(undefined);
    setBusy(false);
    return () => controller.current?.abort();
  }, [options.scopeKey]);
  const navigate = (target: number, cursor: string | undefined) => {
    known.current.set(target, cursor);
    setRecovery(undefined);
    setError(undefined);
    options.update({
      page_cursor: cursor,
      page_index: target === 1 ? undefined : target,
      resource_uid: undefined,
    });
  };
  const rebuild = async (target: number) => {
    controller.current?.abort();
    const task = new AbortController();
    controller.current = task;
    const scope = options.scopeKey;
    setBusy(true);
    setError(undefined);
    try {
      let index = Math.max(
          ...[...known.current.keys()].filter((value) => value <= target),
        ),
        cursor = known.current.get(index);
      for (let reads = 0; index < target && reads < 8; reads++) {
        const data = await options.fetchPage(cursor, task.signal);
        if (task.signal.aborted || identity.current !== scope) return;
        if (!data.next_cursor)
          throw new Error("KUBERNETES_PAGE_NO_LONGER_AVAILABLE");
        cursor = data.next_cursor;
        index++;
        known.current.set(index, cursor);
      }
      if (index === target) navigate(target, cursor);
      else setRecovery({ target, reached: index });
      // Keep a bounded navigation cache, retaining the origin and nearest pages.
      if (known.current.size > 256)
        for (const key of [...known.current.keys()].sort(
          (a, b) => Math.abs(b - target) - Math.abs(a - target),
        )) {
          if (key !== 1 && key !== target) known.current.delete(key);
          if (known.current.size <= 256) break;
        }
    } catch (cause) {
      if (!task.signal.aborted && identity.current === scope) setError(cause);
    } finally {
      if (controller.current === task) setBusy(false);
    }
  };
  return {
    page,
    unknown,
    cursor: options.location.page_cursor,
    busy,
    error,
    recovery,
    go: (target: number, next?: string) => {
      if (target === page + 1 && next) navigate(target, next);
      else if (known.current.has(target))
        navigate(target, known.current.get(target));
      else void rebuild(target);
    },
    continueRecovery: () => recovery && void rebuild(recovery.target),
    cancel: () => {
      controller.current?.abort();
      setBusy(false);
      setRecovery(undefined);
    },
  };
}
