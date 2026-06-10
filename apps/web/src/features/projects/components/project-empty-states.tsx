import { AlertCircle, FolderKanban, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { LiquidCard, SemanticIconTile } from "@/components/superteam";

export function ProjectLoadingState({ label = "加载项目数据" }: { label?: string }) {
  return (
    <LiquidCard className="flex items-center gap-3 rounded-xl p-5 text-sm text-muted-foreground">
      <Loader2 className="size-4 animate-spin" />
      {label}
    </LiquidCard>
  );
}

export function ProjectEmptyState({
  onCreate,
}: {
  onCreate?: () => void;
}) {
  return (
    <LiquidCard className="flex min-h-[360px] flex-col items-center justify-center gap-4 rounded-xl p-8 text-center">
      <SemanticIconTile tone="info" size="lg">
        <FolderKanban />
      </SemanticIconTile>
      <div className="max-w-md space-y-1">
        <h2 className="text-lg font-semibold">暂无项目</h2>
        <p className="text-sm text-muted-foreground">
          创建第一个项目后，可以在这里管理目标、成员池、任务、需求和事件流。
        </p>
      </div>
      {onCreate ? (
        <Button type="button" onClick={onCreate}>
          创建项目
        </Button>
      ) : null}
    </LiquidCard>
  );
}

export function ProjectErrorState({
  label = "项目数据加载失败",
  onRetry,
}: {
  label?: string;
  onRetry?: () => void;
}) {
  return (
    <LiquidCard className="flex items-center justify-between gap-4 rounded-xl p-5">
      <div className="flex items-center gap-3 text-sm">
        <AlertCircle className="size-4 text-destructive" />
        <span>{label}</span>
      </div>
      {onRetry ? (
        <Button size="sm" variant="outline" type="button" onClick={onRetry}>
          重试
        </Button>
      ) : null}
    </LiquidCard>
  );
}
