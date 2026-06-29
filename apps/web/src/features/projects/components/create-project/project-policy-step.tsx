import type { ReactNode } from "react";
import { ShieldCheck, Sparkles, TriangleAlert } from "lucide-react";
import { Switch } from "@/components/ui/switch";
import { StatusPill } from "@/components/superteam";
import { cn } from "@/lib/utils";
import { applyPolicyPreset, type ProjectCreateDraft, type ProjectPolicyPreset } from "./create-project-draft";

type ProjectPolicyStepProps = {
  draft: ProjectCreateDraft;
  onChange: (draft: ProjectCreateDraft) => void;
};

const presets: Array<{
  description: string;
  icon: ReactNode;
  id: ProjectPolicyPreset;
  label: string;
}> = [
  { description: "适合多数项目，保留人工确认、证据和审计边界。", icon: <ShieldCheck className="size-4" />, id: "standard", label: "标准治理" },
  { description: "降低前置确认，适合低风险协作和试运行。", icon: <Sparkles className="size-4" />, id: "lightweight", label: "轻量协作" },
  { description: "强化审批、证据和预算阈值，适合高风险项目。", icon: <TriangleAlert className="size-4" />, id: "highRisk", label: "高风险审批" },
];

export function ProjectPolicyStep({ draft, onChange }: ProjectPolicyStepProps) {
  return (
    <div className="grid gap-6">
      <div>
        <h3 className="text-xl font-semibold text-v3-ink">策略预设</h3>
        <p className="mt-1 text-sm text-v3-ink-2">定义创建后的审批、预算、证据与验收边界。</p>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        {presets.map((preset) => {
          const active = draft.policyPreset === preset.id;
          return (
            <button
              className={cn(
                "rounded-v3-inner border bg-v3-card p-4 text-left transition hover:border-v3-brand/50",
                active ? "border-v3-brand ring-4 ring-v3-brand/10" : "border-v3-line",
              )}
              key={preset.id}
              onClick={() => onChange(applyPolicyPreset(draft, preset.id))}
              type="button"
            >
              <div className="flex items-center gap-2 text-sm font-semibold text-v3-ink">
                <span className={cn("grid size-8 place-items-center rounded-xl", active ? "bg-v3-brand-soft text-v3-brand" : "bg-v3-mute-soft text-v3-mute")}>
                  {preset.icon}
                </span>
                {preset.label}
              </div>
              <p className="mt-3 text-xs leading-5 text-v3-ink-2">{preset.description}</p>
            </button>
          );
        })}
      </div>

      <div className="divide-y divide-v3-line rounded-v3-inner border border-v3-line bg-v3-card">
        <PolicyToggle
          checked={draft.policyToggles.newDemandNeedsHumanConfirmation}
          description="任何新需求在执行前需由主负责人确认。"
          label="新需求需要人工确认"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, newDemandNeedsHumanConfirmation: checked } })}
        />
        <PolicyToggle
          checked={draft.policyToggles.highRiskActionNeedsConfirmation}
          description="涉及数据删除、权限变更、外部调用等高风险动作需暂停并等待主负责人确认。"
          label="高风险动作暂停等待确认"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, highRiskActionNeedsConfirmation: checked } })}
        />
        <PolicyToggle
          checked={draft.policyToggles.requireEvidenceBeforeAcceptance}
          description="最终验收前必须补齐产出、测试、日志或审计证据。"
          label="验收前必须补齐证据"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, requireEvidenceBeforeAcceptance: checked } })}
        />
        <PolicyToggle
          checked={draft.policyToggles.budgetOverrunNeedsOwnerApproval}
          description="实际消耗超过预算阈值时，需要主负责人审批后继续。"
          label="预算超限需负责人审批"
          onCheckedChange={(checked) => onChange({ ...draft, policyToggles: { ...draft.policyToggles, budgetOverrunNeedsOwnerApproval: checked } })}
        />
      </div>

      <div className="flex flex-wrap gap-2">
        <StatusPill tone="info">审批策略</StatusPill>
        <StatusPill tone="artifact">工作契约</StatusPill>
        <StatusPill tone="warn">证据要求</StatusPill>
        <StatusPill tone="ok">审计默认开启</StatusPill>
      </div>
    </div>
  );
}

function PolicyToggle({
  checked,
  description,
  label,
  onCheckedChange,
}: {
  checked: boolean;
  description: string;
  label: string;
  onCheckedChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between gap-4 px-4 py-3">
      <div>
        <p className="text-sm font-semibold text-v3-ink">{label}</p>
        <p className="mt-1 text-xs text-v3-ink-3">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} />
    </div>
  );
}
