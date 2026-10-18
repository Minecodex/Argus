import { Download, FileText } from "lucide-react";
import { Button } from "./button";
import { Markdown } from "./markdown";
import { useUiText } from "./locale";

/** A versioned Markdown artifact. Execution belongs to the ordinary tool timeline. */
export function GuidancePlanCard({
  title,
  markdown,
  status,
  revision,
}: {
  title: string;
  markdown: string;
  status: string;
  revision: number;
}) {
  const text = useUiText();
  const download = () => {
    const url = URL.createObjectURL(
      new Blob([markdown], { type: "text/markdown;charset=utf-8" }),
    );
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `plan-v${revision}.md`;
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 0);
  };
  return (
    <article
      className="argus-guidance-plan argus-plan-document"
      aria-label={title}
    >
      <header className="argus-plan-document__header">
        <FileText aria-hidden size={16} />
        <span>{text("plan.md", "plan.md")}</span>
        <span role="status">{status}</span>
        <Button
          variant="ghost"
          isIconOnly
          aria-label={text("下载计划文件", "Download plan file")}
          onPress={download}
        >
          <Download aria-hidden size={14} />
        </Button>
      </header>
      <Markdown>{markdown}</Markdown>
    </article>
  );
}
