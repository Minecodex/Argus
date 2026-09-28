import { Dialog } from "@argus/ui";
import type { PendingActionPublic } from "@argus/api-client";
import { useTranslation } from "react-i18next";
import { PendingActionConfirm } from "../hosts/pending-action-confirm";
import { PublicationReview } from "./publication-review";
export function DashboardActionDialog({
  action,
  onClose,
  onDone,
  onError,
}: {
  action: PendingActionPublic;
  onClose: () => void;
  onDone: () => void;
  onError?: (error: unknown) => boolean;
}) {
  const { t } = useTranslation();
  return (
    <Dialog
      open
      onOpenChange={(open) => !open && onClose()}
      title={t(
        action.action_type === "telemetry.dashboard.publish"
          ? "dashboards.previewTitle"
          : action.action_type.startsWith("telemetry.dashboard.binding.")
            ? "dashboards.bindingPreviewTitle"
            : "dashboards.changePreviewTitle",
      )}
      description={action.title}
      width={1000}
    >
      {action.action_type === "telemetry.dashboard.publish" && (
        <PublicationReview action={action} />
      )}
      <PendingActionConfirm
        action={action}
        confirmLabel={
          action.action_type === "telemetry.dashboard.publish"
            ? t("dashboards.publish")
            : undefined
        }
        onDone={onDone}
        onCancel={onClose}
        onDismiss={onClose}
        onError={onError}
      />
    </Dialog>
  );
}
