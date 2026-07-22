import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import type { AutomationCoordinationMode } from "@/lib/api/automations";

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

type HumanGateCalloutProps = {
  mode: AutomationCoordinationMode;
  className?: string;
};

export function HumanGateCallout({ mode, className }: HumanGateCalloutProps) {
  const copy = GATE_COPY[mode];
  return (
    <Alert className={className}>
      <AlertTitle>{copy.title}</AlertTitle>
      <AlertDescription>
        <ul className="mt-1 list-disc space-y-1 pl-4 text-sm">
          {copy.lines.map((line) => (
            <li key={line}>{line}</li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}
