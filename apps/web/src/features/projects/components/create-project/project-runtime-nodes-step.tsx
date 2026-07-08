import { useQuery } from "@tanstack/react-query";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { StatusPill, WorkSurface } from "@/components/superteam";
import { listRuntimeNodes, type RuntimeNodeResponse } from "@/lib/api/runtime";
import { cn } from "@/lib/utils";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectRuntimeNodesStepProps = {
  apiBaseUrl: string;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
};

export function ProjectRuntimeNodesStep({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
}: ProjectRuntimeNodesStepProps) {
  const nodesQuery = useQuery({
    queryKey: ["project-create", "runtime-nodes"],
    queryFn: () => listRuntimeNodes({ baseUrl: apiBaseUrl, fetcher }),
  });
  const nodes = nodesQuery.data ?? [];
  const selectedIds = new Set(draft.runtimeNodeIds);

  function toggleNode(node: RuntimeNodeResponse) {
    const nodeId = runtimeNodeIdentifier(node);
    if (selectedIds.has(nodeId)) {
      onChange({
        ...draft,
        runtimeNodeIds: draft.runtimeNodeIds.filter((id) => id !== nodeId),
      });
      return;
    }
    onChange({ ...draft, runtimeNodeIds: [...draft.runtimeNodeIds, nodeId] });
  }

  return (
    <div className="grid gap-5">
      <div>
        <h3 className="text-xl font-semibold text-v3-ink">选择可运行节点</h3>
        <p className="mt-1 text-sm text-v3-ink-2">
          项目至少需要绑定一个可运行节点，任务派发时才能选择本机执行环境；离线节点也可先绑定，派发时再校验可用性。
        </p>
      </div>

      <WorkSurface>
        {nodesQuery.isLoading ? (
          <p className="px-4 py-6 text-sm text-v3-ink-2">正在加载可运行节点...</p>
        ) : nodesQuery.isError ? (
          <p className="px-4 py-6 text-sm text-v3-danger">可运行节点加载失败</p>
        ) : nodes.length === 0 ? (
          <p className="px-4 py-6 text-sm text-v3-ink-2">暂无可运行节点，请先在运行节点管理中注册节点。</p>
        ) : (
          <div className="grid gap-2 p-3">
            {nodes.map((node) => {
              const nodeId = runtimeNodeIdentifier(node);
              const checked = selectedIds.has(nodeId);
              const checkboxId = `project-create-runtime-node-${nodeId}`;

              return (
                <Label
                  className={cn(
                    "flex min-w-0 cursor-pointer items-start gap-3 rounded-[10px] border border-v3-line bg-v3-card p-3 text-sm transition-colors",
                    checked ? "border-v3-brand bg-v3-brand-soft" : "hover:bg-v3-card-soft",
                  )}
                  htmlFor={checkboxId}
                  key={nodeId}
                >
                  <Checkbox
                    aria-label={node.name}
                    checked={checked}
                    id={checkboxId}
                    onCheckedChange={() => toggleNode(node)}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="flex flex-wrap items-center gap-2">
                      <span className="truncate font-semibold text-v3-ink">{node.name}</span>
                      <StatusPill tone={node.status === "online" ? "ok" : "warn"}>
                        {node.status === "online" ? "在线" : "离线"}
                      </StatusPill>
                    </span>
                    <span className="mt-1 block truncate text-xs text-v3-ink-3">
                      负载 {node.current_load}/{node.max_slots}
                      {node.supported_providers.length > 0
                        ? ` · 支持 Provider: ${node.supported_providers.join("、")}`
                        : ""}
                    </span>
                  </span>
                </Label>
              );
            })}
          </div>
        )}
      </WorkSurface>

      <div className="rounded-xl border border-v3-warn/20 bg-v3-warn-soft px-3 py-3 text-sm text-v3-warn">
        至少选择一个可运行节点才能创建项目；节点资格集允许包含离线节点，实际派发时会再次校验可用性。
      </div>
    </div>
  );
}

function runtimeNodeIdentifier(node: RuntimeNodeResponse): string {
  return node.runtime_node_id ?? node.node_id;
}
