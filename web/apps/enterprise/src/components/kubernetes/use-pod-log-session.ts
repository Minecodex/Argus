import { useEffect, useState } from "react";
import {
  useApi,
  type KubernetesLogSession,
  type KubernetesLogSessionInput,
} from "@argus/api-client";

export function usePodLogSession(
  clusterId: string,
  input: KubernetesLogSessionInput,
  formatError: (cause: unknown) => string,
) {
  const api = useApi();
  const [activeInput, setActiveInput] = useState(input);
  const [session, setSession] = useState<KubernetesLogSession>();
  const [content, setContent] = useState("");
  const [error, setError] = useState("");
  const [observed, setObserved] = useState("");
  const [reading, setReading] = useState(false);
  const [complete, setComplete] = useState(false);
  const [truncated, setTruncated] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false,
      id: string | undefined,
      closed = false;
    let cursor: string | undefined,
      finished = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const controller = new AbortController();
    const close = () => {
      if (!id || closed) return;
      closed = true;
      void api.kubernetesWorkbench
        .closeLogSession(clusterId, id)
        .catch(() => undefined);
    };
    setActiveInput(input);
    setSession(undefined);
    setContent("");
    setError("");
    setObserved("");
    setComplete(false);
    setTruncated(false);
    setReading(Boolean(input.container));

    async function read() {
      if (cancelled || !id) return;
      setReading(true);
      try {
        if (finished) {
          await api.kubernetesWorkbench.getLogSession(
            clusterId,
            id,
            controller.signal,
          );
        } else {
          const batch = await api.kubernetesWorkbench.readLogSession(
            clusterId,
            id,
            cursor,
            controller.signal,
          );
          if (cancelled) return;
          cursor = batch.next_cursor;
          finished = batch.complete || batch.truncated;
          setContent((current) => {
            const next = current + batch.content;
            return next.length > 262144 ? next.slice(-262144) : next;
          });
          setObserved(batch.observed_at);
          setComplete(batch.complete);
          setTruncated(batch.truncated);
        }
        if (!cancelled) timer = setTimeout(() => void read(), 2000);
      } catch (cause) {
        if (!cancelled) {
          setContent("");
          setSession(undefined);
          setError(formatError(cause));
          close();
        }
      } finally {
        if (!cancelled) setReading(false);
      }
    }

    async function start() {
      if (cancelled || !input.container) return;
      let ticket: { ticket: string } | undefined;
      try {
        const created = await api.kubernetesWorkbench.createLogSession(
          clusterId,
          input,
        );
        ticket = created;
        id = created.session.id;
        if (cancelled) {
          close();
          return;
        }
        const attached = await api.kubernetesWorkbench.attachLogSession(
          clusterId,
          id,
          created.ticket,
        );
        created.ticket = "";
        if (cancelled) {
          close();
          return;
        }
        setSession(attached);
        await read();
      } catch (cause) {
        close();
        if (!cancelled) {
          setError(formatError(cause));
          setReading(false);
        }
      } finally {
        if (ticket) ticket.ticket = "";
      }
    }
    // A discarded StrictMode mount must not create an orphan session.
    void Promise.resolve().then(start);
    return () => {
      cancelled = true;
      controller.abort();
      if (timer) clearTimeout(timer);
      close();
    };
  }, [api, clusterId, input, attempt, formatError]);

  const current = activeInput === input;
  return {
    session: current ? session : undefined,
    content: current ? content : "",
    error: current ? error : "",
    observed: current ? observed : "",
    reading: !current || reading,
    complete: current && complete,
    truncated: current && truncated,
    retry: () => setAttempt((value) => value + 1),
  };
}
