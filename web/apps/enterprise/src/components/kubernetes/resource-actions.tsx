import type { KubernetesInventoryObject } from "@argus/api-client";
import { ActionMenu } from "@argus/ui";
import { useTranslation } from "react-i18next";
import { usePermission } from "../../lib/permissions";
export function ResourceActions({
  object,
  onAction,
  onCopyLink,
}: {
  object: KubernetesInventoryObject;
  onAction: (action: string, object: KubernetesInventoryObject) => void;
  onCopyLink?: () => void;
}) {
  const { t } = useTranslation(),
    manage = usePermission(object.definition.manage_permission ?? ""),
    logs = usePermission("kubernetes.operational_logs.read"),
    exec = usePermission("kubernetes.exec"),
    drain = usePermission("kubernetes.node.drain");
  const actions = (object.actions ?? []).filter((action) =>
    action.action === "logs"
      ? logs
      : action.action === "exec"
        ? exec
        : action.action === "drain"
          ? drain
          : manage,
  );
  return (
    <ActionMenu
      name={object.name}
      description={
        actions.some((action) => action.state === "requires_check")
          ? t("kubernetesResources.actionPreflightNote")
          : undefined
      }
      items={[
        ...(onCopyLink
          ? [{ label: t("kubernetesResources.copyLink"), onSelect: onCopyLink }]
          : []),
        ...actions.map((action) => ({
          label: t("kubernetesResources.actions." + action.action),
          disabled: action.state === "unavailable",
          description: action.reason
            ? t("kubernetesResources.reasons." + action.reason)
            : undefined,
          danger: action.action === "delete",
          onSelect: () => onAction(action.action, object),
        })),
      ]}
    />
  );
}
