import { useEffect, useRef, useState } from "react";
import { useQueryClient, type QueryKey } from "@tanstack/react-query";
import {
  ApiError,
  useApi,
  type KubernetesInventorySchemas,
} from "@argus/api-client";

type Page = KubernetesInventorySchemas["KubernetesInventoryPage"];
export function useInventoryWatch(
  cluster: string,
  key: QueryKey,
  page: Page | undefined,
  enabled: boolean,
  relist: () => void,
) {
  const api = useApi(),
    cache = useQueryClient(),
    refresh = useRef(relist);
  refresh.current = relist;
  const [status, setStatus] = useState<
      "off" | "connecting" | "live" | "reconnecting" | "failed"
    >("off"),
    [changes, setChanges] = useState(false),
    [error, setError] = useState<unknown>();
  const keyJSON = JSON.stringify(key),
    firstToken = page?.watch_token;
  useEffect(() => {
    setChanges(false);
    setError(undefined);
    if (!enabled || !firstToken) {
      setStatus("off");
      return;
    }
    const controller = new AbortController(),
      queryKey = JSON.parse(keyJSON) as QueryKey;
    let stopped = false,
      token = firstToken,
      delay = 1000;
    const wait = (ms: number) =>
      new Promise<void>((resolve) => {
        if (controller.signal.aborted) {
          resolve();
          return;
        }
        const finish = () => {
          clearTimeout(timer);
          controller.signal.removeEventListener("abort", finish);
          resolve();
        };
        const timer = setTimeout(finish, ms);
        controller.signal.addEventListener("abort", finish, { once: true });
      });
    void (async () => {
      setStatus("connecting");
      while (!stopped) {
        try {
          const result = await api.kubernetesWorkbench.watch(
            cluster,
            token,
            controller.signal,
          );
          if (stopped) return;
          token = result.watch_token;
          setStatus("live");
          setError(undefined);
          delay = 1000;
          cache.setQueryData<Page>(queryKey, (current) => {
            if (!current || current.scope_signature !== result.scope_signature)
              return current;
            const rows = [...current.items];
            let changed = false;
            for (const event of result.events) {
              const index = rows.findIndex(
                (row) => row.uid === event.object.uid,
              );
              if (event.type === "DELETED") {
                if (index >= 0) {
                  rows.splice(index, 1);
                  changed = true;
                } else setChanges(true);
              } else if (index >= 0) {
                rows[index] = event.object;
                changed = true;
              } else {
                setChanges(true);
              }
            }
            return changed
              ? { ...current, items: rows, observed_at: result.observed_at }
              : current;
          });
          await wait(result.more ? 25 : 1000);
        } catch (cause) {
          if (stopped) return;
          if (
            cause instanceof ApiError &&
            (cause.status === 410 ||
              cause.code === "KUBERNETES_OBJECT_IDENTITY_CHANGED")
          ) {
            setStatus("reconnecting");
            refresh.current();
            return;
          }
          if (
            cause instanceof ApiError &&
            [400, 401, 403, 409, 413].includes(cause.status)
          ) {
            setStatus("failed");
            setError(cause);
            void cache.invalidateQueries({
              queryKey: ["kubernetes-workbench", cluster, "scope"],
            });
            return;
          }
          setStatus("reconnecting");
          setError(cause);
          await wait(delay);
          delay = Math.min(delay * 2, 10000);
        }
      }
    })();
    return () => {
      stopped = true;
      controller.abort();
    };
  }, [api, cache, cluster, enabled, firstToken, keyJSON]);
  return { status, changes, error };
}
