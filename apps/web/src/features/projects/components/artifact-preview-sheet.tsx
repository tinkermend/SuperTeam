import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { Download } from "lucide-react";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from "@/components/ui/sheet";
import { MarkdownProse, ErrorState, LoadingState } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import { buildApiUrl } from "@/lib/api/client";
import { getArtifactContentText } from "@/lib/api/projects";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

export type ArtifactPreviewKind = "html" | "markdown" | "text";

/**
 * 预览 Sheet 需要的最小工件形状:content 端点按 id 取回,只用 id/title/
 * content_type。工件面板传 ProjectArtifactRef(超集),验收判据卡传交付物
 * 血缘(v2 §4 P2),都满足此接口,不必耦合完整工件类型。
 */
export type PreviewableArtifact = {
  id: string;
  title: string;
  content_type?: string | null;
};

/** 可预览的 content_type → 渲染方式;其余类型只提供下载。 */
export function artifactPreviewKind(
  artifact: PreviewableArtifact,
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
  artifact: PreviewableArtifact | null;
  onClose: () => void;
};

export function ArtifactPreviewSheet({
  artifact,
  onClose
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

function ArtifactPreviewBody({ artifact }: { artifact: PreviewableArtifact }) {
  const kind = artifactPreviewKind(artifact);
  return (
    <>
      <SheetHeader className="border-b border-line p-4">
        <SheetTitle className="truncate pr-8 text-ink">
          {artifact.title}
        </SheetTitle>
        <SheetDescription className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-2">
          <span>数字员工原样产出,未经平台核实与脱敏</span>
          <a
            className="inline-flex items-center gap-1 font-medium text-brand hover:underline"
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
          <ErrorState description="该类型不支持预览,请下载查看。" />
        )}
      </div>
    </>
  );
}

function ArtifactTextPreview({
  artifact,
  kind
}: {
  artifact: PreviewableArtifact;
  kind: ArtifactPreviewKind;
}) {
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: resolveControlPlaneUrl() }),
    [],
  );
  const contentQuery = useQuery({
    queryKey: ["artifact-content", artifact.id],
    queryFn: () => getArtifactContentText(apiOptions, artifact.id),
    staleTime: Infinity
});

  if (contentQuery.isLoading) {
    return <LoadingState />;
  }
  if (contentQuery.isError || contentQuery.data == null) {
    return (
      <ErrorState
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
        className="h-full min-h-[70vh] w-full rounded-[14px] border border-line bg-white"
        sandbox="allow-scripts"
        srcDoc={contentQuery.data}
        title={artifact.title}
      />
    );
  }
  if (kind === "markdown") {
    return <MarkdownProse>{contentQuery.data}</MarkdownProse>;
  }
  return (
    <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-5 text-ink">
      {contentQuery.data}
    </pre>
  );
}
