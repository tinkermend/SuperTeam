import { useState, type ReactNode } from "react";
import { StatusPill, type V3Tone } from "@/components/superteam";
import type { DigitalEmployeeRunEvent } from "@/lib/api/employees";

type RunEventTimelineProps = {
  events: DigitalEmployeeRunEvent[];
  /** 事件数达到查询上限(50)时由调用方置 true,底部渲染截断提示。 */
  limitReached?: boolean;
};

type MarkerItem = {
  kind: "marker";
  key: string;
  label: string;
  tone: V3Tone;
  detail?: string;
  mono?: boolean;
  events: DigitalEmployeeRunEvent[];
};

type TextItem = { kind: "text"; key: string; text: string; events: DigitalEmployeeRunEvent[] };

type ToolItem = {
  kind: "tool";
  key: string;
  toolId: string;
  name?: string;
  status: "running" | "ok" | "error";
  inputExcerpt?: string;
  outputExcerpt?: string;
  truncated: boolean;
  events: DigitalEmployeeRunEvent[];
};

type ErrorItem = { kind: "error"; key: string; message: string; events: DigitalEmployeeRunEvent[] };

type UnknownItem = { kind: "unknown"; key: string; eventType: string; events: DigitalEmployeeRunEvent[] };

type TimelineItem = MarkerItem | TextItem | ToolItem | ErrorItem | UnknownItem;

const LIFECYCLE_MARKERS: Record<string, { label: string; tone: V3Tone }> = {
  run_cancelled: { label: "运行已取消", tone: "warn" },
  run_completed: { label: "运行完成", tone: "ok" },
  run_dispatched: { label: "命令已下发", tone: "info" },
  run_failed: { label: "运行失败", tone: "danger" },
  run_reaped_stale: { label: "运行已被回收", tone: "danger" },
  run_timed_out: { label: "运行已超时", tone: "danger" },
};

export function RunEventTimeline({ events, limitReached }: RunEventTimelineProps) {
  const items = buildTimeline(events);
  return (
    <div className="space-y-2">
      {items.map((item) => (
        <TimelineRow item={item} key={item.key} />
      ))}
      {limitReached ? <p className="text-xs text-v3-ink-3">仅显示前 {events.length} 条事件。</p> : null}
    </div>
  );
}

function stringField(payload: Record<string, unknown> | undefined, key: string): string | undefined {
  const value = payload?.[key];
  return typeof value === "string" ? value : undefined;
}

function buildTimeline(events: DigitalEmployeeRunEvent[]): TimelineItem[] {
  const sorted = [...events].sort((a, b) => a.sequence_number - b.sequence_number);
  const items: TimelineItem[] = [];
  // tool_completed 与最近一个同 tool_id 的 tool_started 合并为一行。
  const openTools = new Map<string, ToolItem>();

  for (const event of sorted) {
    const payload = event.payload ?? {};
    switch (event.event_type) {
      case "session_started": {
        items.push({
          kind: "marker",
          key: `event-${event.sequence_number}`,
          label: "会话已建立",
          tone: "info",
          detail: stringField(payload, "session_id"),
          mono: true,
          events: [event],
        });
        break;
      }
      case "turn_started": {
        items.push({
          kind: "marker",
          key: `event-${event.sequence_number}`,
          label: "回合开始",
          tone: "mute",
          events: [event],
        });
        break;
      }
      case "text_delta": {
        const text = stringField(payload, "text") ?? "";
        const last = items[items.length - 1];
        if (last?.kind === "text") {
          last.text += text;
          last.events.push(event);
        } else if (text) {
          // 空 text_delta 且无前置正文块时不产出空白卡片。
          items.push({ kind: "text", key: `event-${event.sequence_number}`, text, events: [event] });
        }
        break;
      }
      case "tool_started": {
        const toolId = stringField(payload, "tool_id") ?? `tool-${event.sequence_number}`;
        const item: ToolItem = {
          kind: "tool",
          key: `event-${event.sequence_number}`,
          toolId,
          name: stringField(payload, "name"),
          status: "running",
          inputExcerpt: stringField(payload, "input_excerpt"),
          truncated: payload.input_truncated === true,
          events: [event],
        };
        items.push(item);
        openTools.set(toolId, item);
        break;
      }
      case "tool_completed": {
        const toolId = stringField(payload, "tool_id") ?? `tool-${event.sequence_number}`;
        const status: ToolItem["status"] = payload.is_error === true ? "error" : "ok";
        const open = openTools.get(toolId);
        if (open) {
          open.status = status;
          open.outputExcerpt = stringField(payload, "output_excerpt");
          open.truncated = open.truncated || payload.output_truncated === true;
          open.events.push(event);
          openTools.delete(toolId);
        } else {
          // 前 50 条上限截断可能只留下 completed 一半,按独立工具行展示。
          items.push({
            kind: "tool",
            key: `event-${event.sequence_number}`,
            toolId,
            status,
            outputExcerpt: stringField(payload, "output_excerpt"),
            truncated: payload.output_truncated === true,
            events: [event],
          });
        }
        break;
      }
      case "turn_completed": {
        const summary = stringField(payload, "summary");
        // usage 目前不入 payload(runtime 侧丢弃),防御性读取以兼容未来回写。
        const usage = payload.usage;
        const totalTokens =
          usage && typeof usage === "object" && typeof (usage as Record<string, unknown>).total_tokens === "number"
            ? ((usage as Record<string, unknown>).total_tokens as number)
            : undefined;
        const detailParts = [
          summary,
          totalTokens !== undefined ? `${totalTokens.toLocaleString("zh-CN")} tokens` : undefined,
        ].filter((part): part is string => Boolean(part));
        items.push({
          kind: "marker",
          key: `event-${event.sequence_number}`,
          label: "回合完成",
          tone: "ok",
          detail: detailParts.length ? detailParts.join(" · ") : undefined,
          events: [event],
        });
        break;
      }
      case "turn_error": {
        items.push({
          kind: "error",
          key: `event-${event.sequence_number}`,
          message: stringField(payload, "message") ?? "回合执行出错",
          events: [event],
        });
        break;
      }
      default: {
        const marker = LIFECYCLE_MARKERS[event.event_type];
        if (marker) {
          items.push({
            kind: "marker",
            key: `event-${event.sequence_number}`,
            label: marker.label,
            tone: marker.tone,
            events: [event],
          });
        } else {
          items.push({
            kind: "unknown",
            key: `event-${event.sequence_number}`,
            eventType: event.event_type,
            events: [event],
          });
        }
      }
    }
  }
  return items;
}

