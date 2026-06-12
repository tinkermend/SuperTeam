import { AlertTriangle, ArrowUpRight, Clock, FileText } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import { Button } from "@/components/ui/button";
import { CardContent } from "@/components/ui/card";
import type { InboxAction, InboxItem, InboxViewMode } from "@/lib/api/inbox";
import { cn } from "@/lib/utils";

type InboxItemListProps = {
  items: InboxItem[];
  onAction: (item: InboxItem, action: InboxAction) => void;
  view: InboxViewMode;
};

const riskLabel: Record<string, string> = {
  blocked: "阻断",
  high: "高风险",
  low: "低风险",
  medium: "中风险",
};

const riskTone: Record<string, Tone> = {
  blocked: "danger",
  high: "danger",
  low: "neutral",
  medium: "warning",
};

const actionToneClass: Record<string, string> = {
  danger: "border-destructive/30 text-destructive hover:bg-destructive/10",
  destructive: "border-destructive/30 text-destructive hover:bg-destructive/10",
  primary: "border-primary/30 text-primary hover:bg-primary/10",
  success: "border-[color:var(--superteam-success)]/30 text-[color:var(--superteam-success)] hover:bg-[color:var(--superteam-success)]/10",
  warning: "border-[color:var(--superteam-warning)]/30 text-[color:var(--superteam-warning)] hover:bg-[color:var(--superteam-warning)]/10",
};

export function InboxItemList({ items, onAction, view }: InboxItemListProps) {
  return (
    <LiquidCard>
      <CardContent className="divide-y divide-border/70 p-0">
        {items.map((item) => {
          const contextLabel = formatContext(item);

          return (
            <article key={item.id} className="flex min-w-0 flex-col gap-3 px-4 py-4 md:px-5">
              <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div className="flex min-w-0 gap-3">
                  <SemanticIconTile tone={item.risk_level === "high" ? "danger" : "decision"} size="sm">
                    {item.risk_level === "high" ? <AlertTriangle /> : <FileText />}
                  </SemanticIconTile>
                  <div className="min-w-0 space-y-2">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <h2 className="min-w-0 text-base font-semibold tracking-normal text-foreground">
                        {item.title}
                      </h2>
                      {item.risk_level ? (
                        <StatusBadge tone={riskTone[item.risk_level] ?? "neutral"}>
                          {riskLabel[item.risk_level] ?? item.risk_level}
                        </StatusBadge>
                      ) : null}
                    </div>
                    {item.summary ? (
                      <p className="text-sm leading-6 text-muted-foreground">{item.summary}</p>
                    ) : null}
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      {contextLabel ? <span>{contextLabel}</span> : null}
                      {item.source_task_id ? <span>任务 {item.source_task_id}</span> : null}
                      <span className="inline-flex items-center gap-1">
                        <Clock className="size-3.5" />
                        {formatDateTime(item.last_activity_at)}
                      </span>
                      <a
                        className="inline-flex items-center gap-1 font-medium text-primary hover:underline"
                        href={resolveInboxHref(item)}
                      >
                        查看上下文
                        <ArrowUpRight className="size-3.5" />
                      </a>
                    </div>
                  </div>
                </div>

                {view === "mine" && item.actions.length > 0 ? (
                  <div className="flex shrink-0 flex-wrap items-center gap-2 lg:justify-end">
                    {item.actions.map((action) => (
                      <Button
                        className={cn(actionToneClass[action.tone])}
                        key={action.key}
                        onClick={() => onAction(item, action)}
                        size="sm"
                        type="button"
                        variant="outline"
                      >
                        {action.label}
                      </Button>
                    ))}
                  </div>
                ) : null}
              </div>
            </article>
          );
        })}
      </CardContent>
    </LiquidCard>
  );
}

function formatContext(item: InboxItem) {
  const projectName = readContextText(item.context, ["project_name", "project", "project_title"]);
  const sourceName = readContextText(item.context, ["source_title", "approval_title", "task_title"]);

  if (projectName && sourceName) {
    return `${projectName} / ${sourceName}`;
  }
  return projectName ?? sourceName ?? (item.source_project_id ? `项目 ${item.source_project_id}` : undefined);
}

function readContextText(context: Record<string, unknown>, keys: string[]) {
  for (const key of keys) {
    const value = context[key];
    if (typeof value === "string" && value.trim()) {
      return value.trim();
    }
  }
  return undefined;
}

function resolveInboxHref(item: InboxItem) {
  const route = typeof item.deep_link.route === "string" ? item.deep_link.route : undefined;
  const anchor = typeof item.deep_link.anchor === "string" ? item.deep_link.anchor : undefined;
  const path = route
    ? route.startsWith("/")
      ? route
      : `/${route}`
    : item.source_project_id
      ? `/projects/${encodeURIComponent(item.source_project_id)}`
      : "/inbox";

  return anchor ? `${path}#${encodeURIComponent(anchor)}` : path;
}

function formatDateTime(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  }).format(date);
}
