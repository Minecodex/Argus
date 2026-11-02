export type ChatTaskScope =
  | "general_diagnosis"
  | "dashboard_analysis"
  | "dashboard_authoring";

/** Read only the admitted enum from server metadata, never from message text. */
export function parseChatTaskScope(value: unknown): ChatTaskScope | undefined {
  switch (value) {
    case "general_diagnosis":
    case "dashboard_analysis":
    case "dashboard_authoring":
      return value;
    default:
      return undefined;
  }
}
