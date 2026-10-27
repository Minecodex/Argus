import { useAlertText, datetime } from "./common";

type Event = {
  id: string;
  event_type: string;
  created_at: string;
  origin: string;
  actor_id?: string;
  payload: Record<string, unknown>;
};
const object = (v: unknown): Record<string, unknown> =>
  v !== null && typeof v === "object" && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};

/** Render each event's own snapshot, rather than today's instance or raw storage JSON. */
export function AlertEventTimeline({
  events,
  lastEvaluatedAt,
  userNames,
}: {
  events: Event[];
  lastEvaluatedAt?: string;
  userNames?: Map<string, string>;
}) {
  const { t } = useAlertText();
  return (
    <ol className="argus-alert-timeline">
      {events.map((entry) => {
        const state = object(entry.payload.state);
        const actor = entry.actor_id
          ? (userNames?.get(entry.actor_id) ?? entry.actor_id)
          : t(
              entry.origin === "evaluator"
                ? "alerts.system"
                : "alerts.automatedAction",
            );
        const known = [
          "first",
          "recovery",
          "repeat",
          "severity",
          "acknowledge",
          "assign",
          "comment",
          "source_changed",
          "resource_deleted",
          "muted",
          "unmuted",
          "inhibited",
          "uninhibited",
        ].includes(entry.event_type);
        return (
          <li key={entry.id}>
            <strong>
              {known
                ? t(`alerts.${entry.event_type}`)
                : t("alerts.eventUpdated")}
            </strong>
            <p className="argus-alert-secondary">
              <time dateTime={entry.created_at}>
                {datetime(entry.created_at)}
              </time>{" "}
              · {actor}
            </p>
            {typeof state.value === "number" && (
              <p>
                {t("alerts.currentValue")}: {state.value}
                {String(state.unit ?? "")}
                {typeof state.threshold === "number" &&
                  ` / ${state.threshold}${String(state.unit ?? "")}`}
              </p>
            )}
            {typeof entry.payload.severity === "string" && (
              <p>
                {t("alerts.severity")}: {t(`alerts.${entry.payload.severity}`)}
              </p>
            )}
            {typeof entry.payload.owner_id === "string" && (
              <p>
                {t("alerts.owner")}:{" "}
                {userNames?.get(entry.payload.owner_id) ??
                  entry.payload.owner_id}
              </p>
            )}
            {typeof entry.payload.comment === "string" && (
              <p className="argus-alert-event-comment">
                {entry.payload.comment}
              </p>
            )}
            {typeof state.error_code === "string" && (
              <p>
                {t("alerts.health")}: {state.error_code}
              </p>
            )}
          </li>
        );
      })}
      {lastEvaluatedAt && (
        <li>
          <strong>{t("alerts.lastEval")}</strong>
          <p className="argus-alert-secondary">
            <time dateTime={lastEvaluatedAt}>{datetime(lastEvaluatedAt)}</time>
          </p>
        </li>
      )}
      {!events.length && !lastEvaluatedAt && <li>{t("alerts.noData")}</li>}
    </ol>
  );
}
