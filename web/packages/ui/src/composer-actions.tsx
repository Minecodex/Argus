import type { ReactNode } from "react";
import { Dropdown, Header, Label, Description } from "@heroui/react";
import { Check } from "lucide-react";
export interface ComposerActionItem {
  id: string;
  label: string;
  description?: string;
  icon?: ReactNode;
  selected?: boolean;
  disabled?: boolean;
  onSelect(): void;
}
export function ComposerActions({
  trigger,
  label,
  sections,
}: {
  trigger: ReactNode;
  label: string;
  sections: { label: string; items: ComposerActionItem[] }[];
}) {
  const actions = new Map(
    sections.flatMap((s) => s.items).map((item) => [item.id, item]),
  );
  return (
    <Dropdown>
      {trigger}
      <Dropdown.Popover
        placement="top start"
        className="argus-composer-actions"
      >
        <Dropdown.Menu
          aria-label={label}
          onAction={(key) => {
            const item = actions.get(String(key));
            if (item && !item.disabled) item.onSelect();
          }}
        >
          {sections.map((section) => (
            <Dropdown.Section key={section.label} aria-label={section.label}>
              <Header className="argus-composer-actions__heading">
                {section.label}
              </Header>
              {section.items.map((item) => (
                <Dropdown.Item
                  id={item.id}
                  key={item.id}
                  textValue={item.label}
                  isDisabled={item.disabled}
                  className="argus-composer-actions__item"
                >
                  {item.icon}
                  <div className="argus-composer-actions__copy">
                    <Label>{item.label}</Label>
                    {item.description && (
                      <Description>{item.description}</Description>
                    )}
                  </div>
                  {item.selected && (
                    <Check
                      aria-hidden
                      className="argus-composer-actions__check"
                    />
                  )}
                </Dropdown.Item>
              ))}
            </Dropdown.Section>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}
