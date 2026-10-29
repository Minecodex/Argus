import type { AlertInstance } from "@argus/api-client";
import { AlertCallout, useAlertText } from "./common";
import { queryHealthCause } from "./event-presentation";

export function AlertQueryHealth({ event }: { event: AlertInstance }) {
  const { t } = useAlertText();
  const cause = queryHealthCause(event);
  const description =
    cause === "no_data"
      ? "alerts.queryHealthNoDataHelp"
      : cause === "authorization_denied"
        ? "alerts.queryHealthDeniedHelp"
        : "alerts.queryHealthErrorHelp";
  return (
    <section className="argus-alert-drawer-section">
      <AlertCallout>
        <div>
          <p>{t(description)}</p>
          {event.lifecycle === "resolved" && (
            <p>{t("alerts.queryHealthRecoveredHelp")}</p>
          )}
        </div>
      </AlertCallout>
      <h3>{t("alerts.queryHealthQueries")}</h3>
      {event.state.policy_snapshot.rules.map((rule) => (
        <div key={rule.id}>
          <p>{rule.name}</p>
          <pre className="argus-alert-code-block">{rule.expression}</pre>
        </div>
      ))}
    </section>
  );
}
