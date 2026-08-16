import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { MarkdownProse } from "@/components/superteam";
import {
  listDigitalEmployeeRunEvents,
  type DigitalEmployeeRunEvent,
} from "@/lib/api/employees";
import type { ApiClientOptions } from "@/lib/api/client";

const EXCERPT_LIMIT = 800;

export type LiveTool = {
  key: string;
  name: string;
  status: "running" | "ok" | "error";
  inputExcerpt?: string;
  outputExcerpt?: string;
};

type LiveText = { key: string; text: string };

function stringField(payload: Record<string, unknown> | undefined, key: string): string | undefined {
  const value = payload?.[key];
  return typeof value === "string" ? value : undefined;
}

function clipExcerpt(value: string | undefined): string | undefined {
  if (!value?.trim()) {
    return undefined;
  }
  const trimmed = value.trim();
  if (trimmed.length <= EXCERPT_LIMIT) {
    return trimmed;
  }
  return `${trimmed.slice(0, EXCERPT_LIMIT)}…`;
}

/** 对话面只投影工具与回答文本；其它事件类型忽略（产品选择，不在 UI 解释未映射的上游块）。 */
export function buildChatLiveItems(events: DigitalEmployeeRunEvent[]): {
  texts: LiveText[];
  tools: LiveTool[];
} {
  const sorted = [...events].sort((a, b) => a.sequence_number - b.sequence_number);
  const texts: LiveText[] = [];
  const tools: LiveTool[] = [];
  const openTools = new Map<string, LiveTool>();

  for (const event of sorted) {
    const payload = event.payload ?? {};
    if (event.event_type === "text_delta") {
      const text = stringField(payload, "text") ?? "";
      const last = texts[texts.length - 1];
      if (last) {
        last.text += text;
      } else if (text) {
        texts.push({ key: `text-${event.sequence_number}`, text });
      }
      continue;
    }
    if (event.event_type === "tool_started") {
      const toolId = stringField(payload, "tool_id") ?? `tool-${event.sequence_number}`;
      const item: LiveTool = {
        key: `tool-${event.sequence_number}`,
        name: stringField(payload, "name") ?? toolId,
        status: "running",
        inputExcerpt: clipExcerpt(stringField(payload, "input_excerpt")),
      };
      tools.push(item);
      openTools.set(toolId, item);
      continue;
    }
    if (event.event_type === "tool_completed") {
      const toolId = stringField(payload, "tool_id") ?? `tool-${event.sequence_number}`;
      const status: LiveTool["status"] = payload.is_error === true ? "error" : "ok";
      const outputExcerpt = clipExcerpt(stringField(payload, "output_excerpt"));
      const open = openTools.get(toolId);
      if (open) {
        open.status = status;
        open.outputExcerpt = outputExcerpt;
        openTools.delete(toolId);
      } else {
        tools.push({
          key: `tool-${event.sequence_number}`,
          name: toolId,
          status,
          outputExcerpt,
        });
      }
    }
  }
  return { texts, tools };
}

export function chatLiveStatusLine(tools: LiveTool[], liveText: string): string {
  const running = [...tools].reverse().find((tool) => tool.status === "running");
  if (running) {
    return `正在调用 ${running.name}`;
  }
  if (liveText.trim()) {
    return "正在写出回答…";
  }
  if (tools.length > 0) {
    return "正在整理回答…";
  }
  return "正在执行…";
}

function ToolRows({
  expanded,
  onToggle,
  tools,
}: {
  expanded: Set<string>;
  onToggle: (key: string) => void;
  tools: LiveTool[];
}) {
  return (
    <>
      {tools.map((tool) => {
        const open = tool.status === "running" || expanded.has(tool.key);
        return (
          <div className="hub-tool" key={tool.key}>
            <button
              className={tool.status === "running" ? "hub-tool-row is-running" : "hub-tool-row"}
              onClick={() => onToggle(tool.key)}
              type="button"
            >
              {tool.status === "running" ? (
                <Loader2 aria-hidden className="size-3.5 animate-spin" />
              ) : (
                <span className="hub-tool-dot" data-status={tool.status} />
              )}
              <span className="hub-tool-name">{tool.name}</span>
              <span className="hub-tool-state">
                {tool.status === "running" ? "进行中" : tool.status === "error" ? "失败" : "完成"}
              </span>
            </button>
            {open ? (
              <div className="hub-tool-excerpts">
                {tool.inputExcerpt ? (
                  <pre className="hub-tool-excerpt">
                    <span className="hub-tool-excerpt-label">输入</span>
                    {tool.inputExcerpt}
                  </pre>
                ) : null}
                {tool.outputExcerpt ? (
                  <pre className="hub-tool-excerpt">
                    <span className="hub-tool-excerpt-label">输出</span>
                    {tool.outputExcerpt}
                  </pre>
                ) : null}
                {!tool.inputExcerpt && !tool.outputExcerpt ? (
                  <p className="hub-tool-excerpt-empty">没有可展示的输入/输出摘录</p>
                ) : null}
              </div>
            ) : null}
          </div>
        );
      })}
    </>
  );
}

export function ChatLiveProcess({
  apiOptions,
  employeeId,
  pollIntervalMs,
  runId,
  variant = "live",
}: {
  apiOptions: ApiClientOptions;
  employeeId: string;
  pollIntervalMs: number | false;
  runId: string;
  variant?: "live" | "settled";
}) {
  const queryClient = useQueryClient();
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [summaryOpen, setSummaryOpen] = useState(false);
  const cached = queryClient.getQueryData<DigitalEmployeeRunEvent[]>([
    "chat-run-events",
    employeeId,
    runId,
  ]);
  const eventsQuery = useQuery({
    enabled:
      Boolean(employeeId && runId) &&
      (variant === "live" || summaryOpen || cached != null),
    queryFn: () => listDigitalEmployeeRunEvents(apiOptions, employeeId, runId, { limit: 200 }),
    queryKey: ["chat-run-events", employeeId, runId],
    refetchInterval: variant === "live" ? pollIntervalMs : false,
    staleTime: variant === "settled" ? 60_000 : 0,
  });
  const { texts, tools } = buildChatLiveItems(eventsQuery.data ?? cached ?? []);
  const liveText = texts.map((item) => item.text).join("");
  const statusLine = chatLiveStatusLine(tools, liveText);

  function toggle(key: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }
      return next;
    });
  }

  if (variant === "settled") {
    if (tools.length === 0) {
      return null;
    }
    return (
      <div className="hub-live hub-live-settled">
        <button
          aria-expanded={summaryOpen}
          className="hub-process-chip"
          onClick={() => setSummaryOpen((open) => !open)}
          type="button"
        >
          工具 · {tools.length}
        </button>
        {summaryOpen ? <ToolRows expanded={expanded} onToggle={toggle} tools={tools} /> : null}
      </div>
    );
  }

  return (
    <div className="hub-live">
      <ToolRows expanded={expanded} onToggle={toggle} tools={tools} />
      {liveText.trim() ? (
        <MarkdownProse className="hub-md hub-md-live" streaming>
          {liveText}
        </MarkdownProse>
      ) : null}
      <p aria-live="polite" className="hub-agent-pending">
        {statusLine}
      </p>
    </div>
  );
}
