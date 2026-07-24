import { FolderKanban } from "lucide-react";
import {
  SoftCard,
  Button,
  EmptyState,
  ErrorState,
  LoadingState
} from "@/components/superteam";

export function ProjectLoadingState({ label = "加载项目数据" }: { label?: string }) {
  return (
    <SoftCard>
      <LoadingState label={label} />
    </SoftCard>
  );
}

export function ProjectEmptyState({
  onCreate
}: {
  onCreate?: () => void;
}) {
  return (
    <SoftCard className="min-h-[360px]">
      <EmptyState
        icon={<FolderKanban />}
        title="暂无项目"
        description="创建第一个项目后，可以在这里管理目标、成员池、任务、需求和事件流。"
        action={
          onCreate ? (
            <Button type="button" onClick={onCreate}>
              创建项目
            </Button>
          ) : undefined
        }
      />
    </SoftCard>
  );
}

export function ProjectErrorState({
  label = "项目数据加载失败",
  onRetry
}: {
  label?: string;
  onRetry?: () => void;
}) {
  return (
    <SoftCard className="p-4">
      <ErrorState title={label} onRetry={onRetry} />
    </SoftCard>
  );
}
