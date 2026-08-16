import { memo, useState, type ReactNode } from "react";
import type { Components } from "react-markdown";
import ReactMarkdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkGfm from "remark-gfm";
import { cn } from "@/lib/utils";
import { splitMarkdownBlocks } from "./split-markdown-blocks";

function FenceBlock({ children }: { children: ReactNode }) {
  const [copied, setCopied] = useState(false);
  function copy() {
    const text = typeof children === "string" ? children : extractText(children);
    if (!text || !navigator.clipboard?.writeText) {
      return;
    }
    void navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1400);
    });
  }
  return (
    <div className="relative min-w-0">
      <pre className="min-w-0 overflow-x-hidden whitespace-pre-wrap break-all rounded-lg bg-background p-3 pr-16 text-[13px] leading-6">
        {children}
      </pre>
      <button
        className="absolute right-2 top-2 rounded-md border border-line bg-card px-1.5 py-0.5 text-[11px] text-ink-2"
        onClick={copy}
        type="button"
      >
        {copied ? "已复制" : "复制"}
      </button>
    </div>
  );
}

function extractText(node: ReactNode): string {
  if (node == null || typeof node === "boolean") {
    return "";
  }
  if (typeof node === "string" || typeof node === "number") {
    return String(node);
  }
  if (Array.isArray(node)) {
    return node.map(extractText).join("");
  }
  if (typeof node === "object" && "props" in node) {
    return extractText((node as { props?: { children?: ReactNode } }).props?.children);
  }
  return "";
}

const markdownComponents: Components = {
  pre({ children }) {
    return <FenceBlock>{children}</FenceBlock>;
  },
  code({ className, children }) {
    const fenced = typeof className === "string" && className.startsWith("language-")
      || String(children).includes("\n");
    if (fenced) {
      return (
        <code className={cn("whitespace-pre-wrap break-all bg-transparent p-0 font-mono text-[0.85em] text-ink", className)}>
          {children}
        </code>
      );
    }
    return (
      <code className="rounded bg-background px-1 py-0.5 font-mono text-[0.85em] text-ink">
        {children}
      </code>
    );
  },
};

const proseClassName =
  "grid min-w-0 max-w-none gap-3 overflow-x-hidden break-words text-sm leading-6 text-ink [&_a]:text-brand [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-line [&_blockquote]:pl-3 [&_blockquote]:text-ink-2 [&_h1]:text-xl [&_h1]:font-bold [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:text-base [&_h3]:font-semibold [&_hr]:border-line [&_li]:my-0.5 [&_ol]:list-decimal [&_ol]:pl-5 [&_pre]:min-w-0 [&_table]:w-full [&_table]:border-collapse [&_td]:border [&_td]:border-line [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-line [&_th]:bg-background [&_th]:px-2 [&_th]:py-1 [&_ul]:list-disc [&_ul]:pl-5";

const FrozenMarkdownBlock = memo(function FrozenMarkdownBlock({ text }: { text: string }) {
  return (
    <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} components={markdownComponents}>
      {text}
    </ReactMarkdown>
  );
});

export const MarkdownProse = memo(function MarkdownProse({
  children,
  className,
  streaming = false,
}: {
  children: string;
  className?: string;
  streaming?: boolean;
}) {
  const blocks = streaming ? splitMarkdownBlocks(children) : null;
  return (
    <div className={cn(proseClassName, className)}>
      {blocks && blocks.length > 0 ? (
        blocks.map((block) =>
          block.frozen ? (
            <FrozenMarkdownBlock key={block.key} text={block.text} />
          ) : (
            <ReactMarkdown
              key={block.key}
              remarkPlugins={[remarkGfm, remarkBreaks]}
              components={markdownComponents}
            >
              {block.text}
            </ReactMarkdown>
          ),
        )
      ) : (
        <ReactMarkdown remarkPlugins={[remarkGfm, remarkBreaks]} components={markdownComponents}>
          {children}
        </ReactMarkdown>
      )}
    </div>
  );
});
