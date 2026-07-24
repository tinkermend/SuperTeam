import { Link } from "@tanstack/react-router";
import { Play, Pencil, Power, Trash2, X } from "lucide-react";
import {
  SoftCard,
  StatusPill,
  Button,
  EmptyState,
  LoadingState,
  DataTable,
  Td,
  Th,
  Tr
} from "@/components/superteam";
import {
  formatAutomationScheduleSummary,
  type AutomationFire,
  type AutomationRule
} from "@/lib/api/automations";
import { formatRelativeFuture } from "@/lib/format-time";
import { statusLabel } from "@/lib/status-labels";
import { automationFireTone } from "../fire-tone";
import { HumanGateCallout } from "./human-gate-callout";

function disabledReasonLabel(reason: string | null | undefined): string {
  if (!reason) return "";
  return statusLabel(reason);
}

type AutomationRuleDetailProps = {
  rule: AutomationRule;
  fires: AutomationFire[];
  firesLoading?: boolean;
  actionError?: string | null;
  busy?: boolean;
  /** Precomputed next fire ISO from page-level map; avoids per-render cron scan. */
  nextFireIso?: string | null;
  onDisable: () => void;
  onEnable: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onTrigger: () => void;
  onClose?: () => void;
};

export function AutomationRuleDetail({
  rule,
  fires,
  firesLoading,
  actionError,
  busy,
  nextFireIso,
  onDisable,
  onEnable,
  onEdit,
  onDelete,
  onTrigger,
  onClose
}: AutomationRuleDetailProps) {
  const projectLabel = rule.project_name?.trim() || rule.project_id;
  const nextIso = nextFireIso ?? null;

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <SoftCard className="space-y-3 p-4">
        <div className="space-y-1">
          <div className="flex items-start justify-between gap-2">
            <h2 className="text-base font-extrabold text-ink">{rule.name}</h2>
            {onClose ? (
              <Button
                aria-label="返回工作台"
                size="icon"
                type="button"
                variant="outline"
                onClick={onClose}
              >
                <X className="size-4" />
              </Button>
            ) : null}
          </div>
          <p className="text-sm text-muted-foreground">
            项目{" "}
            <Link
              className="text-primary underline-offset-2 hover:underline"
              params={{ projectId: rule.project_id }}
              to="/projects/$projectId"
            >
              {projectLabel}
            </Link>
            {" · "}
            {statusLabel(rule.coordination_mode)}
            {" · "}
            {formatAutomationScheduleSummary(rule)}
          </p>
          <div className="flex flex-wrap items-center gap-2 pt-1">
            <StatusPill tone={rule.enabled ? "ok" : "mute"}>
              {rule.enabled ? "启用中" : "已禁用"}
            </StatusPill>
            {nextIso ? (
              <span className="text-sm tabular-nums text-muted-foreground">
                下次{" "}
                <time dateTime={nextIso}>{formatRelativeFuture(nextIso)}</time>
              </span>
            ) : null}
            {!rule.enabled && rule.disabled_reason ? (
              <span className="text-sm text-muted-foreground">
                {disabledReasonLabel(rule.disabled_reason)}
              </span>
            ) : null}
          </div>
        </div>

        <HumanGateCallout mode={rule.coordination_mode} />

        {actionError ? (
          <p className="text-sm text-destructive" role="alert">
            {actionError}
          </p>
        ) : null}

        <div className="flex flex-wrap gap-2">
          {rule.enabled ? (
            <Button disabled={busy} onClick={onDisable} size="sm" variant="outline">
              <Power data-icon="inline-start" />
              停用
            </Button>
          ) : (
            <Button disabled={busy} onClick={onEnable} size="sm" variant="outline">
              <Power data-icon="inline-start" />
              启用
            </Button>
          )}
          <Button disabled={busy || !rule.enabled} onClick={onTrigger} size="sm">
            <Play data-icon="inline-start" />
            立即试跑
          </Button>
          <Button disabled={busy} onClick={onEdit} size="sm" variant="outline">
            <Pencil data-icon="inline-start" />
            编辑
          </Button>
          <Button disabled={busy} onClick={onDelete} size="sm" variant="danger">
            <Trash2 data-icon="inline-start" />
            删除
          </Button>
        </div>
      </SoftCard>

      <SoftCard className="min-h-0 flex-1 space-y-3 p-4">
        <h3 className="text-sm font-medium text-foreground">最近触发</h3>
        {firesLoading ? (
          <LoadingState label="加载触发记录…" />
        ) : fires.length === 0 ? (
          <EmptyState
            description="保存规则后可点「立即试跑」，或等待日程到点。"
            title="尚未触发"
          />
        ) : (
          <DataTable>
            <thead>
              <tr>
                <Th>时间</Th>
                <Th>结果</Th>
                <Th>对象</Th>
              </tr>
            </thead>
            <tbody>
              {fires.map((fire) => (
                <Tr
                  key={fire.id}
                  tone={automationFireTone(fire.status) === "danger" ? "danger" : undefined}
                >
                  <Td className="tabular-nums text-sm text-muted-foreground">
                    <time dateTime={fire.scheduled_fire_at}>
                      {new Date(fire.scheduled_fire_at).toLocaleString("zh-CN")}
                    </time>
                  </Td>
                  <Td>
                    <StatusPill tone={automationFireTone(fire.status)}>
                      {statusLabel(fire.status)}
                    </StatusPill>
                    {fire.error_message ? (
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {fire.error_message}
                      </p>
                    ) : null}
                  </Td>
                  <Td className="text-sm">
                    {fire.demand_id ? (
                      <Link
                        className="text-primary underline-offset-2 hover:underline"
                        params={{ demandId: fire.demand_id }}
                        to="/workflows/$demandId"
                      >
                        需求
                      </Link>
                    ) : null}
                    {fire.run_id ? (
                      <span className="text-muted-foreground">运行 {fire.run_id.slice(0, 8)}</span>
                    ) : null}
                    {!fire.demand_id && !fire.run_id ? (
                      <span className="text-muted-foreground">—</span>
                    ) : null}
                  </Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        )}
      </SoftCard>
    </div>
  );
}
