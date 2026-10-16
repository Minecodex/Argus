import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ListCollapse } from "lucide-react";
import { Button, ContentPopover } from "@argus/ui";
import { ConversationWorkspace } from "./conversation-workspace";

export function ConversationSummary(props: {
  conversationId: string;
  selected: string[];
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <ContentPopover
      open={open}
      onOpenChange={setOpen}
      title={t("chat.header.pinnedSummary")}
      trigger={
        <Button variant="ghost" className={open ? "is-active" : undefined}>
          <ListCollapse size={16} aria-hidden />
          {t("chat.header.pinnedSummary")}
        </Button>
      }
    >
      <ConversationWorkspace {...props} expanded />
    </ContentPopover>
  );
}
