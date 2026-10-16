import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";

/** The optional parser stays outside the login and initial portal bundle. */
export default function MarkdownContent({ children }: { children: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      skipHtml
      urlTransform={defaultUrlTransform}
      components={{
        a: ({ children: text, href }) => (
          <a
            href={href}
            target={href?.startsWith("http") ? "_blank" : undefined}
            rel="noopener noreferrer"
          >
            {text}
          </a>
        ),
        img: ({ alt, src }) => (
          <a href={src} target="_blank" rel="noopener noreferrer">
            {alt || src}
          </a>
        ),
        table: ({ children: rows }) => (
          <div className="argus-markdown__table">
            <table>{rows}</table>
          </div>
        ),
      }}
    >
      {children}
    </ReactMarkdown>
  );
}
