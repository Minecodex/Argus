import { Button } from "./button";
export function ConversationListItem({
  title,
  meta,
  selected,
  onPress,
}: {
  title: string;
  meta: string;
  selected: boolean;
  onPress: () => void;
}) {
  return (
    <Button
      variant="ghost"
      layout="content"
      className={`argus-conversation-item${selected ? " is-selected" : ""}`}
      onPress={onPress}
      aria-current={selected ? "page" : undefined}
    >
      <span className="argus-conversation-item__layout">
        <span className="argus-conversation-item__content">
          <span title={title}>{title}</span>
          <small>{meta}</small>
        </span>
      </span>
    </Button>
  );
}
