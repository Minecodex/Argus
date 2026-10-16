import { MoreHorizontal } from "lucide-react";
import { Button } from "./button";
import { Dropdown, type DropdownItem } from "./primitives";
import { useUiText } from "./locale";

/** One consistent entry point for secondary and destructive entity actions. */
export function ActionMenu({
  items,
  label,
  name,
  isDisabled,
}: {
  items: DropdownItem[];
  label?: string;
  name?: string;
  isDisabled?: boolean;
}) {
  const text = useUiText();
  if (!items.some((item) => item !== "separator")) return null;
  const accessibleLabel =
    label ??
    (name
      ? text(`${name} · 更多操作`, `${name} · More actions`)
      : text("更多操作", "More actions"));
  return (
    <Dropdown
      items={items}
      trigger={
        <Button
          isIconOnly
          isDisabled={isDisabled}
          size="sm"
          variant="ghost"
          aria-label={accessibleLabel}
          title={accessibleLabel}
        >
          <MoreHorizontal aria-hidden />
        </Button>
      }
    />
  );
}
