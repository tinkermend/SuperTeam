import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import {
  Button,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  confirmWorkspaceDelete,
  listWorkspaceDeleteRequests,
  rejectWorkspaceDelete,
  type WorkspaceDeleteRequest,
} from "@/lib/api/projects";
import { statusLabel } from "@/lib/status-labels";

type Props = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

/**
 * Admin queue for deferred workspace directory deletion
 * (spec 2026-08-12 P0). Hosted on Runtime 节点 (治理平台) — node-scoped
 * disk governance, not project portfolio. Hidden when empty/error.
 */
export function WorkspaceDeleteQueueSection({ apiBaseUrl, fetcher }: Props) {
  const apiOptions: ApiClientOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();
  const pending = useQuery({
    queryKey: ["workspace-delete-requests"],
    queryFn: () => listWorkspaceDeleteRequests(apiOptions),
  });
  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["workspace-delete-requests"] });
  };
  const confirmMutation = useMutation({
    mutationFn: (requestId: string) => confirmWorkspaceDelete(apiOptions, requestId),
    onSuccess: invalidate,
  });
  const rejectMutation = useMutation({
    mutationFn: (requestId: string) => rejectWorkspaceDelete(apiOptions, requestId),
    onSuccess: invalidate,
  });

  const items = Array.isArray(pending.data) ? pending.data : [];
  if (pending.isLoading || pending.isError || items.length === 0) return null;

  return (
    <WorkSurface data-workspace-delete-queue>
      <div className="flex items-center justify-between gap-3 px-4 pt-4">
        <h2 className="text-sm font-semibold text-ink">工作区目录待删除确认</h2>
        <span className="text-xs text-ink-3">
          确认后从节点磁盘删除；拒绝则平台放手不再管理该目录
        </span>
      </div>
      <DataTable aria-label="工作区目录待删除确认">
        <thead>
          <tr>
            <Th>目录</Th>
            <Th>节点</Th>
            <Th>来源</Th>
            <Th>请求时间</Th>
            <Th>滞留</Th>
            <Th aria-label="操作" />
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <QueueRow
              key={item.id}
              item={item}
              confirmPending={confirmMutation.isPending}
              rejectPending={rejectMutation.isPending}
              onConfirm={() => confirmMutation.mutate(item.id)}
              onReject={() => rejectMutation.mutate(item.id)}
            />
          ))}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}

function QueueRow({
  item,
  confirmPending,
  rejectPending,
  onConfirm,
  onReject,
}: {
  item: WorkspaceDeleteRequest;
  confirmPending: boolean;
  rejectPending: boolean;
  onConfirm: () => void;
  onReject: () => void;
}) {
  const stalledDays = Math.max(
    0,
    Math.floor((Date.now() - new Date(item.requested_at).getTime()) / 86_400_000),
  );
  const projectName =
    typeof item.repo_summary?.project_name === "string"
      ? item.repo_summary.project_name
      : undefined;
  return (
    <Tr>
      <Td>
        <span className="font-medium text-ink">{item.directory_name}</span>
        {projectName ? (
          <span className="ml-2 text-xs text-ink-3">{projectName}</span>
        ) : null}
      </Td>
      <Td className="font-mono text-xs">{item.node_id_snapshot}</Td>
      <Td>{statusLabel(item.ownership)}</Td>
      <Td className="tabular-nums">
        {new Date(item.requested_at).toLocaleString("zh-CN", { hour12: false })}
      </Td>
      <Td className="tabular-nums">{stalledDays} 天</Td>
      <Td>
        <div className="flex justify-end gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={rejectPending || confirmPending}
            onClick={onReject}
          >
            拒绝（放手）
          </Button>
          <AlertDialog>
            <AlertDialogTrigger asChild>
              <Button size="sm" variant="danger" disabled={confirmPending || rejectPending}>
                确认删除
              </Button>
            </AlertDialogTrigger>
            <AlertDialogContent>
              <AlertDialogHeader>
                <AlertDialogTitle>确认删除工作区目录</AlertDialogTitle>
                <AlertDialogDescription>
                  将从节点「{item.node_id_snapshot}」删除目录「{item.directory_name}
                  」（{statusLabel(item.ownership)}）。操作不可逆；若节点离线将保持待确认可稍后重试。
                </AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel>取消</AlertDialogCancel>
                <AlertDialogAction onClick={onConfirm}>确认删除目录</AlertDialogAction>
              </AlertDialogFooter>
            </AlertDialogContent>
          </AlertDialog>
        </div>
      </Td>
    </Tr>
  );
}
