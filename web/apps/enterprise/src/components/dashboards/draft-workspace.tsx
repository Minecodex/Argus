import {
  Outlet,
  useBlocker,
  useParams,
  useNavigate,
} from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Button, PageShell, QueryBoundary } from "@argus/ui";
import { useDashboardDraft } from "./use-dashboard-draft";
import { DashboardDraftContext } from "./draft-workspace-context";
export function DashboardDraftWorkspace() {
  const { draftId } = useParams({
    from: "/authed/admin/dashboard-drafts/$draftId",
  });
  return <OwnedDraftWorkspace key={draftId} draftId={draftId} />;
}

function OwnedDraftWorkspace({ draftId }: { draftId: string }) {
  const editor = useDashboardDraft(draftId);
  const { t } = useTranslation(),
    navigate = useNavigate();
  const root = `/dashboard-drafts/${draftId}`;
  useBlocker({
    shouldBlockFn: async ({ next }) => {
      if (
        next.pathname === root ||
        next.pathname.startsWith(root + "/") ||
        !editor.dirty
      )
        return false;
      try {
        await editor.save();
        return false;
      } catch {
        return true;
      }
    },
    enableBeforeUnload: () => editor.dirty,
  });
  if (!editor.draft)
    return (
      <PageShell
        title={t("dashboards.draft")}
        actions={
          <Button onPress={() => void navigate({ to: "/dashboards" })}>
            {t("dashboards.back")}
          </Button>
        }
      >
        <QueryBoundary
          query={{
            isPending: editor.loading,
            error: editor.error,
            refetch: editor.reload,
          }}
        >
          <></>
        </QueryBoundary>
      </PageShell>
    );
  return (
    <DashboardDraftContext.Provider value={editor}>
      <Outlet />
    </DashboardDraftContext.Provider>
  );
}
