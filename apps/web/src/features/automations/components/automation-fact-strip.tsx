import {
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  UserCheck,
} from "lucide-react";
import { cn } from "@/lib/utils";
import type { V3Tone } from "@/components/superteam";

const accentBar: Record<V3Tone, string> = {
  brand: "bg-v3-brand",
  ok: "bg-v3-ok",
  warn: "bg-v3-warn",
  danger: "bg-v3-danger",
  info: "bg-v3-info",
  mute: "bg-v3-mute",
  artifact: "bg-v3-artifact",
};

const softBg: Record<V3Tone, string> = {
  brand: "bg-v3-brand-soft",
  ok: "bg-v3-ok-soft",
  warn: "bg-v3-warn-soft",
  danger: "bg-v3-danger-soft",
  info: "bg-v3-info-soft",
  mute: "bg-v3-mute-soft",
  artifact: "bg-v3-artifact-soft",
};

const numText: Record<V3Tone, string> = {
  brand: "text-v3-brand-deep",
  ok: "text-v3-ok-text",
  warn: "text-v3-warn-text",
  danger: "text-v3-danger-text",
  info: "text-v3-info-text",
  mute: "text-v3-mute-text",
  artifact: "text-v3-artifact-text",
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
  pendingDecisionCount,
}: AutomationFactStripProps) {
  const items = [
    {
      icon: CheckCircle2,
      label: "启用中",
      value: enabledCount,
      tone: (enabledCount > 0 ? "ok" : "mute") as V3Tone,
    },
    {
      icon: CalendarClock,
      label: "72h 内待触发",
      value: dueSoonCount,
      tone: (dueSoonCount > 0 ? "info" : "mute") as V3Tone,
    },
    {
      icon: AlertTriangle,
      label: "需关注",
      value: attentionCount,
      tone: (attentionCount > 0 ? "warn" : "mute") as V3Tone,
    },
    {
      icon: UserCheck,
      label: "待你处理",
      value: pendingDecisionCount,
      tone: (pendingDecisionCount > 0 ? "warn" : "mute") as V3Tone,
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
            className="relative flex items-center gap-3 overflow-hidden rounded-[14px] border border-v3-line bg-v3-card px-3.5 py-3 shadow-sm"
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
              <dt className="mt-1 text-[11.5px] font-medium text-v3-ink-3">{item.label}</dt>
            </div>
          </div>
        );
      })}
    </dl>
  );
}
