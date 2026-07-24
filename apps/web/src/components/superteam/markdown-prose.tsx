import ReactMarkdown from "react-markdown";
import { cn } from "@/lib/utils";

// react-markdown 默认不渲染内嵌原始 HTML(转义输出),天然免 XSS,
// 因此可直接承载数字员工/工件等不可信来源的文本。
export function MarkdownProse({ children, className }: { children: string; className?: string }) {
  return (
    <div
      className={cn(
        "grid max-w-none gap-3 text-sm leading-6 text-ink [&_a]:text-brand [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-line [&_blockquote]:pl-3 [&_blockquote]:text-ink-2 [&_code]:rounded [&_code]:bg-background [&_code]:px-1 [&_code]:font-mono [&_code]:text-[0.85em] [&_h1]:text-xl [&_h1]:font-bold [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:text-base [&_h3]:font-semibold [&_hr]:border-line [&_li]:my-0.5 [&_ol]:list-decimal [&_ol]:pl-5 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:bg-background [&_pre]:p-3 [&_table]:w-full [&_td]:border [&_td]:border-line [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-line [&_th]:bg-background [&_th]:px-2 [&_th]:py-1 [&_ul]:list-disc [&_ul]:pl-5",
        className,
      )}
    >
      <ReactMarkdown>{children}</ReactMarkdown>
    </div>
  );
}
