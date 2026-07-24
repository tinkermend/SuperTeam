import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ShieldCheck, Boxes, Plug, ScrollText } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { CreateTeamDraft } from "./create-team-draft";
import { TeamIconTile } from "@/components/superteam/team-icon-tile";

export function CreateTeamStepReview({
  draft,
  goToConstitution,
  setGoToConstitution
}: {
  draft: CreateTeamDraft;
  goToConstitution: boolean;
  setGoToConstitution: (val: boolean) => void;
}) {
  const previewName = draft.name.trim() || "未命名团队";
  const previewSlug = draft.slug.trim() || "team-slug";
  const previewDescription = draft.description.trim();

  return (
    <div className="flex flex-col gap-5 lg:mx-auto lg:w-full lg:max-w-3xl">
      <div className="overflow-hidden rounded-[22px] border border-line bg-card shadow-card">
        <div className="bg-brand px-5 py-5 text-white sm:px-6">
          <p className="text-xs font-semibold tracking-wide text-white/80">创建确认预览</p>
          <div className="mt-3 flex items-center gap-3">
            <TeamIconTile
              className="size-12 rounded-[14px] border-white/40 bg-white/90 [&_svg]:size-5"
              metadata={{ display: draft.display }}
            />
            <div className="min-w-0">
              <div className="truncate text-base font-bold">{previewName}</div>
              <div className="truncate font-mono text-xs text-white/85">
                /teams/{previewSlug}
              </div>
            </div>
          </div>
          <p className="mt-3 line-clamp-2 max-w-2xl text-sm leading-5 text-white/85">
            {previewDescription || "暂未填写团队说明"}
          </p>
          <div className="mt-4 flex gap-8 border-t border-white/20 pt-3">
            <PreviewStat label="负责人" value={draft.owners.length} />
            <PreviewStat label="数字员工" value={draft.initial_digital_employees.length} />
          </div>
        </div>
      </div>

      <div className="rounded-[22px] border border-line bg-card p-4 shadow-card sm:p-5">
        <h3 className="text-sm font-semibold text-ink">创建后解锁</h3>
        <p className="mb-3 mt-1 text-xs text-ink-3">
          下列能力在团队创建后开放，可按需逐步配置：
        </p>
        <ul className="flex flex-col">
          <LifecycleRow
            desc="章程 · 能力策略 · 审批策略 · 工件契约"
            icon={<ShieldCheck className="size-4" />}
            state="创建后配置"
            stateTone="neutral"
            title="宪法"
            tone="text-emerald-600 bg-emerald-500/10"
          />
          <LifecycleRow
            desc="从员工池调度可执行的数字员工"
            icon={<Boxes className="size-4" />}
            state={`${draft.initial_digital_employees.length} 预选`}
            stateTone="neutral"
            title="数字员工"
            tone="text-teal-600 bg-teal-500/10"
          />
          <LifecycleRow
            desc="绑定 HTTP 连接器与外部系统能力"
            icon={<Plug className="size-4" />}
            state="0 已绑定"
            stateTone="neutral"
            title="外部能力"
            tone="text-amber-600 bg-amber-500/10"
          />
          <LifecycleRow
            desc="团队内所有操作自动留痕"
            icon={<ScrollText className="size-4" />}
            state="自动开启"
            stateTone="neutral"
            title="审计日志"
            tone="text-slate-500 bg-slate-500/10"
          />
        </ul>
      </div>

      <div className="rounded-[22px] border border-line bg-card p-5 shadow-card">
        <Label className="flex cursor-pointer items-start gap-2.5">
          <Checkbox
            checked={goToConstitution}
            className="mt-0.5"
            onCheckedChange={(checked) => setGoToConstitution(checked === true)}
          />
          <span className="text-sm text-ink">
            <span className="font-medium">创建后前往宪法</span>
            <span className="mt-0.5 block text-xs text-ink-3">
              立即进入团队宪法编辑。
            </span>
          </span>
        </Label>
      </div>
    </div>
  );
}

function PreviewStat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="text-lg font-bold">{value}</div>
      <div className="text-xs opacity-85">{label}</div>
    </div>
  );
}

function LifecycleRow({
  desc,
  icon,
  state,
  stateTone,
  title,
  tone
}: {
  desc: string;
  icon: ReactNode;
  state: string;
  stateTone: "neutral" | "warning";
  title: string;
  tone: string;
}) {
  return (
    <li className="flex gap-3 border-t py-3 first:border-t-0">
      <span
        className={cn(
          "flex size-8 flex-none items-center justify-center rounded-lg",
          tone,
        )}
      >
        {icon}
      </span>
      <div className="min-w-0">
        <div className="text-sm font-medium">{title}</div>
        <div className="mt-0.5 text-xs text-muted-foreground">{desc}</div>
      </div>
      <span
        className={cn(
          "ml-auto self-center whitespace-nowrap rounded-full border px-2.5 py-0.5 text-xs font-medium",
          stateTone === "warning"
            ? "border-amber-500/30 bg-amber-500/10 text-amber-700"
            : "border-slate-300/60 bg-slate-500/10 text-slate-600",
        )}
      >
        {state}
      </span>
    </li>
  );
}
