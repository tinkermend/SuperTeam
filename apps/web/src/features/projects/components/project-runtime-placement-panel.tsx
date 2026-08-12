import { useState, type ReactNode } from "react";
import {
  Cable,
  ChevronDown,
  Link2,
  MonitorCog,
  PlugZap,
  Unlink
} from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from "@/components/ui/collapsible";
import {
  IconTile,
  SoftCard,
  StatusPill,
  Button,
  type Tone
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  ProjectRuntimeNodeBinding,
  ProjectRuntimePlacementReadiness
} from "@/lib/api/projects";
import type { RuntimeNodeResponse } from "@/lib/api/runtime";

export type ProjectRuntimePlacementPanelProps = {
  readiness?: ProjectRuntimePlacementReadiness;
  runtimeNodes: RuntimeNodeResponse[];
  /** 已绑定节点及其供给状态（spec 2026-08-12 §5.2）。 */
  bindings?: ProjectRuntimeNodeBinding[];
  selectedRuntimeNodeId: string;
  onSelectedRuntimeNodeIdChange: (nodeId: string) => void;
  onBindRuntime: () => void;
  onReleaseRuntime: () => void;
  onProvisionRuntimeNode?: (runtimeNodeId: string) => void;
  isBinding: boolean;
  isReleasing: boolean;
  provisioningNodeId?: string | null;
  isReadinessLoading?: boolean;
};

