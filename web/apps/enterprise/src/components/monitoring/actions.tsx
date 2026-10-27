import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatApiError,
  useApi,
  type MonitoringMutation,
  type PendingActionPublic,
} from "@argus/api-client";
import { Dialog } from "@argus/ui";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";

export function useMonitoringActions() {
  const api = useApi(),
    cache = useQueryClient(),
    { t } = useTranslation();
  const [pending, setPending] = useState<PendingActionPublic | null>(null);
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const preview = async (mutation: MonitoringMutation) => {
    setBusy(true);
    setError("");
    try {
      setPending(await api.monitoring.preview(mutation));
      return true;
    } catch (cause) {
      setError(
        formatApiError(cause, t("monitoring.errors.unavailable"), (requestId) =>
          t("common.requestReference", { requestId }),
        ),
      );
      return false;
    } finally {
      setBusy(false);
    }
  };
  const confirmation = pending ? (
    <Dialog
      open
      title={pending.title}
      onOpenChange={(open) => {
        if (!open) setPending(null);
      }}
    >
      <PendingActionConfirm
        confirmLabel={t("monitoring.confirmChanges")}
        action={pending}
        onDone={() => {
          setPending(null);
          void cache.invalidateQueries({ queryKey: ["monitoring"] });
        }}
        onCancel={() => setPending(null)}
        onDismiss={() => setPending(null)}
      />
    </Dialog>
  ) : null;
  return { preview, busy, error, confirmation };
}
