import { Link } from "@tanstack/react-router";
import { ArrowLeft, Blocks, FileClock, Play, Settings, Trash2 } from "lucide-react";
import { StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type { DigitalEmployee, DigitalEmployeeAvatarAsset } from "@/lib/api/employees";
import { EmployeeAvatar } from "../avatar";
import { providerDisplayName } from "../provider-label";

type EmployeeDetailHeaderProps = {
  employee: DigitalEmployee;
  onDelete?: () => void;
  onStartTask: () => void;
  onManageCapabilities: () => void;
};

const statusTone: Record<string, V3Tone> = {
  active: "ok",
  ready: "info",
  disabled: "mute",
  archived: "mute",
  error: "danger",
};

// EmployeeAvatar.asset 期望 DigitalEmployeeAvatarAsset | null | undefined，而
// employee.metadata?.avatar 在类型上是 Record<string, unknown> | undefined——
// 两者结构相同但 TS 不能自动兼容，用 as never 会掩盖真实类型问题，因此在这里显式收窄。
function avatarAssetFromMetadata(
  metadata: DigitalEmployee["metadata"],
): DigitalEmployeeAvatarAsset | undefined {
  const avatar = metadata?.avatar;
  return avatar && typeof avatar === "object"
    ? (avatar as DigitalEmployeeAvatarAsset)
    : undefined;
}

export function EmployeeDetailHeader({
  employee,
  onDelete,
  onStartTask,
  onManageCapabilities,
}: EmployeeDetailHeaderProps) {
  const avatarAsset = avatarAssetFromMetadata(employee.metadata);
  const canDelete = Boolean(onDelete && employee.allowed_actions?.includes("employee.delete"));

  return (
    <div className="flex flex-col gap-4 rounded-v3-card border border-v3-line bg-v3-card p-5 shadow-sm lg:flex-row lg:items-center lg:justify-between">
      <div className="flex min-w-0 items-center gap-4">
        <EmployeeAvatar asset={avatarAsset} name={employee.name} size="lg" />
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate text-[22px] font-extrabold tracking-tight text-v3-ink">
              {employee.name}
            </h2>
            <StatusPill tone={statusTone[employee.status] ?? "mute"}>{employee.status}</StatusPill>
          </div>
          <p className="mt-1 truncate text-[13px] text-v3-ink-2">
            数字员工身份 · Provider {providerDisplayName(employee.provider_type)} · 生效上下文与历史执行记录
          </p>
          <p className="mt-1 truncate text-xs text-v3-ink-3">
            角色 {employee.role}
            {employee.description ? ` · ${employee.description}` : ""}
          </p>
        </div>
      </div>
      <div className="flex shrink-0 flex-wrap gap-2">
        <V3Button asChild variant="outline">
          <Link to="/employees">
            <ArrowLeft className="size-4" />
            返回列表
          </Link>
        </V3Button>
        <V3Button onClick={onManageCapabilities} type="button" variant="outline">
          <Blocks className="size-4" />
          管理技能与 MCP
        </V3Button>
        <V3Button asChild variant="outline">
          <Link params={{ employeeId: employee.id }} to="/employees/$employeeId/config">
            <Settings className="size-4" />
            编辑员工配置
          </Link>
        </V3Button>
        <V3Button asChild variant="outline">
          <Link to="/audit">
            <FileClock className="size-4" />
            查看审计
          </Link>
        </V3Button>
        {canDelete ? (
          <V3Button onClick={onDelete} type="button" variant="danger">
            <Trash2 className="size-4" />
            删除员工
          </V3Button>
        ) : null}
        <V3Button onClick={onStartTask} type="button" variant="outline">
          <Play className="size-4" />
          开始任务
        </V3Button>
      </div>
    </div>
  );
}
