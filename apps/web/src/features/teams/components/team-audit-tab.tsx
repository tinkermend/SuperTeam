import { useQuery } from "@tanstack/react-query";
import {
  FileClock,
  GitCompareArrows,
  Link2,
  ShieldX,
  UserRound,
  Users,
  type LucideIcon,
} from "lucide-react";
import { useMemo, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { listTeamAuditEvents, type TeamAuditEvent } from "@/lib/api/teams";

const AUDIT_LIMIT = 20;

type TeamAuditTabProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teamId: string;
};

export function TeamAuditTab({ apiBaseUrl, fetcher, teamId }: TeamAuditTabProps) {
  const [selectedEventId, setSelectedEventId] = useState("");
  const options = { baseUrl: apiBaseUrl, fetcher };
  const auditEvents = useQuery({
    queryKey: ["team-audit-events", teamId],
    queryFn: () => listTeamAuditEvents(options, teamId, { limit: AUDIT_LIMIT, offset: 0 }),
  });
  const events = useMemo(
    () => (auditEvents.data ?? []).filter((event) => event.action.startsWith("team.")),
    [auditEvents.data],
  );
  const selectedEvent = selectedEventId ? events.find((event) => event.id === selectedEventId) : undefined;
  const todayCount = events.filter((event) => isToday(event.created_at)).length;
  const memberChangeCount = events.filter((event) => event.action.includes(".member.") || event.action.includes(".role.")).length;
  const governanceCount = events.filter((event) => event.action.includes(".governance.")).length;
  const capabilityCount = events.filter((event) => event.action.includes(".capability.")).length;
  const rejectedCount = events.filter((event) => resultValue(event) === "rejected").length;

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="flex min-w-0 flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <MetricTile icon={FileClock} label="今日操作" value={todayCount} detail="24 小时内" />
          <MetricTile icon={Users} label="成员变更" value={memberChangeCount} detail="成员与角色" />
          <MetricTile icon={GitCompareArrows} label="治理版本" value={governanceCount} detail="草案与审批" />
          <MetricTile icon={Link2} label="能力绑定" value={capabilityCount} detail="外部能力" />
          <MetricTile icon={ShieldX} label="被拒绝" value={rejectedCount} detail="需复核" />
        </div>

        <Card>
          <CardHeader className="border-b">
            <CardTitle>团队管理审计</CardTitle>
          </CardHeader>
          <CardContent className="pt-4">
            {auditEvents.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {auditEvents.isError ? <p className="text-sm text-destructive">审计记录加载失败</p> : null}
            {!auditEvents.isLoading && !auditEvents.isError && events.length === 0 ? (
              <p className="text-sm text-muted-foreground">暂无团队管理审计记录</p>
            ) : null}
            {events.length > 0 ? (
              <div className="w-full overflow-x-auto rounded-md border">
                <Table className="min-w-[1040px]">
                  <TableHeader>
                    <TableRow>
                      <TableHead>时间</TableHead>
                      <TableHead>操作者</TableHead>
                      <TableHead>操作</TableHead>
                      <TableHead>资源</TableHead>
                      <TableHead>结果</TableHead>
                      <TableHead>授权动作</TableHead>
                      <TableHead className="min-w-60">审计摘要</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {events.map((event) => (
                      <AuditTableRow event={event} key={event.id} onSelect={() => setSelectedEventId(event.id)} />
                    ))}
                  </TableBody>
                </Table>
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <AuditDetailPanel event={selectedEvent} />
    </div>
  );
}

function AuditTableRow({ event, onSelect }: { event: TeamAuditEvent; onSelect: () => void }) {
  const isRejected = resultValue(event) === "rejected";

  return (
    <TableRow className="cursor-pointer" onClick={onSelect} tabIndex={0}>
      <TableCell>{formatDateTime(event.created_at)}</TableCell>
      <TableCell>
        <div className="flex min-w-0 items-center gap-2">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted">
            <UserRound />
          </div>
          <span className="truncate">{event.actor_id}</span>
        </div>
      </TableCell>
      <TableCell>{actionLabel(event.action)}</TableCell>
      <TableCell>{resourceLabel(event)}</TableCell>
      <TableCell>
        <Badge variant={isRejected ? "destructive" : "secondary"}>{isRejected ? "拒绝" : "成功"}</Badge>
      </TableCell>
      <TableCell>
        <Badge variant="outline">{authorizationAction(event)}</Badge>
      </TableCell>
      <TableCell>
        <span className="line-clamp-2">{summaryText(event)}</span>
      </TableCell>
    </TableRow>
  );
}

function AuditDetailPanel({ event }: { event?: TeamAuditEvent }) {
  if (!event) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>事件详情</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">选择一条审计记录查看详情。</p>
        </CardContent>
      </Card>
    );
  }

  const before = event.details.before;
  const after = event.details.after;

  return (
    <Card>
      <CardHeader className="border-b">
        <CardTitle>事件详情</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 pt-4 text-sm">
        <DetailItem label="事件时间" value={formatDateTime(event.created_at)} />
        <DetailItem label="操作" value={actionLabel(event.action)} />
        <DetailItem label="结果" value={resultValue(event) === "rejected" ? "拒绝" : "成功"} />
        <DetailItem label="操作者" value={`${event.actor_id}（${event.actor_type}）`} />
        <DetailItem label="资源" value={`${event.resource_type} / ${resourceLabel(event)}`} />
        <DetailItem label="授权动作" value={authorizationAction(event)} prefix="授权动作：" />

        <div className="flex flex-col gap-2 border-t pt-4">
          <p className="font-medium">变更内容（前后对比）</p>
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-1">
            <JsonSnapshot title="变更前" value={before} />
            <JsonSnapshot title="变更后" value={after} />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

function MetricTile({
  detail,
  icon: Icon,
  label,
  value,
}: {
  detail: string;
  icon: LucideIcon;
  label: string;
  value: number;
}) {
  return (
    <Card aria-label={`${label} ${value} ${detail}`} className="gap-3 py-4" role="group">
      <CardContent className="flex items-center gap-3 px-4">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted">
          <Icon />
        </div>
        <div className="min-w-0">
          <p className="truncate text-sm text-muted-foreground">{label}</p>
          <p className="text-xl font-semibold">{value}</p>
          <p className="truncate text-xs text-muted-foreground">{detail}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function DetailItem({ label, prefix, value }: { label: string; prefix?: string; value: string }) {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span>{prefix ? `${prefix}${value}` : value}</span>
    </div>
  );
}

function JsonSnapshot({ title, value }: { title: string; value: unknown }) {
  const lines = formatJson(value).split("\n");

  return (
    <div className="flex min-w-0 flex-col gap-2 rounded-md border bg-muted/30 p-3">
      <p className="text-xs text-muted-foreground">{title}</p>
      <pre className="flex flex-col gap-1 overflow-x-auto text-xs">
        {lines.map((line, index) => (
          <span key={`${title}-${index}`}>{line}</span>
        ))}
      </pre>
    </div>
  );
}

function isToday(value?: string) {
  if (!value) {
    return false;
  }
  const eventDate = new Date(value);
  const now = new Date();

  return (
    eventDate.getFullYear() === now.getFullYear() &&
    eventDate.getMonth() === now.getMonth() &&
    eventDate.getDate() === now.getDate()
  );
}

function formatDateTime(value?: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return date.toLocaleString("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    year: "numeric",
  });
}

function actionLabel(action: string) {
  const labels: Record<string, string> = {
    "team.archive.confirm": "归档确认",
    "team.capability.bind": "绑定能力",
    "team.create": "团队创建",
    "team.governance.approve": "治理审批",
    "team.governance.edit": "治理编辑",
    "team.member.add": "添加成员",
    "team.role.approve": "批准角色",
    "team.role.request": "角色申请",
    "team.update": "基础信息编辑",
  };

  return labels[action] ?? action;
}

function resultValue(event: TeamAuditEvent) {
  const result = event.details.result;
  return typeof result === "string" ? result : "success";
}

function authorizationAction(event: TeamAuditEvent) {
  const value = event.details.authorization_action;
  return typeof value === "string" && value ? value : event.action;
}

function resourceLabel(event: TeamAuditEvent) {
  const value = event.details.resource_label;
  return typeof value === "string" && value ? value : event.resource_id;
}

function summaryText(event: TeamAuditEvent) {
  const value = event.details.summary;
  return typeof value === "string" && value ? value : event.action;
}

function formatJson(value: unknown) {
  if (value === undefined || value === null) {
    return "-";
  }

  return JSON.stringify(value, null, 2);
}
