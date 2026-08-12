import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Button, StatusPill, WorkSurface } from "@/components/superteam";
import { probeProjectDirectory } from "@/lib/api/projects";
import { listRuntimeNodes, type RuntimeNodeResponse } from "@/lib/api/runtime";
import { cn } from "@/lib/utils";
import { attachProbeTarget, type ProjectCreateDraft } from "./create-project-draft";

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
  onChange
}: ProjectRuntimeNodesStepProps) {
  const nodesQuery = useQuery({
    queryKey: ["project-create", "runtime-nodes"],
    queryFn: () => listRuntimeNodes({ baseUrl: apiBaseUrl, fetcher })
});
  const nodes = nodesQuery.data ?? [];
  const selectedIds = new Set(draft.runtimeNodeIds);

  function toggleNode(node: RuntimeNodeResponse) {
    const nodeId = runtimeNodeIdentifier(node);
    if (selectedIds.has(nodeId)) {
      onChange({
        ...draft,
        runtimeNodeIds: draft.runtimeNodeIds.filter((id) => id !== nodeId)
});
      return;
    }
    onChange({ ...draft, runtimeNodeIds: [...draft.runtimeNodeIds, nodeId] });
  }

  return (
    <div className="grid gap-5">
      <div>
        <h3 className="text-xl font-semibold text-ink">选择可运行节点</h3>
        <p className="mt-1 text-sm text-ink-2">
          项目至少需要绑定一个可运行节点，任务派发时才能选择本机执行环境；离线节点也可先绑定，派发时再校验可用性。
        </p>
      </div>

      <WorkSurface>
        {nodesQuery.isLoading ? (
          <p className="px-4 py-6 text-sm text-ink-2">正在加载可运行节点...</p>
        ) : nodesQuery.isError ? (
          <p className="px-4 py-6 text-sm text-danger">可运行节点加载失败</p>
        ) : nodes.length === 0 ? (
          <p className="px-4 py-6 text-sm text-ink-2">暂无可运行节点，请先在运行节点管理中注册节点。</p>
        ) : (
          <div className="grid gap-2 p-3">
            {nodes.map((node) => {
              const nodeId = runtimeNodeIdentifier(node);
              const checked = selectedIds.has(nodeId);
              const checkboxId = `project-create-runtime-node-${nodeId}`;

              return (
                <Label
                  className={cn(
                    "flex min-w-0 cursor-pointer items-start gap-3 rounded-[10px] border border-line bg-card p-3 text-sm transition-colors",
                    checked ? "border-brand bg-brand-soft" : "hover:bg-card-soft",
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
                      <span className="truncate font-semibold text-ink">{node.name}</span>
                      <StatusPill tone={node.status === "online" ? "ok" : "warn"}>
                        {node.status === "online" ? "在线" : "离线"}
                      </StatusPill>
                    </span>
                    <span className="mt-1 block truncate text-xs text-ink-3">
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

      <div className="rounded-xl border border-warn/20 bg-warn-soft px-3 py-3 text-sm text-warn">
        至少选择一个可运行节点才能创建项目；节点资格集允许包含离线节点，实际派发时会再次校验可用性。
        <span className="mt-1 block">
          只有<strong>第一个</strong>节点会在创建时被供给（建目录 / clone / 认领）；其余仅为候选，
          需要时再由管理员确认供给。
        </span>
      </div>

      {draft.sourceKind === "attach" ? (
        <AttachProbePanel
          apiBaseUrl={apiBaseUrl}
          draft={draft}
          fetcher={fetcher}
          nodes={nodes}
          onChange={onChange}
        />
      ) : null}
    </div>
  );
}

/**
 * 认领已有目录的探测面板（spec 2026-08-12 §5.1 / §7）：先探测目标节点上的目录，
 * 把真实事实摆给人看，人确认后才允许提交。平台只读——探测不创建目录、不改 git 状态。
 */
function AttachProbePanel({
  apiBaseUrl,
  draft,
  fetcher,
  nodes,
  onChange
}: {
  apiBaseUrl: string;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  nodes: RuntimeNodeResponse[];
  onChange: (draft: ProjectCreateDraft) => void;
}) {
  const primaryNodeId = draft.runtimeNodeIds[0] ?? "";
  const primaryNode = nodes.find((node) => runtimeNodeIdentifier(node) === primaryNodeId);
  const directoryName = draft.directoryName.trim();
  const target = attachProbeTarget(draft);
  const canProbe = Boolean(primaryNodeId) && Boolean(directoryName);

  // 探测结果只对「探测时的那个目标」有效：换节点或改目录名后事实即失效，不复用。
  const [probedTarget, setProbedTarget] = useState<string | null>(null);
  const probe = useMutation({
    mutationFn: () =>
      probeProjectDirectory(
        { baseUrl: apiBaseUrl, fetcher },
        { directory_name: directoryName, runtime_node_id: primaryNodeId },
      ),
    onSuccess: () => {
      setProbedTarget(target);
      // 新一轮探测的结果尚未被人看过：旧确认作废。
      onChange({ ...draft, attachProbeConfirmed: false, attachProbeKey: "" });
    },
  });
  const facts = probe.data;
  const showFacts = Boolean(facts) && probedTarget === target;

  return (
    <WorkSurface className="grid gap-3 p-4" data-testid="project-create-attach-probe">
      <div>
        <h4 className="text-sm font-semibold text-ink">认领目录探测</h4>
        <p className="mt-1 text-xs leading-5 text-ink-2">
          平台不会创建或填充该目录，也不会改动它的 git 状态。请先探测，确认这就是要认领的目录再提交。
        </p>
      </div>

      <div className="grid gap-1.5 rounded-inner border border-line bg-card-soft p-3 text-xs">
        <ProbeLine label="主节点" value={primaryNode?.name ?? primaryNodeId ?? "未选择"} />
        <ProbeLine label="目录名" value={directoryName || "未填写"} />
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          disabled={!canProbe || probe.isPending}
          size="sm"
          type="button"
          onClick={() => probe.mutate()}
        >
          {probe.isPending ? "探测中" : showFacts ? "重新探测" : "探测目录"}
        </Button>
        {!canProbe ? (
          <span className="text-xs text-ink-2">先在上方选定主节点，并在「基础信息」填写目录名。</span>
        ) : null}
      </div>

      {probe.isError ? (
        <p className="rounded-inner border border-danger/20 bg-danger-soft px-3 py-2 text-xs text-danger" role="alert">
          探测失败：{probe.error instanceof Error ? probe.error.message : "未知错误"}
        </p>
      ) : null}

      {showFacts && facts ? (
        <>
          <div className="grid gap-1.5 rounded-inner border border-line bg-card p-3 text-xs">
            <ProbeLine label="目录存在" value={facts.exists ? "是" : "否"} />
            <ProbeLine label="Git 仓库" value={facts.is_git_repo ? "是" : "否（普通目录）"} />
            {facts.is_git_repo ? (
              <>
                <ProbeLine label="远端 origin" value={facts.origin_url || "未配置（无法推送）"} />
                <ProbeLine
                  label="当前分支"
                  value={facts.detached ? "detached HEAD" : facts.current_branch || "未知"}
                />
                <ProbeLine label="工作区改动" value={facts.dirty ? "有未提交改动" : "干净"} />
                {facts.head_commit ? (
                  <ProbeLine label="HEAD" value={facts.head_commit.slice(0, 12)} />
                ) : null}
              </>
            ) : null}
          </div>

          <Label className="flex cursor-pointer items-start gap-2.5 text-xs leading-5 text-ink">
            <Checkbox
              aria-label="确认认领该目录"
              checked={draft.attachProbeConfirmed && draft.attachProbeKey === target}
              onCheckedChange={(checked) =>
                onChange({
                  ...draft,
                  attachProbeConfirmed: checked === true,
                  attachProbeKey: checked === true ? target : "",
                })
              }
            />
            <span>
              我已确认：节点「{primaryNode?.name ?? primaryNodeId}」上的目录「{directoryName}」
              就是要纳入平台管理的目录。
            </span>
          </Label>
        </>
      ) : null}
    </WorkSurface>
  );
}

function ProbeLine({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-baseline gap-2">
      <span className="shrink-0 font-semibold text-ink-2">{label}</span>
      <span className="min-w-0 truncate text-ink">{value}</span>
    </div>
  );
}

function runtimeNodeIdentifier(node: RuntimeNodeResponse): string {
  return node.runtime_node_id ?? node.node_id;
}
