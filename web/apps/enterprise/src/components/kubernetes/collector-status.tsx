import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { CollectorInstance, KubernetesCluster } from "@argus/api-client";
import { ActionMenu, Card, CardContent, CardHeader, Dialog } from "@argus/ui";
import { usePermission } from "../../lib/permissions";
import { BuiltinCollections } from "../monitoring/builtin-collections";
import { CollectorRuntimeSettings } from "./collector-runtime-settings";

/** Daily collection controls; deployment maintenance is a secondary action. */
export function CollectorStatusPanel({
  cluster,
  collector,
}: {
  cluster: KubernetesCluster;
  collector: CollectorInstance;
}) {
  const { t } = useTranslation();
  const canManage = usePermission("telemetry.collector.manage");
  const [settingsOpen, setSettingsOpen] = useState(false);
  return (
    <>
      <Card>
        <CardHeader
          title={t("builtinCollections.clusterTitle")}
          description={t("builtinCollections.clusterHint")}
          action={
            canManage && (
              <ActionMenu
                name={t("kubernetes.detail.collectorTab")}
                items={[
                  {
                    label: t("kubernetes.collector.settings.title"),
                    onSelect: () => setSettingsOpen(true),
                  },
                ]}
              />
            )
          }
        />
        <CardContent>
          <BuiltinCollections
            kind="kubernetes_cluster"
            id={cluster.id}
            showTitle={false}
          />
        </CardContent>
      </Card>
      {settingsOpen && (
        <Dialog
          open
          title={t("kubernetes.collector.settings.title")}
          onOpenChange={setSettingsOpen}
        >
          <CollectorRuntimeSettings
            clusterId={cluster.id}
            collector={collector}
          />
        </Dialog>
      )}
    </>
  );
}
