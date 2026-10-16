import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useState } from "react";
import {
  useApi,
  formatApiError,
  type Host,
  type PendingActionPublic,
} from "@argus/api-client";
import { Alert, Button, Dialog } from "@argus/ui";
import { PendingActionConfirm } from "./pending-action-confirm";

export function HostReconnectDialog({
  host,
  onClose,
}: {
  host: Host;
  onClose: () => void;
}) {
  const { t } = useTranslation(),
    api = useApi(),
    cache = useQueryClient();
  const [action, setAction] = useState<PendingActionPublic>();
  const preview = useMutation({
    mutationFn: () =>
      api.hosts.previewReconnect(host.id, host.resource_version),
    onSuccess: setAction,
  });
  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("hostInventory.reconnect")}
      description={t("hostInventory.reconnectHint")}
    >
      {preview.isError && (
        <Alert
          tone="danger"
          title={t("hostInventory.reconnectFailed")}
          description={formatApiError(
            preview.error,
            t("hostInventory.reconnectFailed"),
            (requestId) => t("common.requestReference", { requestId }),
          )}
        />
      )}
      {action ? (
        <PendingActionConfirm
          action={action}
          onDone={() => {
            void cache.invalidateQueries({ queryKey: ["hosts"] });
            void cache.invalidateQueries({
              queryKey: ["telemetry", "collectors"],
            });
            onClose();
          }}
        />
      ) : (
        <Button
          variant="primary"
          isPending={preview.isPending}
          onPress={() => preview.mutate()}
        >
          {t(
            preview.isPending
              ? "hostInventory.verifyingConnection"
              : "hostInventory.verifyReconnect",
          )}
        </Button>
      )}
    </Dialog>
  );
}