export function ProjectRuntimePlacementPanel({
  readiness,
  runtimeNodes,
  bindings,
  selectedRuntimeNodeId,
  onSelectedRuntimeNodeIdChange,
  onBindRuntime,
  onReleaseRuntime,
  onProvisionRuntimeNode,
  isBinding,
  isReleasing,
  provisioningNodeId,
  isReadinessLoading
}: ProjectRuntimePlacementPanelProps) {
  const sortedNodes = [...runtimeNodes].sort((left, right) => {
    if (left.status === right.status) {
      return runtimeNodeLabel(left).localeCompare(runtimeNodeLabel(right));
    }
    return left.status === "online" ? -1 : 1;
  });
  const selectedNode = sortedNodes.find(
    (node) => runtimeNodeValue(node) === selectedRuntimeNodeId,
  );
  const providerSummary = readiness?.provider_capabilities?.length
    ? readiness.provider_capabilities
    : selectedNode?.supported_providers ?? readiness?.required_provider_types ?? [];
  const hasActivePlacement = Boolean(readiness?.runtime_node_id);
  const canBind =
    Boolean(selectedRuntimeNodeId) && !isReadinessLoading && !isBinding && !isReleasing;
  const canRelease = hasActivePlacement && !isReadinessLoading && !isBinding && !isReleasing;
  // 未绑定或阻塞时默认展开（需要用户介入），已就绪默认折叠为一行摘要。
  const isReady = hasActivePlacement && readiness?.placement_status === "ready";
  const [openOverride, setOpenOverride] = useState<boolean | null>(null);
  const open = openOverride ?? !isReady;

  return (
    <SoftCard className="overflow-hidden" data-testid="project-runtime-placement-panel">
      <Collapsible open={open} onOpenChange={setOpenOverride}>
        <div
          className={cn(
            "flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between",
            open && "border-b border-line",
          )}
        >
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="brand" size="sm">
              <MonitorCog />
            </IconTile>
            <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1.5">
              <h3 className="text-sm font-semibold tracking-normal text-ink">
                运行落点
              </h3>
              <StatusPill tone={readinessTone(readiness?.placement_status)}>
                {readinessStatusLabel(readiness?.placement_status)}
              </StatusPill>
              {!open ? (
                <>
                  <span className="flex items-center gap-1.5 text-xs text-ink-2">
                    <Cable className="size-3.5" />
                    {readiness?.command_channel_connected
                      ? "命令通道已连接"
                      : "命令通道未连接"}
                  </span>
                  <span className="min-w-0 max-w-40 truncate text-xs text-ink-2">
                    {readiness?.runtime_node_name ??
                      runtimeNodeLabel(selectedNode) ??
                      "未绑定"}
                  </span>
                  {providerSummary.length > 0 ? (
                    <span className="flex flex-wrap gap-1.5">
                      {providerSummary.map((provider) => (
                        <StatusPill key={provider} tone="info" showDot={false}>
                          {provider}
                        </StatusPill>
                      ))}
                    </span>
                  ) : null}
                </>
              ) : null}
            </div>
          </div>
          <CollapsibleTrigger asChild>
            <Button
              aria-label={open ? "收起运行落点" : "展开运行落点"}
              className="shrink-0"
              size="sm"
              type="button"
              variant="outline"
            >
              {open ? "收起" : "管理"}
              <ChevronDown
                className={cn("size-3.5 transition-transform", open && "rotate-180")}
              />
            </Button>
          </CollapsibleTrigger>
        </div>

        <CollapsibleContent>
          <div className="flex flex-col gap-3 p-4 sm:flex-row sm:items-start sm:justify-between">
            <p className="min-w-0 text-xs leading-5 text-ink-2">
              绑定项目执行使用的 Runtime 节点，并检查命令通道与 Provider 能力。
            </p>
            <div className="flex shrink-0 flex-wrap gap-2">
              <Button
                disabled={!canBind}
                size="sm"
                type="button"
                onClick={onBindRuntime}
              >
                <Link2 data-icon="inline-start" />
                {isBinding ? "绑定中" : "绑定运行节点"}
              </Button>
              <Button
                disabled={!canRelease}
                size="sm"
                type="button"
                variant="outline"
                onClick={onReleaseRuntime}
              >
                <Unlink data-icon="inline-start" />
                {isReleasing ? "释放中" : "释放绑定"}
              </Button>
            </div>
          </div>

          <div className="grid gap-4 px-4 pb-4 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.8fr)]">
            <div className="grid min-w-0 gap-3">
              <label className="grid gap-1.5 text-sm font-medium text-ink">
                选择运行节点
                <select
                  aria-label="选择运行节点"
                  className="h-10 w-full rounded-xl border border-line-strong bg-card px-3 text-sm text-ink shadow-sm outline-none transition focus:border-brand focus:ring-2 focus:ring-brand/20 disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={sortedNodes.length === 0 || isBinding || isReleasing}
                  value={selectedRuntimeNodeId}
                  onChange={(event) =>
                    onSelectedRuntimeNodeIdChange(event.currentTarget.value)
                  }
                >
                  <option value="">选择 Runtime 节点</option>
                  {sortedNodes.map((node, index) => {
                    const value = runtimeNodeValue(node);
                    return (
                      <option key={`${value}-${index}`} value={value}>
                        {runtimeNodeLabel(node)} · {runtimeNodeStatusLabel(node.status)}
                      </option>
                    );
                  })}
                </select>
              </label>

              <div className="grid gap-2 rounded-inner border border-line bg-card-soft p-3">
                <RuntimeLine
                  icon={<PlugZap className="size-3.5" />}
                  label="命令通道"
                  value={
                    readiness?.command_channel_connected
                      ? "已连接"
                      : "未连接或等待绑定"
                  }
                />
                <RuntimeLine
                  icon={<Cable className="size-3.5" />}
                  label="当前节点"
                  value={readiness?.runtime_node_name ?? runtimeNodeLabel(selectedNode) ?? "未绑定"}
                />
              </div>
            </div>

            <div className="grid min-w-0 gap-3">
              <div>
                <p className="text-xs font-semibold text-ink-2">已绑定节点</p>
                {bindings && bindings.length > 0 ? (
                  <ul className="mt-2 grid gap-2" data-testid="project-runtime-bindings">
                    {bindings.map((binding) => {
                      const supplied = binding.provision_status !== "unprovisioned";
                      const boundNode = runtimeNodes.find(
                        (node) => runtimeNodeValue(node) === binding.runtime_node_id,
                      );
                      // 名称查不到时退回 id，别显示成「未绑定」。
                      const label = boundNode
                        ? runtimeNodeLabel(boundNode)
                        : binding.runtime_node_id;
                      const isProvisioning = provisioningNodeId === binding.runtime_node_id;
                      return (
                        <li
                          className="flex flex-wrap items-center gap-2 rounded-inner border border-line bg-card px-3 py-2 text-xs"
                          key={binding.runtime_node_id}
                        >
                          <span className="min-w-0 flex-1 truncate text-ink">{label}</span>
                          <StatusPill tone={supplied ? "ok" : "warn"}>
                            {supplied ? "已供给" : "已绑定未供给"}
                          </StatusPill>
                          {!supplied && onProvisionRuntimeNode ? (
                            <Button
                              disabled={isProvisioning}
                              size="sm"
                              type="button"
                              variant="outline"
                              onClick={() => onProvisionRuntimeNode(binding.runtime_node_id)}
                            >
                              {isProvisioning ? "供给中" : "供给工作区"}
                            </Button>
                          ) : null}
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <p className="mt-2 text-xs text-ink-2">尚未绑定运行节点。</p>
                )}
                {bindings?.some((binding) => binding.provision_status === "unprovisioned") ? (
                  <p className="mt-2 text-[11px] leading-4 text-ink-3">
                    未供给的节点不进派发候选。供给会在该节点建目录或 clone 仓库；
                    <strong>主节点上未推送的改动不会跟过去</strong>。
                  </p>
                ) : null}
              </div>

              <div>
                <p className="text-xs font-semibold text-ink-2">Provider 能力</p>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {providerSummary.length > 0 ? (
                    providerSummary.map((provider) => (
                      <StatusPill key={provider} tone="info" showDot={false}>
                        {provider}
                      </StatusPill>
                    ))
                  ) : (
                    <span className="text-xs text-ink-2">暂无可用 Provider</span>
                  )}
                </div>
              </div>

              <div>
                <p className="text-xs font-semibold text-ink-2">阻塞原因</p>
                {readiness?.blocking_reasons?.length ? (
                  <ul className="mt-2 grid gap-2">
                    {readiness.blocking_reasons.map((reason) => (
                      <li
                        className="rounded-inner border border-warn/20 bg-warn-soft px-3 py-2 text-xs leading-5 text-ink"
                        key={`${reason.code}-${reason.resource_id ?? reason.message}`}
                      >
                        <span className="font-semibold">{reason.message}</span>
                        <span className="ml-2 font-mono text-[11px] text-ink-2">
                          {reason.code}
                        </span>
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="mt-2 text-xs text-ink-2">当前没有运行落点阻塞。</p>
                )}
              </div>
            </div>
          </div>
        </CollapsibleContent>
      </Collapsible>
    </SoftCard>
  );
}

function RuntimeLine({
  icon,
  label,
  value
}: {
  icon: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="flex min-w-0 items-center gap-2 text-xs">
      <span className="grid size-7 shrink-0 place-items-center rounded-lg bg-card text-ink-2">
        {icon}
      </span>
      <span className="shrink-0 font-semibold text-ink-2">{label}</span>
      <span className="min-w-0 truncate text-ink">{value}</span>
    </div>
  );
}

function readinessStatusLabel(status?: ProjectRuntimePlacementReadiness["placement_status"]) {
  switch (status) {
    case "ready":
      return "已就绪";
    case "missing":
      return "缺少运行节点";
    case "runtime_offline":
      return "节点离线";
    case "command_channel_disconnected":
      return "命令通道断开";
    case "provider_unavailable":
      return "Provider 不可用";
    case "capacity_full":
      return "容量已满";
    case "workspace_pending":
      return "工作区准备中";
    case "contract_mismatch":
      return "契约不匹配";
    default:
      return "等待检查";
  }
}

function readinessTone(status?: ProjectRuntimePlacementReadiness["placement_status"]): Tone {
  switch (status) {
    case "ready":
      return "ok";
    case "missing":
    case "runtime_offline":
    case "command_channel_disconnected":
    case "provider_unavailable":
    case "capacity_full":
    case "contract_mismatch":
      return "warn";
    case "workspace_pending":
      return "info";
    default:
      return "mute";
  }
}

function runtimeNodeStatusLabel(status: RuntimeNodeResponse["status"]) {
  return status === "online" ? "在线" : "离线";
}

function runtimeNodeValue(node: RuntimeNodeResponse) {
  return node.runtime_node_id ?? node.node_id ?? node.name ?? "";
}

function runtimeNodeLabel(node?: RuntimeNodeResponse) {
  if (!node) {
    return "未绑定";
  }
  return node.name || node.node_id || node.runtime_node_id || "未命名节点";
}
