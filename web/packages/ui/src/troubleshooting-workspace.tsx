import { useState, type ComponentProps } from "react";
import { Maximize2, Minimize2 } from "lucide-react";
import { Dialog } from "./primitives";
import { Button } from "./button";
import { useUiText } from "./locale";
export function TroubleshootingWorkspace({
  titleExtra,
  className,
  ...props
}: ComponentProps<typeof Dialog>) {
  const [expanded, setExpanded] = useState(false),
    text = useUiText();
  return (
    <Dialog
      {...props}
      size={expanded ? "cover" : "lg"}
      className={["argus-troubleshooting-workspace", className]
        .filter(Boolean)
        .join(" ")}
      titleExtra={
        <>
          {titleExtra}
          <Button
            isIconOnly
            variant="ghost"
            aria-label={
              expanded
                ? text("恢复工作区尺寸", "Restore workspace size")
                : text("展开排障工作区", "Expand troubleshooting workspace")
            }
            aria-pressed={expanded}
            onPress={() => setExpanded((value) => !value)}
          >
            {expanded ? <Minimize2 aria-hidden /> : <Maximize2 aria-hidden />}
          </Button>
        </>
      }
    />
  );
}
