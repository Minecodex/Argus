import { Checkbox as HeroCheckbox } from "@heroui/react/checkbox";
import {
  useEffect,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { cx } from "./lib";

export type CheckboxProps = Omit<
  ComponentProps<typeof HeroCheckbox>,
  "children" | "className"
> & {
  children?: ReactNode;
  className?: string;
  appearance?: "default" | "outline";
  density?: "default" | "compact";
};
export function Checkbox({
  children,
  className,
  appearance = "default",
  density = "default",
  ...props
}: CheckboxProps) {
  if (appearance === "outline") {
    return (
      <OutlineCheckbox {...props} className={className} density={density}>
        {children}
      </OutlineCheckbox>
    );
  }
  return (
    <HeroCheckbox
      {...props}
      className={cx(
        "argus-checkbox",
        density === "compact" && "argus-checkbox--compact",
        className,
      )}
    >
      <HeroCheckbox.Content>
        <HeroCheckbox.Control>
          <HeroCheckbox.Indicator />
        </HeroCheckbox.Control>
        {children}
      </HeroCheckbox.Content>
    </HeroCheckbox>
  );
}

/** Native square controls preserve the platform's light/dark checked states. */
function OutlineCheckbox({
  children,
  className,
  density,
  ...props
}: CheckboxProps) {
  const input = useRef<HTMLInputElement>(null);
  const [localSelected, setLocalSelected] = useState(
    props.defaultSelected ?? false,
  );
  const selected = props.isSelected ?? localSelected;
  useEffect(() => {
    if (input.current)
      input.current.indeterminate = props.isIndeterminate ?? false;
  }, [props.isIndeterminate]);
  const control = (
    <input
      ref={input}
      type="checkbox"
      className={
        children == null
          ? cx("argus-checkbox--native-input", className)
          : undefined
      }
      id={props.id}
      name={props.name}
      value={props.value}
      checked={selected}
      disabled={props.isDisabled}
      readOnly={props.isReadOnly}
      required={props.isRequired}
      aria-label={props["aria-label"]}
      aria-labelledby={props["aria-labelledby"]}
      aria-describedby={props["aria-describedby"]}
      aria-invalid={props.isInvalid || undefined}
      onChange={(event) => {
        if (props.isReadOnly) return;
        setLocalSelected(event.target.checked);
        props.onChange?.(event.target.checked);
      }}
    />
  );
  if (children == null) return control;
  return (
    <label
      className={cx(
        "argus-checkbox",
        "argus-checkbox--native",
        density === "compact" && "argus-checkbox--compact",
        className,
      )}
    >
      {control}
      {children}
    </label>
  );
}
