import { useTranslation } from "react-i18next";
import { DataTable, Badge } from "@argus/ui";
import type { KubernetesDrainProgress } from "@argus/api-client";

export function KubernetesDrainProgressView({
  progress,
}: {
  progress: KubernetesDrainProgress;
}) {
  const { t } = useTranslation();
  return (
    <section aria-label={t("kubernetesWorkbench.drain.title")}>
      <p>
        {t("kubernetesWorkbench.drain.phase")}:{" "}
        {t(`kubernetesWorkbench.drain.phases.${progress.phase}`)}
      </p>
      <DataTable
        data={progress.pods}
        getRowKey={(row) => row.uid}
        columns={[
          {
            key: "namespace",
            header: t("kubernetesWorkbench.namespace"),
            render: (row) => row.namespace,
          },
          {
            key: "name",
            header: t("kubernetesWorkbench.name"),
            render: (row) => row.name,
          },
          {
            key: "state",
            header: t("kubernetesWorkbench.mutation.status"),
            render: (row) => (
              <Badge>
                {t(`kubernetesWorkbench.drain.states.${row.state}`)}
              </Badge>
            ),
          },
          {
            key: "reason",
            header: t("kubernetesWorkbench.drain.reason"),
            render: (row) =>
              row.reason
                ? t(`kubernetesWorkbench.drain.reasons.${row.reason}`, {
                    defaultValue: row.reason,
                  })
                : t("kubernetesWorkbench.drain.none"),
          },
          {
            key: "volumes",
            header: t("kubernetesWorkbench.drain.storage"),
            render: (row) =>
              row.volumes
                .map(
                  (v) =>
                    `${v.name}: ${v.kind}${v.claim ? ` / ${v.claim}` : ""}`,
                )
                .join(", ") || t("kubernetesWorkbench.drain.none"),
          },
          {
            key: "budgets",
            header: t("kubernetesWorkbench.drain.budgets"),
            render: (row) =>
              row.budgets
                .map((b) => `${b.name} / ${b.disruptions_allowed}`)
                .join(", ") || t("kubernetesWorkbench.drain.none"),
          },
        ]}
      />
    </section>
  );
}
