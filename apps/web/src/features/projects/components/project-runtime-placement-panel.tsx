import {
  Cable,
  Link2,
  MonitorCog,
  PlugZap,
  Unlink,
} from "lucide-react";
import type { ReactNode } from "react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  type V3Tone,
} from "@/components/superteam";
import type { ProjectRuntimePlacementReadiness } from "@/lib/api/projects";
import type { RuntimeNodeResponse } from "@/lib/api/runtime";
import { cn } from "@/lib/utils";

export type ProjectRuntimePlacementPanelProps = {
  readiness?: ProjectRuntimePlacementReadiness;
  runtimeNodes: RuntimeNodeResponse[];
  selectedRuntimeNodeId: string;
  onSelectedRuntimeNodeIdChange: (nodeId: string) => void;
  onBindRuntime: () => void;
  onReleaseRuntime: () => void;
  isBinding: boolean;
  isReleasing: boolean;
};

export function ProjectRuntimePlacementPanel({
  readiness,
  runtimeNodes,
  selectedRuntimeNodeId,
  onSelectedRuntimeNodeIdChange,
  onBindRuntime,
  onReleaseRuntime,
  isBinding,
  isReleasing,
}: ProjectRuntimePlacementPanelProps) {
  const sortedNodes = [...runtimeNodes].sort((left, right) => {
    if (left.status === right.status) {
      return left.name.localeCompare(right.name);
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
  const canBind = Boolean(selectedRuntimeNodeId) && !isBinding && !isReleasing;
  const canRelease = hasActivePlacement && !isBinding && !isReleasing;

  return (
    <SoftCard className="overflow-hidden" data-testid="project-runtime-placement-panel">
      <div className="flex flex-col gap-4 border-b border-v3-line p-4 lg:flex-row lg:items-start lg:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone="brand" size="sm">
            <MonitorCog />
          </IconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-sm font-semibold tracking-normal text-v3-ink">
                运行落点
              </h3>
              <StatusPill tone={readinessTone(readiness?.placement_status)}>
                {readinessStatusLabel(readiness?.placement_status)}
              </StatusPill>
            </div>
            <p className="mt-1 text-xs leading-5 text-v3-ink-2">
              绑定项目执行使用的 Runtime 节点，并检查命令通道与 Provider 能力。
            </p>
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap gap-2">
          <V3Button
            disabled={!canBind}
            size="sm"
            type="button"
            onClick={onBindRuntime}
          >
            <Link2 data-icon="inline-start" />
            {isBinding ? "绑定中" : "绑定运行节点"}
          </V3Button>
          <V3Button
            disabled={!canRelease}
            size="sm"
            type="button"
            variant="outline"
            onClick={onReleaseRuntime}
          >
            <Unlink data-icon="inline-start" />
            {isReleasing ? "释放中" : "释放绑定"}
          </V3Button>
        </div>
      </div>

      <div className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(280px,0.8fr)]">
        <div className="grid min-w-0 gap-3">
          <label className="grid gap-1.5 text-sm font-medium text-v3-ink">
            选择运行节点
            <select
              aria-label="选择运行节点"
              className="h-10 w-full rounded-xl border border-v3-line-strong bg-v3-card px-3 text-sm text-v3-ink shadow-sm outline-none transition focus:border-v3-brand focus:ring-2 focus:ring-v3-brand/20 disabled:cursor-not-allowed disabled:opacity-60"
              disabled={sortedNodes.length === 0 || isBinding || isReleasing}
              value={selectedRuntimeNodeId}
              onChange={(event) =>
                onSelectedRuntimeNodeIdChange(event.currentTarget.value)
              }
            >
              <option value="">选择 Runtime 节点</option>
              {sortedNodes.map((node) => {
                const value = runtimeNodeValue(node);
                return (
                  <option key={value} value={value}>
                    {node.name} · {runtimeNodeStatusLabel(node.status)}
                  </option>
                );
              })}
            </select>
          </label>

          <div className="grid gap-2 rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
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
              value={readiness?.runtime_node_name ?? selectedNode?.name ?? "未绑定"}
            />
          </div>
        </div>

        <div className="grid min-w-0 gap-3">
          <div>
            <p className="text-xs font-semibold text-v3-ink-2">Provider 能力</p>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {providerSummary.length > 0 ? (
                providerSummary.map((provider) => (
                  <StatusPill key={provider} tone="info" showDot={false}>
                    {provider}
                  </StatusPill>
                ))
              ) : (
                <span className="text-xs text-v3-ink-2">暂无可用 Provider</span>
              )}
            </div>
          </div>

          <div>
            <p className="text-xs font-semibold text-v3-ink-2">阻塞原因</p>
            {readiness?.blocking_reasons?.length ? (
              <ul className="mt-2 grid gap-2">
                {readiness.blocking_reasons.map((reason) => (
                  <li
                    className="rounded-v3-inner border border-v3-warn/20 bg-v3-warn-soft px-3 py-2 text-xs leading-5 text-v3-ink"
                    key={`${reason.code}-${reason.resource_id ?? reason.message}`}
                  >
                    <span className="font-semibold">{reason.message}</span>
                    <span className="ml-2 font-mono text-[11px] text-v3-ink-2">
                      {reason.code}
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="mt-2 text-xs text-v3-ink-2">当前没有运行落点阻塞。</p>
            )}
          </div>
        </div>
      </div>
    </SoftCard>
  );
}

function RuntimeLine({
  icon,
  label,
  value,
}: {
  icon: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="flex min-w-0 items-center gap-2 text-xs">
      <span className="grid size-7 shrink-0 place-items-center rounded-lg bg-v3-card text-v3-ink-2">
        {icon}
      </span>
      <span className="shrink-0 font-semibold text-v3-ink-2">{label}</span>
      <span className="min-w-0 truncate text-v3-ink">{value}</span>
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

function readinessTone(status?: ProjectRuntimePlacementReadiness["placement_status"]): V3Tone {
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
  return node.runtime_node_id ?? node.node_id;
}
