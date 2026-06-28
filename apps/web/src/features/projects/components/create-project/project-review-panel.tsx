import type { ReactNode } from "react";
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
  const team = selectableTeams.find((scope) => scope.team_id === draft.teamId);
  const requiredPassed = [
    Boolean(draft.name.trim()) && Boolean(draft.goal.trim()),
    Boolean(team),
    draft.policyToggles.auditLogEnabled,
  ].filter(Boolean).length;

  return (
    <aside className="grid content-start gap-4">
      <WorkSurface className="p-6">
        <div className="mb-5 flex items-start justify-between gap-4">
          <div>
            <h3 className="text-lg font-semibold text-v3-ink">创建前审阅</h3>
            <p className="mt-1 text-sm text-v3-ink-2">以下为将要创建的项目对象预览。</p>
          </div>
          <StatusPill tone="warn">待创建</StatusPill>
        </div>

        <ReviewSection title="项目事实">
          <ReviewRow label="项目名称" value={draft.name || "未填写"} />
          <ReviewRow label="所属团队" value={team?.team.name ?? "未选择"} />
          <ReviewRow label="目标" value={draft.goal || "未填写"} />
        </ReviewSection>

        <ReviewSection title="人类责任">
          <ReviewRow label="固定负责人" value={currentUser?.display_name ?? currentUser?.username ?? "未加载"} />
          <ReviewRow label="项目负责人" value={draft.leaderUser?.display_name ?? draft.leaderUser?.username ?? "未选择"} />
          <ReviewRow label="验收负责人" value={draft.acceptanceUser?.display_name ?? draft.acceptanceUser?.username ?? "未选择"} />
          <ReviewRow label="审核人" value={`${draft.reviewerUsers.length} 位已选`} />
        </ReviewSection>

        <ReviewSection title="数字员工池">
          <ReviewRow label="执行员工" value={`${draft.selectedDigitalEmployees.length} 位已选`} />
        </ReviewSection>

        <ReviewSection title="策略与审计">
          <ReviewRow label="策略预设" value={policyPresetLabel(draft.policyPreset)} />
          <ReviewRow label="审计日志" value={draft.policyToggles.auditLogEnabled ? "自动开启" : "未开启"} />
          <ReviewRow label="证据要求" value={draft.policyToggles.requireEvidenceBeforeAcceptance ? "验收前必须补齐" : "轻量要求"} />
        </ReviewSection>

        <div className="mt-5 rounded-xl border border-v3-info/20 bg-v3-info-soft px-3 py-3 text-sm text-v3-info">
          <div className="flex gap-2">
            <Network className="mt-0.5 size-4 shrink-0" />
            <span>创建完成后，系统会注册项目协调线程，并可在任务发起中选择该项目提交需求。</span>
          </div>
        </div>
      </WorkSurface>

      <div className="rounded-v3-card border border-v3-line bg-v3-card p-5 shadow-sm">
        <div className="flex items-center gap-2 text-sm font-semibold text-v3-ink">
          <CheckCircle2 className="size-4 text-v3-ok" />
          必备项 {requiredPassed} / 3 已就绪
        </div>
        <div className="mt-3 grid gap-2 text-sm text-v3-ink-2">
          <CheckLine checked={Boolean(draft.name.trim()) && Boolean(draft.goal.trim())} label="基础信息已填写" />
          <CheckLine checked={Boolean(team)} label="团队授权有效" />
          <CheckLine checked={draft.policyToggles.auditLogEnabled} label="审计策略已开启" />
          <CheckLine checked={draft.selectedDigitalEmployees.length > 0} label="数字员工池已选择（可选）" />
        </div>
      </div>
    </aside>
  );
}

function ReviewSection({ children, title }: { children: ReactNode; title: string }) {
  return (
    <section className="border-t border-v3-line py-4 first:border-t-0 first:pt-0">
      <h4 className="mb-3 text-sm font-semibold text-v3-ink">{title}</h4>
      <div className="grid gap-2">{children}</div>
    </section>
  );
}

function ReviewRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[7rem_1fr] gap-3 text-sm">
      <span className="text-v3-ink-3">{label}</span>
      <span className="min-w-0 truncate font-medium text-v3-ink">{value}</span>
    </div>
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
