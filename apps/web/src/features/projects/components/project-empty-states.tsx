import { FolderKanban } from "lucide-react";
import {
  SoftCard,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
} from "@/components/superteam";

export function ProjectLoadingState({ label = "加载项目数据" }: { label?: string }) {
  return (
    <SoftCard>
      <V3LoadingState label={label} />
    </SoftCard>
  );
}

export function ProjectEmptyState({
  onCreate,
}: {
  onCreate?: () => void;
}) {
  return (
    <SoftCard className="min-h-[360px]">
      <V3EmptyState
        icon={<FolderKanban />}
        title="暂无项目"
        description="创建第一个项目后，可以在这里管理目标、成员池、任务、需求和事件流。"
        action={
          onCreate ? (
            <V3Button type="button" onClick={onCreate}>
              创建项目
            </V3Button>
          ) : undefined
        }
      />
    </SoftCard>
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
    <SoftCard className="p-4">
      <V3ErrorState title={label} onRetry={onRetry} />
    </SoftCard>
  );
}