function TimelineRow({ item }: { item: TimelineItem }) {
  switch (item.kind) {
    case "marker":
      return (
        <TimelineCard events={item.events}>
          <div className="flex flex-wrap items-center gap-2">
            <StatusPill tone={item.tone}>{item.label}</StatusPill>
            {item.detail ? (
              <span
                className={
                  item.mono ? "break-all font-mono text-xs text-v3-ink-2" : "text-xs text-v3-ink-2"
                }
              >
                {item.detail}
              </span>
            ) : null}
          </div>
        </TimelineCard>
      );
    case "text":
      return (
        <TimelineCard events={item.events}>
          <p className="whitespace-pre-wrap break-words text-sm leading-6 text-v3-ink">{item.text}</p>
        </TimelineCard>
      );
    case "tool":
      return (
        <TimelineCard events={item.events}>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-[11px] text-v3-ink-3">工具调用</span>
            <span className="font-mono text-xs text-v3-ink">{item.name ?? item.toolId}</span>
            <StatusPill tone={item.status === "error" ? "danger" : item.status === "ok" ? "ok" : "mute"}>
              {item.status === "error" ? "失败" : item.status === "ok" ? "成功" : "运行中"}
            </StatusPill>
          </div>
          {item.inputExcerpt ? <ExcerptBlock label="输入" value={item.inputExcerpt} /> : null}
          {item.outputExcerpt ? <ExcerptBlock label="输出" value={item.outputExcerpt} /> : null}
          {item.truncated ? <p className="mt-1 text-[11px] text-v3-ink-3">内容已截断。</p> : null}
        </TimelineCard>
      );
    case "error":
      return (
        <TimelineCard events={item.events}>
          <div className="rounded-v3-inner bg-v3-danger-soft p-2">
            <p className="text-xs font-medium text-v3-danger">回合出错</p>
            <p className="mt-1 whitespace-pre-wrap break-words text-xs text-v3-ink-2">{item.message}</p>
          </div>
        </TimelineCard>
      );
    case "unknown":
      return (
        <TimelineCard events={item.events}>
          <StatusPill tone="mute">{item.eventType}</StatusPill>
        </TimelineCard>
      );
  }
}

function TimelineCard({ children, events }: { children: ReactNode; events: DigitalEmployeeRunEvent[] }) {
  const [rawOpen, setRawOpen] = useState(false);
  return (
    <div className="rounded-md border border-v3-line px-3 py-2">
      {children}
      <details className="mt-1" onToggle={(event) => setRawOpen(event.currentTarget.open)}>
        <summary className="cursor-pointer text-[11px] text-v3-ink-3">原始 JSON</summary>
        {rawOpen ? (
          <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-v3-inner bg-v3-card-soft p-2 font-mono text-xs text-v3-ink-2">
            {JSON.stringify(events.length === 1 ? events[0] : events, null, 2)}
          </pre>
        ) : null}
      </details>
    </div>
  );
}

function ExcerptBlock({ label, value }: { label: string; value: string }) {
  return (
    <details className="mt-1 min-w-0">
      <summary className="cursor-pointer text-[11px] text-v3-ink-3">{label}</summary>
      <pre className="mt-1 max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-v3-inner bg-v3-card-soft p-2 font-mono text-xs text-v3-ink-2">
        {value}
      </pre>
    </details>
  );
}
