export function dashboardIssueMessage(
  t: (key: string) => string,
  issue: { code?: string; message: string },
) {
  return issue.code === "QUANTITY_DISPLAY_MISMATCH"
    ? t("dashboards.editor.quantityDisplayMismatch")
    : issue.message;
}
