import { useState } from "react";
import { Textarea, type TextareaProps } from "./form";
import { Button } from "./button";
import { useUiText } from "./locale";
import { cx } from "./lib";

/** Multiline private input remains local to its owning form. */
export function ProtectedTextarea(props: TextareaProps) {
  const [visible, setVisible] = useState(false),
    text = useUiText();
  return (
    <div className="argus-protected-textarea">
      <Textarea
        {...props}
        autoComplete="off"
        spellCheck={false}
        className={cx(
          props.className,
          !visible && "argus-protected-textarea--hidden",
        )}
      />
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onPress={() => setVisible((v) => !v)}
      >
        {visible
          ? text("隐藏内容", "Hide value")
          : text("显示内容", "Show value")}
      </Button>
    </div>
  );
}
