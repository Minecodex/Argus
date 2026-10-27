import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import { Button, Dialog, ToolPresentationFrame, useTheme } from "@argus/ui";
import { LoaderCircle } from "lucide-react";
import { runtimeConfig } from "../../lib/runtime-config";
import { toolDisplayName } from "./tool-name";
import { MonitoringHostForm } from "../monitoring/host-form";

export function ConversationPresentation({
  conversationId,
  toolCallId,
  toolName,
  source,
}: {
  conversationId: string;
  toolCallId: string;
  toolName?: string;
  source?: string;
}) {
  const navigate = useNavigate();
  const [opened, setOpened] = useState<{
    kind: "result" | "connector";
    ref: string;
  } | null>(null);
  const api = useApi();
  const { t, i18n } = useTranslation();
  const { resolvedTheme } = useTheme();
  const query = useQuery({
    queryKey: ["tool-presentation", conversationId, toolCallId],
    queryFn: () => api.presentations.get(conversationId, toolCallId),
    staleTime: 15_000,
  });
  const detail = useQuery<unknown>({
    queryKey: ["stored-tool-detail", opened],
    queryFn: () =>
      opened?.kind === "connector"
        ? api.connectors.get(opened.ref)
        : api.presentations.result(opened!.ref),
    enabled: !!opened,
  });
  if (query.isError)
    return (
      <div className="argus-tool-presentation__placeholder" role="alert">
        <p>{t("planv5.template.failed")}</p>
        <Button onPress={() => void query.refetch()}>
          {t("planv5.template.retry")}
        </Button>
      </div>
    );
  if (!query.data)
    return (
      <div className="argus-tool-presentation__placeholder" role="status">
        <LoaderCircle className="argus-spin" aria-hidden size={24} />
        <p>{t("planv5.template.loading")}</p>
      </div>
    );
  // This descriptor comes from the sealed, reauthorized native presentation.
  // Private fields are rendered by the trusted host, outside tool iframes.
  if (
    source === "argus" &&
    query.data.status === "ready" &&
    query.data.detail_data.tool === "monitoring.collection.form" &&
    query.data.detail_data.schema_version === "argus.monitoring_host_form/v1"
  )
    return <MonitoringHostForm data={query.data.detail_data} />;
  const origin =
    runtimeConfig().templateOrigin ?? import.meta.env.VITE_TEMPLATE_ORIGIN;
  if (!origin) return <p role="alert">{t("planv5.template.failed")}</p>;
  const displayName = toolName
    ? toolDisplayName(t, toolName, source)
    : t("planv5.template.title");
  return (
    <>
      <ToolPresentationFrame
        layout="document"
        origin={origin}
        presentation={query.data}
        locale={i18n.language === "en-US" ? "en-US" : "zh-CN"}
        colorScheme={resolvedTheme}
        title={displayName}
        onRead={(input) =>
          api.presentations.read(conversationId, toolCallId, input)
        }
        dataVersion={query.dataUpdatedAt}
        retryLabel={t("planv5.template.retry")}
        loadingLabel={t("planv5.template.loading")}
        failureLabel={t("planv5.template.failed")}
        expandLabel={t("planv5.template.expand")}
        collapseLabel={t("planv5.template.collapse")}
        onResource={(ref) => {
          if (ref.type === "host")
            void navigate({ to: "/hosts/$hostId", params: { hostId: ref.id } });
          else if (ref.type === "kubernetes_cluster")
            void navigate({
              to: "/kubernetes/$clusterId",
              params: { clusterId: ref.id },
            });
          else if (ref.type === "connector")
            setOpened({ kind: "connector", ref: ref.id });
          else if (ref.type === "application")
            void navigate({
              to: "/applications/$applicationId",
              params: { applicationId: ref.id },
            });
        }}
        onResult={(ref) => setOpened({ kind: "result", ref })}
      />
      <Dialog
        open={!!opened}
        onOpenChange={(open) => !open && setOpened(null)}
        title={t("planv5.template.title")}
      >
        {detail.isError ? (
          <p role="alert">{t("planv5.template.failed")}</p>
        ) : (
          <pre>
            {detail.data
              ? JSON.stringify(detail.data, null, 2)
              : t("planv5.template.loading")}
          </pre>
        )}
      </Dialog>
    </>
  );
}
