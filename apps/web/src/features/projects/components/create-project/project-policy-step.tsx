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
  {
    description: "新需求默认需人类复核后再进入协调规划。",
    icon: <ShieldCheck className="size-4" />,
    id: "standard",
    label: "标准治理",
  },
  {
    description: "新需求可直接进入规划，适合低风险试运行。",
    icon: <Sparkles className="size-4" />,
    id: "lightweight",
    label: "轻量协作",
  },
  {
    description: "与标准相同：新需求强制人类复核（高风险场景预设）。",
    icon: <TriangleAlert className="size-4" />,
    id: "highRisk",
    label: "高风险复核",
  },
];

export function ProjectPolicyStep({ draft, onChange }: ProjectPolicyStepProps) {
  return (
    <div className="grid gap-6">
      <div>
        <h3 className="text-xl font-semibold text-v3-ink">协调策略</h3>
        <p className="mt-1 text-sm text-v3-ink-2">
          仅配置实际驱动协调线程的开关；项目级审批/证据 JSON 策略已退役。
        </p>
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
                <span
                  className={cn(
                    "grid size-8 place-items-center rounded-xl",
                    active ? "bg-v3-brand-soft text-v3-brand" : "bg-v3-mute-soft text-v3-mute",
                  )}
                >
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
          description="开启后，协调线程对新提交需求强制人工复核，并将新任务标记为需审批。"
          label="新需求需要人工确认"
          onCheckedChange={(checked) =>
            onChange({
              ...draft,
              policyToggles: { ...draft.policyToggles, newDemandNeedsHumanConfirmation: checked },
            })
          }
        />
      </div>

      <div className="flex flex-wrap gap-2">
        <StatusPill tone="info">协调策略</StatusPill>
        <StatusPill tone="ok">写入 require_human_review_for_new_demands</StatusPill>
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
    <label className="flex cursor-pointer items-start justify-between gap-4 px-4 py-3">
      <div className="min-w-0">
        <div className="text-sm font-medium text-v3-ink">{label}</div>
        <p className="mt-1 text-xs leading-5 text-v3-ink-2">{description}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onCheckedChange} />
    </label>
  );
}
