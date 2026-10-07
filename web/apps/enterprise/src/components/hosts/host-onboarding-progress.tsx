import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useApi, type Host } from "@argus/api-client";
import { Alert, Badge, Button, Dialog } from "@argus/ui";
import { AddHostWizard } from "./add-host-wizard";
import { onboardingFailureMessageKey as failureMessageKey } from "./onboarding-errors";

export function HostOnboardingProgress({ host }: { host: Host }) {
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
  if (!active || !id) return null;
  const operation = query.data;
  const code = operation?.error_code ?? host.onboarding.error_code;
  const failureMessage = t(failureMessageKey(code));
  return (
    <>
      {host.onboarding.state === "install_failed" && (
        <p className="argus-muted">
          <span>{failureMessage}</span>{" "}
          {code && (
            <span>{t("hosts.onboardingProgress.errorCode", { code })}</span>
          )}
        </p>
      )}
      <Button size="sm" variant="secondary" onPress={() => setOpen(true)}>
        {t("hosts.onboardingProgress.view")}
      </Button>
      {host.onboarding.state === "install_failed" &&
        !host.connector_id &&
        (operation?.install_method === "ssh" ||
          host.onboarding.install_method === "ssh") && (
          <Button
            size="sm"
            variant="secondary"
            onPress={() => {
              setOpen(false);
              setRetryOpen(true);
            }}
          >
            {t("hosts.onboardingProgress.retry")}
          </Button>
        )}
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
        {operation && (
          <div className="argus-operation-progress">
            <div className="argus-operation-progress__head">
              <strong>
                {t(`hosts.onboardingProgress.stages.${operation.stage}`)}
              </strong>
              <Badge
                tone={
                  operation.status === "failed" ||
                  operation.status === "expired"
                    ? "danger"
                    : "info"
                }
              >
                {t(`hosts.bastionOperationStatus.${operation.status}`)}
              </Badge>
            </div>
            {code && (
              <Alert
                tone="danger"
                title={failureMessage}
                description={t("hosts.onboardingProgress.errorCode", { code })}
              />
            )}
            <ol className="argus-operation-timeline">
              {operation.events.map((event) => (
                <li key={event.id} className={`is-${event.status}`}>
                  <strong>
                    {t(`hosts.onboardingProgress.stages.${event.stage}`)}
                  </strong>
                  <span>
                    {event.error_code ? (
                      <>
                        <span>{t(failureMessageKey(event.error_code))}</span>{" "}
                        <span>
                          {t("hosts.onboardingProgress.errorCode", {
                            code: event.error_code,
                          })}
                        </span>
                      </>
                    ) : (
                      t(`hosts.bastionOperationEventStatus.${event.status}`)
                    )}
                  </span>
                </li>
              ))}
            </ol>
          </div>
        )}
      </Dialog>
    </>
  );
}
