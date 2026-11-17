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
  const described = sections.some((section) =>
    section.items.some((item) => !!item.description),
  );
  return (
    <Dropdown>
      {trigger}
      <Dropdown.Popover
        placement="top start"
        className={`argus-composer-actions${described ? " argus-composer-actions--described" : ""}`}
      >
        <Dropdown.Menu
          aria-label={label}
          className="argus-composer-actions__menu"
          onAction={(key) => {
            const item = actions.get(String(key));
            if (item && !item.disabled) item.onSelect();
          }}
        >
          {sections.map((section) => (
            <Dropdown.Section
              key={section.label}
              aria-label={section.label}
              className="argus-composer-actions__section"
            >
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
                  {item.icon && (
                    <span aria-hidden className="argus-composer-actions__icon">
                      {item.icon}
                    </span>
                  )}
                  <div className="argus-composer-actions__copy">
                    <Label className="argus-composer-actions__label">
                      {item.label}
                    </Label>
                    {item.description && (
                      <Description className="argus-composer-actions__description">
                        {item.description}
                      </Description>
                    )}
                  </div>
                  <span aria-hidden className="argus-composer-actions__check">
                    {item.selected && <Check />}
                  </span>
                </Dropdown.Item>
              ))}
            </Dropdown.Section>
          ))}
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  );
}
