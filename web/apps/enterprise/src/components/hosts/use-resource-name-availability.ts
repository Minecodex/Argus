import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  apiErrorPresentation,
  formatApiError,
  formatErrorCode,
  type ResourceNameAvailability,
} from "@argus/api-client";

export type ResourceNameSnapshot = {
  readonly revision: number;
  readonly name: string;
};

/** Read-only feedback is advisory; each submit checks again before probing. */
export function useResourceNameAvailability({
  name,
  enabled,
  check,
}: {
  name: string;
  enabled: boolean;
  check: (name: string) => Promise<ResourceNameAvailability>;
}) {
  const { t } = useTranslation();
  const current = useRef({ name, enabled, revision: 0 });
  if (current.current.name !== name || current.current.enabled !== enabled) {
    current.current = { name, enabled, revision: current.current.revision + 1 };
  }
  const mounted = useRef(true);
  const pending = useRef<{
    revision: number;
    promise: Promise<boolean>;
  } | null>(null);
  const [feedback, setFeedback] = useState<{
    revision: number;
    error?: string;
    checking?: boolean;
  }>();
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      current.current.revision += 1;
    };
  }, []);
  const validRevision = (revision: number) =>
    mounted.current &&
    current.current.enabled &&
    current.current.revision === revision;
  const conflictMessage = formatErrorCode(
    "RESOURCE_NAME_CONFLICT",
    t("resourceNames.conflict"),
  );
  const requestReference = (requestId: string) =>
    t("common.requestReference", { requestId });

  const validate = (): Promise<boolean> => {
    const { revision, enabled: active, name: rawName } = current.current;
    if (!active) return Promise.resolve(false);
    const normalized = rawName.trim();
    if (!normalized || Array.from(normalized).length > 128)
      return Promise.resolve(false);
    if (pending.current?.revision === revision) return pending.current.promise;
    setFeedback({ revision, checking: true });
    const promise = (async () => {
      try {
        const result = await check(normalized);
        if (!validRevision(revision)) return false;
        setFeedback({
          revision,
          error: result.available ? undefined : conflictMessage,
        });
        return result.available;
      } catch (error) {
        if (validRevision(revision)) {
          setFeedback({
            revision,
            error: formatApiError(
              error,
              t("resourceNames.checkFailed"),
              requestReference,
            ),
          });
        }
        return false;
      } finally {
        if (pending.current?.revision === revision) pending.current = null;
      }
    })();
    pending.current = { revision, promise };
    return promise;
  };

  const capture = (): ResourceNameSnapshot => ({
    revision: current.current.revision,
    name: current.current.name,
  });
  const isCurrent = (snapshot: ResourceNameSnapshot) =>
    validRevision(snapshot.revision);
  const handleConflict = (error: unknown, snapshot: ResourceNameSnapshot) => {
    if (apiErrorPresentation(error)?.code !== "RESOURCE_NAME_CONFLICT")
      return false;
    if (isCurrent(snapshot)) {
      setFeedback({
        revision: current.current.revision,
        error: formatApiError(error, conflictMessage, requestReference),
      });
    }
    return true;
  };
  const reset = () => {
    current.current.revision += 1;
    pending.current = null;
    setFeedback(undefined);
  };
  const visible = enabled && feedback?.revision === current.current.revision;
  return {
    validate,
    capture,
    isCurrent,
    handleConflict,
    reset,
    onBlur: () => {
      void validate();
    },
    error: visible ? feedback.error : undefined,
    checking: visible && feedback.checking,
  };
}
