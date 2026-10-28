import { useState, type ReactNode } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, type Host } from "@argus/api-client";
import {
  ActionMenu,
  Alert,
  Dialog,
  OperationPanel,
  Spinner,
  type DropdownItem,
} from "@argus/ui";
import { operationTone } from "../operations/execution-result";
import { AddHostWizard } from "./add-host-wizard";
import { onboardingFailureMessageKey as failureMessageKey } from "./onboarding-errors";

export function HostOnboardingProgress({
  host,
  renderActions,
  showSummary = true,
  showActions = true,
}: {
  host: Host;
  renderActions?: (items: DropdownItem[]) => ReactNode;
  showSummary?: boolean;
  showActions?: boolean;
}) {
  const { t } = useTranslation();
  const api = useApi();
  const [open, setOpen] = useState(false);
  const [retryOpen, setRetryOpen] = useState(false);
  const queryClient = useQueryClient();
  const scopes = useQuery({
    queryKey: ["bastion-scopes"],
    queryFn: () => api.connectors.listBastionScopes(),
    enabled: retryOpen,
  });
  const id = host.onboarding.operation_id;
  const active =
    host.status === "active" &&
    ["installing", "install_failed"].includes(host.onboarding.state);
  const query = useQuery({
    queryKey: ["host-onboarding-operation", id],
    queryFn: () => api.hosts.getOnboardingOperation(id!),
    enabled: active && Boolean(id),
    refetchInterval: (q) =>
      active &&
      (!q.state.data || ["queued", "running"].includes(q.state.data.status))
        ? 2000
        : false,
  });
  if (!active || !id) return renderActions?.([]) ?? null;
  const operation = query.data;
  const code = operation?.error_code ?? host.onboarding.error_code;
  const failureMessage = t(failureMessageKey(code));
  const actions: DropdownItem[] = [
    {
      label: t("hosts.onboardingProgress.view"),
      onSelect: () => setOpen(true),
    },
    ...(host.onboarding.state === "install_failed" &&
    !host.connector_id &&
    (operation?.install_method === "ssh" ||
      host.onboarding.install_method === "ssh")
      ? [
          {
            label: t("hosts.onboardingProgress.retry"),
            onSelect: () => {
              setOpen(false);
              setRetryOpen(true);
            },
          },
        ]
      : []),
  ];
  return (
    <>
      {showSummary && host.onboarding.state === "install_failed" && (
        <p className="argus-muted">
          <span>{failureMessage}</span>{" "}
          {code && (
            <span>{t("hosts.onboardingProgress.errorCode", { code })}</span>
          )}
        </p>
      )}
      {showActions &&
        (renderActions ? (
          renderActions(actions)
        ) : (
          <ActionMenu name={host.name} items={actions} />
        ))}
      {retryOpen && (
        <AddHostWizard
          open={retryOpen}
          retryHost={host}
          scopes={scopes.data?.items ?? []}
          onOpenChange={setRetryOpen}
          onCreated={() => {
            setRetryOpen(false);
            void queryClient.invalidateQueries({ queryKey: ["hosts"] });
          }}
        />
      )}
      <Dialog
        title={t("hosts.onboardingProgress.view")}
        open={open}
        onOpenChange={setOpen}
        width={680}
      >
        {query.isError && (
          <Alert
            tone="danger"
            title={t("hosts.onboardingProgress.loadFailed")}
            description={t("hosts.onboardingProgress.failure")}
          />
        )}
        {query.isPending && <Spinner label={t("common.loading")} />}
        {operation && (
          <OperationPanel
            title={host.name}
            description={t(
              `hosts.onboardingProgress.stages.${operation.stage}`,
            )}
            statusLabel={t(`hosts.bastionOperationStatus.${operation.status}`)}
            tone={operationTone(operation.status)}
            busy={["queued", "running"].includes(operation.status)}
            steps={operation.events.map((event) => ({
              title: t(`hosts.onboardingProgress.stages.${event.stage}`),
              status:
                event.status === "failed"
                  ? "danger"
                  : event.status === "succeeded"
                    ? "done"
                    : "current",
              meta: event.error_code
                ? <><span>{t(failureMessageKey(event.error_code))}</span>{" "}<span>{t("hosts.onboardingProgress.errorCode", { code: event.error_code })}</span></>
                : t(`hosts.bastionOperationEventStatus.${event.status}`),
            }))}
          >
            {code && (
              <Alert
                tone="danger"
                title={failureMessage}
                description={t("hosts.onboardingProgress.errorCode", { code })}
              />
            )}
          </OperationPanel>
        )}
      </Dialog>
    </>
  );
}
