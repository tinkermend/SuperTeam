import { CheckCircle2, CircleDot, Network } from "lucide-react";
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import { StatusPill, WorkSurface } from "@/components/superteam";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectReviewPanelProps = {
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  selectableTeams: UserProjectTeamScope[];
};

export function ProjectReviewPanel({ currentUser, draft, selectableTeams }: ProjectReviewPanelProps) {
  const sourceTeams = selectableTeams.filter((scope) => draft.sourceTeamIds.includes(scope.team_id));
  const requiredPassed = [
    Boolean(draft.name.trim()) && Boolean(draft.goal.trim()),
    sourceTeams.length > 0,
    draft.policyToggles.auditLogEnabled,
  ].filter(Boolean).length;
  const reviewRows = [
    { group: "项目事实", label: "项目名称", value: draft.name || "未填写" },
    {
      group: "项目事实",
      label: "来源团队",
      value: sourceTeams.length > 0 ? `${sourceTeams.length} 个已选` : "未选择",
    },
    { group: "项目事实", label: "目标", value: draft.goal || "未填写" },
    {
      group: "人类负责人",
      label: "主负责人",
      value: currentUser?.display_name ?? currentUser?.username ?? "未加载",
    },
    { group: "人类负责人", label: "额外负责人", value: `${draft.ownerUsers.length} 位已选` },
    { group: "数字员工池", label: "执行员工", value: `${draft.selectedDigitalEmployees.length} 位已选` },
    { group: "协调策略", label: "协调预设", value: policyPresetLabel(draft.policyPreset) },
    {
      group: "协调策略",
      label: "审计日志",
      value: draft.policyToggles.auditLogEnabled ? "自动开启" : "未开启",
    },
    {
      group: "协调策略",
      label: "新需求需人工确认",
      value: draft.policyToggles.newDemandNeedsHumanConfirmation ? "是" : "否",
    },
  ];

  return (
    <aside className="grid content-start gap-4">
      <WorkSurface className="rounded-[14px] p-0 shadow-sm">
        <div className="flex items-start justify-between gap-3 border-b border-v3-line px-3 py-3">
          <div>
            <h3 className="text-base font-extrabold text-v3-ink">创建前审阅</h3>
            <p className="mt-1 text-[12px] text-v3-ink-2">将要创建的项目对象字段。</p>
          </div>
          <StatusPill tone="warn">待创建</StatusPill>
        </div>

        <div className="overflow-auto">
          <table
            className="w-full border-separate border-spacing-0 text-[12px]"
            data-testid="project-create-review-table"
          >
            <thead>
              <tr>
                <th className="border-b border-v3-line-strong bg-v3-card-soft px-3 py-2 text-left text-[11px] font-bold uppercase tracking-wide text-v3-ink-3">
                  分组
                </th>
                <th className="border-b border-v3-line-strong bg-v3-card-soft px-3 py-2 text-left text-[11px] font-bold uppercase tracking-wide text-v3-ink-3">
                  字段
                </th>
                <th className="border-b border-v3-line-strong bg-v3-card-soft px-3 py-2 text-left text-[11px] font-bold uppercase tracking-wide text-v3-ink-3">
                  当前值
                </th>
              </tr>
            </thead>
            <tbody>
              {reviewRows.map((row) => (
                <tr key={`${row.group}:${row.label}`} className="[&:hover>td]:bg-v3-card-inner">
                  <td className="border-b border-v3-line px-3 py-2 align-top font-semibold text-v3-ink-2">
                    {row.group}
                  </td>
                  <td className="border-b border-v3-line px-3 py-2 align-top text-v3-ink-3">
                    {row.label}
                  </td>
                  <td className="min-w-0 border-b border-v3-line px-3 py-2 align-top font-semibold text-v3-ink">
                    <span className="line-clamp-2 break-words">{row.value}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <div className="m-3 rounded-[10px] border border-v3-info/20 bg-v3-info-soft px-3 py-2.5 text-[12px] leading-5 text-v3-info">
          <div className="flex gap-2">
            <Network className="mt-0.5 size-4 shrink-0" />
            <span>创建完成后，系统会注册项目协调线程，并可在任务发起中选择该项目提交需求。</span>
          </div>
        </div>
      </WorkSurface>

      <div className="rounded-[14px] border border-v3-line bg-v3-card p-4 shadow-sm">
        <div className="flex items-center gap-2 text-sm font-semibold text-v3-ink">
          <CheckCircle2 className="size-4 text-v3-ok" />
          必备项 {requiredPassed} / 3 已就绪
        </div>
        <div className="mt-3 grid gap-2 text-[13px] text-v3-ink-2">
          <CheckLine checked={Boolean(draft.name.trim()) && Boolean(draft.goal.trim())} label="基础信息已填写" />
          <CheckLine checked={sourceTeams.length > 0} label="来源团队已选择" />
          <CheckLine checked={draft.policyToggles.auditLogEnabled} label="审计策略已开启" />
          <CheckLine checked={draft.selectedDigitalEmployees.length > 0} label="数字员工池已选择（可选）" />
        </div>
      </div>
    </aside>
  );
}

function CheckLine({ checked, label }: { checked: boolean; label: string }) {
  const Icon = checked ? CheckCircle2 : CircleDot;
  return (
    <div className="flex items-center gap-2">
      <Icon className={checked ? "size-4 text-v3-ok" : "size-4 text-v3-ink-3"} />
      <span>{label}</span>
    </div>
  );
}

function policyPresetLabel(preset: ProjectCreateDraft["policyPreset"]) {
  if (preset === "lightweight") return "轻量协作";
  if (preset === "highRisk") return "高风险审批";
  return "标准治理";
}
