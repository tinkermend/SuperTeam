import { humanWaitLabel } from "@/lib/status-labels";
import {
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  UserCheck
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { Tone } from "@/components/superteam";

const accentBar: Record<Tone, string> = {
  brand: "bg-brand",
  ok: "bg-ok",
  warn: "bg-warn",
  danger: "bg-danger",
  info: "bg-info",
  mute: "bg-mute",
  artifact: "bg-artifact"
};

const softBg: Record<Tone, string> = {
  brand: "bg-brand-soft",
  ok: "bg-ok-soft",
  warn: "bg-warn-soft",
  danger: "bg-danger-soft",
  info: "bg-info-soft",
  mute: "bg-mute-soft",
  artifact: "bg-artifact-soft"
};

const numText: Record<Tone, string> = {
  brand: "text-brand-deep",
  ok: "text-ok-text",
  warn: "text-warn-text",
  danger: "text-danger-text",
  info: "text-info-text",
  mute: "text-mute-text",
  artifact: "text-artifact-text"
};

export type AutomationFactStripProps = {
  enabledCount: number;
  dueSoonCount: number;
  attentionCount: number;
  pendingDecisionCount: number;
};

export function AutomationFactStrip({
  enabledCount,
  dueSoonCount,
  attentionCount,
  pendingDecisionCount
}: AutomationFactStripProps) {
  const items = [
    {
      icon: CheckCircle2,
      label: "启用中",
      value: enabledCount,
      tone: (enabledCount > 0 ? "ok" : "mute") as Tone
},
    {
      icon: CalendarClock,
      label: "72h 内待触发",
      value: dueSoonCount,
      tone: (dueSoonCount > 0 ? "info" : "mute") as Tone
},
    {
      icon: AlertTriangle,
      label: "需关注",
      value: attentionCount,
      tone: (attentionCount > 0 ? "warn" : "mute") as Tone
},
    {
      icon: UserCheck,
      label: humanWaitLabel("automations_gate"),
      value: pendingDecisionCount,
      tone: (pendingDecisionCount > 0 ? "warn" : "mute") as Tone
},
  ];

  return (
    <dl
      aria-label="自动化运营摘要"
      className="grid grid-cols-2 gap-3 @3xl/master-detail:grid-cols-4"
      data-testid="automations-fact-strip"
    >
      {items.map((item) => {
        const Icon = item.icon;
        return (
          <div
            key={item.label}
            className="relative flex items-center gap-3 overflow-hidden rounded-[14px] border border-line bg-card px-3.5 py-3 shadow-sm"
          >
            <span
              aria-hidden
              className={cn("absolute inset-y-0 left-0 w-0.5", accentBar[item.tone])}
            />
            <div
              className={cn(
                "flex size-8 shrink-0 items-center justify-center rounded-full",
                softBg[item.tone],
              )}
            >
              <Icon aria-hidden className={cn("size-[15px]", numText[item.tone])} />
            </div>
            <div className="min-w-0">
              <dd
                className={cn(
                  "text-xl font-extrabold leading-none tabular-nums",
                  numText[item.tone],
                )}
              >
                {item.value}
              </dd>
              <dt className="mt-1 text-[11.5px] font-medium text-ink-3">{item.label}</dt>
            </div>
          </div>
        );
      })}
    </dl>
  );
}
