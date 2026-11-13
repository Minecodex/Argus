import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useApi,
  type KubernetesInventoryObject,
  type KubernetesResourceReference,
} from "@argus/api-client";
import { Alert, Button } from "@argus/ui";
export function KubernetesHPANotice({
  clusterId,
  object,
  namespaceId,
  onOpen,
}: {
  clusterId: string;
  object: KubernetesInventoryObject;
  namespaceId?: string;
  onOpen?: (reference: KubernetesResourceReference) => void;
}) {
  const api = useApi(),
    { t } = useTranslation();
  const query = useQuery({
    queryKey: ["kubernetes-workbench", clusterId, "hpa-relations", object.uid],
    queryFn: ({ signal }) =>
      api.kubernetesWorkbench.snapshot(
        clusterId,
        {
          namespace_mode: namespaceId ? "explicit" : "all_authorized",
          namespace_ids: namespaceId ? [namespaceId] : undefined,
          types: ["hpa", object.definition.key],
          max_items: 1000,
        },
        signal,
      ),
    enabled: ["deployment", "statefulset", "replicaset"].includes(
      object.definition.key,
    ),
  });
  const ids = new Set(
    query.data?.relations
      .filter((edge) => edge.kind === "scales" && edge.to === object.uid)
      .map((edge) => edge.from),
  );
  const items = query.data?.resources.filter((row) => ids.has(row.uid)) ?? [];
  if (query.error)
    return (
      <Alert
        tone="warning"
        title={t("kubernetesResources.relations")}
        description={t("kubernetesResources.notAvailable")}
      />
    );
  if (!items.length)
    return query.data && !query.data.complete ? (
      <Alert
        tone="warning"
        title={t("kubernetesResources.relations")}
        description={t("kubernetesResources.partial")}
      />
    ) : null;
  return (
    <div>
      <Alert
        tone="warning"
        title={t("kubernetesResources.hpaTitle")}
        description={t("kubernetesResources.hpaNotice")}
      />
      {items.map((row) =>
        onOpen ? (
          <Button key={row.uid} variant="ghost" onPress={() => onOpen(row)}>
            {row.name}
          </Button>
        ) : (
          <p key={row.uid}>{row.name}</p>
        ),
      )}
    </div>
  );
}
