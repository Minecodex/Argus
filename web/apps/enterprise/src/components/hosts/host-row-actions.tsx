import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  formatApiError,
  type Host,
  type PendingActionPublic,
} from "@argus/api-client";
import { ActionMenu, Alert, Button, Dialog, Spinner } from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { PendingActionConfirm } from "./pending-action-confirm";
import { HostOnboardingProgress } from "./host-onboarding-progress";
import { activeCollectorProfileIds } from "../telemetry/collector-profiles";

type CollectorAction = "upgrade" | "repair" | "uninstall";

export function HostRowActions({
  host,
  hasCollector,
  onEdit,
  onDelete,
  onRemove,
  onCollectorAction,
}: {
  host: Host;
  hasCollector: boolean;
  onEdit: (host: Host) => void;
  onDelete: (host: Host) => void;
  onRemove: (host: Host) => void;
  onCollectorAction: (host: Host) => void;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const queryClient = useQueryClient();
  const canManageCollector = usePermission("telemetry.collector.manage");
  const [selected, setSelected] = useState<CollectorAction | null>(null);
  const [pending, setPending] = useState<PendingActionPublic | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const preview = async (action: CollectorAction) => {
    if (busy) return;
    setSelected(action);
    setPending(null);
    setError("");
    setBusy(true);
    try {
      // Read fresh versions when selected; opening the row menu is read-only.
      const [collector, claims, routes, distributions] = await Promise.all([
        api.hosts.getCollector(host.id),
        api.telemetry.listClaims(host.id),
        api.telemetry.listRoutes(),
        action === "upgrade" ? api.telemetry.listDistributions() : [],
      ]);
      const route = routes.find(
        (entry) => entry.collector_id === collector?.id,
      );
      const profiles = activeCollectorProfileIds(claims, collector?.id);
      if (!collector || !route)
        throw new Error(t("resourceList.collectorActionUnavailable"));
      const distribution = distributions.find(
        (item) =>
          item.support_status === "supported" &&
          item.artifacts.some(
            (artifact) => artifact.platform === collector.platform,
          ),
      );
      setPending(
        await api.hosts.previewCollectorAction(host.id, action, {
          distribution_version_id:
            distribution?.id ?? collector.distribution_version_id,
          profile_ids: profiles,
          route_kind: route.kind,
          transport: route.transport,
          loopback_port: route.loopback_port,
          gateway_collector_id: route.gateway_collector_id,
          expected_version: collector.version,
        }),
      );
    } catch (cause) {
      setError(
        formatApiError(
          cause,
          t("resourceList.collectorActionFailed"),
          (requestId) => t("common.requestReference", { requestId }),
        ),
      );
    } finally {
      setBusy(false);
    }
  };

  const close = () => {
    setSelected(null);
    setPending(null);
    setError("");
  };
  const changed = () => {
    close();
    for (const key of [
      "hosts",
      "host-collector",
      "telemetry",
      "tasks",
      "audit",
    ])
      void queryClient.invalidateQueries({ queryKey: [key] });
  };
  const collectorItems =
    hasCollector && canManageCollector && host.status === "active"
      ? (["upgrade", "repair", "uninstall"] as const).map((action) => ({
          label: t(
            `resourceList.collector${action === "upgrade" ? "Upgrade" : action === "repair" ? "Repair" : "Uninstall"}`,
          ),
          danger: action === "uninstall",
          onSelect: () => void preview(action),
        }))
      : [];

  return (
    <>
      <HostOnboardingProgress
        host={host}
        showSummary={false}
        renderActions={(onboardingItems) => (
          <ActionMenu
            name={host.name}
            isDisabled={busy}
            items={[
              { label: t("hosts.row.edit"), onSelect: () => onEdit(host) },
              {
                label: t("hosts.detail.tabComponents"),
                onSelect: () => onCollectorAction(host),
              },
              ...onboardingItems,
              ...collectorItems,
              "separator",
              host.status === "uninstalled"
                ? {
                    label: t("hosts.row.delete"),
                    danger: true,
                    onSelect: () => onDelete(host),
                  }
                : {
                    label: t("resourceList.hostUninstall"),
                    danger: true,
                    onSelect: () => onRemove(host),
                  },
            ]}
          />
        )}
      />
      <Dialog
        open={selected !== null}
        onOpenChange={(open) => {
          if (!open && !busy) close();
        }}
        dismissable={!busy}
        title={
          selected
            ? t(
                `resourceList.collector${selected === "upgrade" ? "Upgrade" : selected === "repair" ? "Repair" : "Uninstall"}`,
              )
            : ""
        }
      >
        {busy && <Spinner />}
        {error && (
          <>
            <Alert
              tone="danger"
              title={t("resourceList.collectorActionFailed")}
              description={error}
            />
            <Button onPress={() => selected && void preview(selected)}>
              {t("common.retry")}
            </Button>
          </>
        )}
        {pending && (
          <PendingActionConfirm
            action={pending}
            onCancel={close}
            onDismiss={close}
            onDone={changed}
          />
        )}
      </Dialog>
    </>
  );
}
