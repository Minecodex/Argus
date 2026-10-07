import { Checkbox as HeroCheckbox } from "@heroui/react/checkbox";
import type { ComponentProps, ReactNode } from "react";
import { cx } from "./lib";

export type CheckboxProps = Omit<
  ComponentProps<typeof HeroCheckbox>,
  "children" | "className"
> & { children?: ReactNode; className?: string };
export function Checkbox({ children, className, ...props }: CheckboxProps) {
  return (
    <HeroCheckbox {...props} className={cx("argus-checkbox", className)}>
      <HeroCheckbox.Content>
        <HeroCheckbox.Control>
          <HeroCheckbox.Indicator />
        </HeroCheckbox.Control>
        {children}
      </HeroCheckbox.Content>
    </HeroCheckbox>
  );
}
