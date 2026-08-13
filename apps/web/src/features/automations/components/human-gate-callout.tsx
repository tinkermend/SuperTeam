import { Callout } from "@/components/superteam";
import type {
  AutomationAutonomyTier,
  AutomationCoordinationMode,
} from "@/lib/api/automations";

const GATE_COPY: Record<
  AutomationCoordinationMode,
  { title: string; lines: string[] }
> = {
  loop: {
    title: "自动触发 ≠ 无人值守",
    lines: [
      "到点后按 Loop 模式发起需求，中间尽量自治补闭合。",
      "终态验收仍需人类处理（Console 或飞书投影）。",
      "执行中若出现规划缺口等闸门，仍会停等人工。",
    ],
  },
  plan: {
    title: "自动触发 ≠ 无人值守",
    lines: [
      "到点后按 Plan 模式发起需求。",
      "通常需要计划确认后才派发；确认可在飞书审批卡或 Console 完成。",
      "终态验收仍需人类处理。",
    ],
  },
  chat: {
    title: "定时对话，不进项目验收",
    lines: [
      "到点后对指定数字员工发起对话 run。",
      "不进入项目协调线程，不触发需求验收。",
      "若要进项目闭环，需另行转为任务或配置 Demand 规则。",
    ],
  },
};

const AUTONOMY_LINES: Record<AutomationAutonomyTier, string> = {
  pause_at_gate: "自治档位：遇闸暂停——撞人闸时进收件箱等人。",
  full_auto:
    "自治档位：完全自动化——闸照触发，决策由策略自动放行（resolved_by=policy:规则），验收闸仍不无条件自动通过。",
};

type HumanGateCalloutProps = {
  mode: AutomationCoordinationMode;
  autonomyTier?: AutomationAutonomyTier;
  className?: string;
};

export function HumanGateCallout({
  mode,
  autonomyTier = "pause_at_gate",
  className,
}: HumanGateCalloutProps) {
  const copy = GATE_COPY[mode];
  return (
    <Callout className={className} tone="info" title={copy.title}>
      <ul className="list-disc space-y-1 pl-4">
        <li>{AUTONOMY_LINES[autonomyTier]}</li>
        {copy.lines.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ul>
    </Callout>
  );
}
