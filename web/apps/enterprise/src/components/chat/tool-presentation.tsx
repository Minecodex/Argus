import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi } from "@argus/api-client";
import { Dialog, ToolPresentationFrame, useTheme } from "@argus/ui";
import { runtimeConfig } from "../../lib/runtime-config";

export function ConversationPresentation({
  conversationId,
  toolCallId,
}: {
  conversationId: string;
  toolCallId: string;
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
    staleTime: 0,
  });
  const detail = useQuery<unknown>({
    queryKey: ["stored-tool-detail", opened],
    queryFn: () =>
      opened?.kind === "connector"
        ? api.connectors.get(opened.ref)
        : api.presentations.result(opened!.ref),
    enabled: !!opened,
  });
  if (query.isError) return <p role="alert">{t("planv5.template.failed")}</p>;
  if (!query.data) return <p role="status">{t("planv5.template.loading")}</p>;
  const origin =
    runtimeConfig().templateOrigin ?? import.meta.env.VITE_TEMPLATE_ORIGIN;
  if (!origin) return <p role="alert">{t("planv5.template.failed")}</p>;
  return (
    <>
      <ToolPresentationFrame
        origin={origin}
        presentation={query.data}
        locale={i18n.language === "en-US" ? "en-US" : "zh-CN"}
        colorScheme={resolvedTheme}
        title={t("planv5.template.title")}
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
