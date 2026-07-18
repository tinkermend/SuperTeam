import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import ReactMarkdown from "react-markdown";
import { Download } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { V3ErrorState, V3LoadingState } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import { buildApiUrl } from "@/lib/api/client";
import {
  getArtifactContentText,
  type ProjectArtifactRef,
} from "@/lib/api/projects";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export type ArtifactPreviewKind = "html" | "markdown" | "text";

/** 可预览的 content_type → 渲染方式;其余类型只提供下载。 */
export function artifactPreviewKind(
  artifact: ProjectArtifactRef,
): ArtifactPreviewKind | null {
  const contentType = (artifact.content_type ?? "").split(";")[0]?.trim();
  switch (contentType) {
    case "text/html":
      return "html";
    case "text/markdown":
      return "markdown";
    case "text/plain":
      return "text";
    default:
      return null;
  }
}

export function artifactContentHref(artifactId: string): string {
  return buildApiUrl(
    resolveControlPlaneUrl(),
    `/api/v1/artifacts/${encodeURIComponent(artifactId)}/content`,
  );
}

type ArtifactPreviewSheetProps = {
  /** 当前预览的工件;null 时关闭。 */
  artifact: ProjectArtifactRef | null;
  onClose: () => void;
};

export function ArtifactPreviewSheet({
  artifact,
  onClose,
}: ArtifactPreviewSheetProps) {
  return (
    <Sheet onOpenChange={(open) => (open ? undefined : onClose())} open={artifact != null}>
      <SheetContent
        className="flex w-[min(880px,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] flex-col gap-0 p-0 sm:max-w-[880px]"
        side="right"
      >
        {artifact ? <ArtifactPreviewBody artifact={artifact} /> : null}
      </SheetContent>
    </Sheet>
  );
}

function ArtifactPreviewBody({ artifact }: { artifact: ProjectArtifactRef }) {
  const kind = artifactPreviewKind(artifact);
  return (
    <>
      <SheetHeader className="border-b border-v3-line p-4">
        <SheetTitle className="truncate pr-8 text-v3-ink">
          {artifact.title}
        </SheetTitle>
        <SheetDescription className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-v3-ink-2">
          <span>数字员工原样产出,未经平台核实与脱敏</span>
          <a
            className="inline-flex items-center gap-1 font-medium text-v3-brand hover:underline"
            href={artifactContentHref(artifact.id)}
            rel="noreferrer"
            target="_blank"
          >
            <Download aria-hidden className="size-3.5" />
            下载原文件
          </a>
        </SheetDescription>
      </SheetHeader>
      <div className="min-h-0 flex-1 overflow-y-auto p-4">
        {kind != null ? (
          <ArtifactTextPreview artifact={artifact} kind={kind} />
        ) : (
          <V3ErrorState description="该类型不支持预览,请下载查看。" />
        )}
      </div>
    </>
  );
}

function ArtifactTextPreview({
  artifact,
  kind,
}: {
  artifact: ProjectArtifactRef;
  kind: ArtifactPreviewKind;
}) {
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: resolveControlPlaneUrl() }),
    [],
  );
  const contentQuery = useQuery({
    queryKey: ["artifact-content", artifact.id],
    queryFn: () => getArtifactContentText(apiOptions, artifact.id),
    staleTime: Infinity,
  });

  if (contentQuery.isLoading) {
    return <V3LoadingState />;
  }
  if (contentQuery.isError || contentQuery.data == null) {
    return (
      <V3ErrorState
        description="内容拉取失败(可能是对象存储跨域配置未放行),请用上方链接下载查看。"
        onRetry={() => void contentQuery.refetch()}
      />
    );
  }
  if (kind === "html") {
    // agent 产出的 HTML 等同不可信内容:内容经 fetch 拉回后以 srcDoc 注入
    // sandbox iframe(只放开脚本,绝不给 allow-same-origin),脚本运行在
    // opaque origin,拿不到平台 cookie/storage(spec §3)。不用 iframe src
    // 直连 302:对象存储对原始对象强制 Content-Disposition: attachment,
    // sandbox 内的下载会被静默拦截,表现为空白(E2E 2026-07-19 实测)。
    return (
      <iframe
        className="h-full min-h-[70vh] w-full rounded-[14px] border border-v3-line bg-white"
        sandbox="allow-scripts"
        srcDoc={contentQuery.data}
        title={artifact.title}
      />
    );
  }
  if (kind === "markdown") {
    // react-markdown 默认不渲染内嵌原始 HTML(转义输出),天然免 XSS。
    return (
      <div className="grid max-w-none gap-3 text-sm leading-6 text-v3-ink [&_a]:text-v3-brand [&_a]:underline [&_blockquote]:border-l-2 [&_blockquote]:border-v3-line [&_blockquote]:pl-3 [&_blockquote]:text-v3-ink-2 [&_code]:rounded [&_code]:bg-v3-bg [&_code]:px-1 [&_code]:font-mono [&_code]:text-[0.85em] [&_h1]:text-xl [&_h1]:font-bold [&_h2]:text-lg [&_h2]:font-semibold [&_h3]:text-base [&_h3]:font-semibold [&_hr]:border-v3-line [&_li]:my-0.5 [&_ol]:list-decimal [&_ol]:pl-5 [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:bg-v3-bg [&_pre]:p-3 [&_table]:w-full [&_td]:border [&_td]:border-v3-line [&_td]:px-2 [&_td]:py-1 [&_th]:border [&_th]:border-v3-line [&_th]:bg-v3-bg [&_th]:px-2 [&_th]:py-1 [&_ul]:list-disc [&_ul]:pl-5">
        <ReactMarkdown>{contentQuery.data}</ReactMarkdown>
      </div>
    );
  }
  return (
    <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-5 text-v3-ink">
      {contentQuery.data}
    </pre>
  );
}
