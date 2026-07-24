import { useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient
} from "@tanstack/react-query";
import { AlertTriangle, ShieldCheck } from "lucide-react";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  decidePermissionApproval,
  listPermissionApprovals,
  type PermissionApproval,
  type PermissionApprovalAction,
  type PermissionApprovalDecisionInput,
  type PermissionApprovalListFilters,
  type PermissionApprovalStatus,
  type PermissionApprovalView
} from "@/lib/api/permission-approvals";
import {
  ObjectRef,
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  WorkSurface
} from "@/components/superteam";
import {
  permissionResourceTypeLabel,
  riskLevelLabel,
  statusLabel
} from "@/lib/status-labels";
import { PermissionApprovalDialog } from "./permission-approval-dialog";

type PermissionApprovalsQueueProps = {
  apiOptions: ApiClientOptions;
};

type SelectedAction = {
  action: PermissionApprovalAction;
  approval: PermissionApproval;
};

const VIEW_OPTIONS: { value: PermissionApprovalView; label: string }[] = [
  { value: "mine", label: "待我审批" },
  { value: "team", label: "作用域全部" },
];

const STATUS_OPTIONS: { value: PermissionApprovalStatus; label: string }[] = [
  { value: "pending", label: "待处理" },
  { value: "approved", label: "已同意" },
  { value: "rejected", label: "已驳回" },
  { value: "needs_more_evidence", label: "需补证" },
  { value: "cancelled", label: "已取消" },
];

const RISK_OPTIONS: { value: string; label: string }[] = [
  { value: "blocked", label: "阻断" },
  { value: "high", label: "高风险" },
  { value: "medium", label: "中风险" },
  { value: "low", label: "低风险" },
];

function riskTone(level: string | undefined): "danger" | "warn" | "ok" | "mute" {
  switch (level?.trim().toLowerCase()) {
    case "blocked":
    case "high":
      return "danger";
    case "medium":
      return "warn";
    case "low":
      return "ok";
    default:
      return "mute";
  }
}

export function PermissionApprovalsQueue({ apiOptions }: PermissionApprovalsQueueProps) {
  const queryClient = useQueryClient();
  const [view, setView] = useState<PermissionApprovalView>("mine");
  const [status, setStatus] = useState<PermissionApprovalStatus>("pending");
  const [riskLevel, setRiskLevel] = useState<string | null>(null);

  const [selectedAction, setSelectedAction] = useState<SelectedAction | null>(null);
  // 与审批中心同一套按事项并行模式:在飞按 id 记录,不同事项互不阻塞;
  // 弹窗切走或关闭后失败的后台提交升级到页面横幅,不静默丢失。
  const [pendingItemIds, setPendingItemIds] = useState<ReadonlySet<string>>(() => new Set());
  const [backgroundActionError, setBackgroundActionError] = useState<Error | null>(null);
  const selectedActionRef = useRef<SelectedAction | null>(null);
  selectedActionRef.current = selectedAction;

  const filters = useMemo<PermissionApprovalListFilters>(() => {
    const next: PermissionApprovalListFilters = {
      view,
      status,
      limit: 50,
      offset: 0
};
    if (riskLevel) {
      next.risk_level = riskLevel;
    }
    return next;
  }, [view, status, riskLevel]);

  const approvalsQuery = useQuery({
    queryKey: ["permission-approvals", apiOptions.baseUrl, filters],
    queryFn: () => listPermissionApprovals(apiOptions, filters),
    placeholderData: keepPreviousData,
    // 审批决策会触发外部 apply,无推送;靠轮询反映状态变化。
    refetchInterval: 5000
});

  const decisionMutation = useMutation({
    mutationFn: ({
      id,
      input
}: {
      id: string;
      input: PermissionApprovalDecisionInput;
    }) => decidePermissionApproval(apiOptions, id, input),
    onMutate: ({ id }) => {
      setPendingItemIds((current) => {
        const next = new Set(current);
        next.add(id);
        return next;
      });
    },
    onSuccess: (_data, { id }) => {
      setSelectedAction((current) => (current && current.approval.id === id ? null : current));
      void queryClient.invalidateQueries({ queryKey: ["permission-approvals"] });
    },
    onError: (error, { id }) => {
      const current = selectedActionRef.current;
      if (!current || current.approval.id !== id) {
        setBackgroundActionError(error instanceof Error ? error : new Error(String(error)));
      }
    },
    onSettled: (_data, _error, { id }) => {
      setPendingItemIds((current) => {
        const next = new Set(current);
        next.delete(id);
        return next;
      });
    }
});

  const data = approvalsQuery.data;
  const items = data?.items ?? [];

  return (
    <div className="space-y-5">
      {backgroundActionError ? (
        <div className="rounded-inner bg-danger-soft p-4 text-sm text-danger" role="alert">
          <p className="font-bold">操作未完成</p>
          <p className="mt-1 text-ink-2">{backgroundActionError.message}</p>
        </div>
      ) : null}

      {data ? (
        <section className="grid gap-4 sm:grid-cols-3">
          <MetricCard
            icon={<ShieldCheck />}
            iconTone="info"
            label="开放审批"
            value={data.summary.open_count}
            meta="待处理决策"
          />
          <MetricCard
            icon={<AlertTriangle />}
            iconTone="danger"
            label="高风险"
            value={data.summary.high_risk_count}
            meta="需优先确认"
          />
          <MetricCard
            icon={<AlertTriangle />}
            iconTone="warn"
            label="阻断"
            value={data.summary.blocked_count}
            meta="等待人类判断"
          />
        </section>
      ) : null}

      <WorkSurface className="min-w-0">
        <div className="flex flex-col gap-3 border-b border-line p-4">
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-ink">权限审批队列</h2>
            <p className="mt-1 text-xs text-ink-2">
              直读审批域（category=permission），与收件箱分离；决策会触发主体授权动作。
            </p>
          </div>
          <div className="flex flex-wrap items-center gap-4 text-xs">
            <FilterGroup label="视图">
              {VIEW_OPTIONS.map((option) => (
                <FilterButton
                  key={option.value}
                  active={view === option.value}
                  onClick={() => setView(option.value)}
                >
                  {option.label}
                </FilterButton>
              ))}
            </FilterGroup>
            <FilterGroup label="状态">
              {STATUS_OPTIONS.map((option) => (
                <FilterButton
                  key={option.value}
                  active={status === option.value}
                  onClick={() => setStatus(option.value)}
                >
                  {option.label}
                </FilterButton>
              ))}
            </FilterGroup>
            <FilterGroup label="风险">
              <FilterButton active={riskLevel === null} onClick={() => setRiskLevel(null)}>
                全部
              </FilterButton>
              {RISK_OPTIONS.map((option) => (
                <FilterButton
                  key={option.value}
                  active={riskLevel === option.value}
                  onClick={() => setRiskLevel(option.value)}
                >
                  {option.label}
                </FilterButton>
              ))}
            </FilterGroup>
          </div>
        </div>

        {approvalsQuery.isLoading && !data ? <LoadingState label="加载权限审批" /> : null}
        {approvalsQuery.isError && !data ? (
          <ErrorState
            title="权限审批加载失败"
            description={
              approvalsQuery.error instanceof Error
                ? approvalsQuery.error.message
                : "请稍后刷新或检查 Control Plane 连接。"
            }
          />
        ) : null}
        {data && items.length === 0 ? <EmptyState title="当前没有权限审批事项" /> : null}
        {items.length > 0 ? (
          <div className="divide-y divide-line">
            {items.map((approval) => (
              <PermissionApprovalRow
                approval={approval}
                key={approval.id}
                onAction={(action) => {
                  setBackgroundActionError(null);
                  setSelectedAction({ action, approval });
                }}
              />
            ))}
          </div>
        ) : null}
      </WorkSurface>

      <PermissionApprovalDialog
        action={selectedAction?.action ?? null}
        approval={selectedAction?.approval ?? null}
        onOpenChange={(open) => {
          if (!open) {
            setSelectedAction(null);
          }
        }}
        onSubmit={(input) => {
          const current = selectedActionRef.current;
          if (!current || pendingItemIds.has(current.approval.id)) {
            return Promise.resolve();
          }
          return decisionMutation.mutateAsync({ id: current.approval.id, input });
        }}
        open={Boolean(selectedAction)}
        pending={selectedAction ? pendingItemIds.has(selectedAction.approval.id) : false}
      />
    </div>
  );
}

function PermissionApprovalRow({
  approval,
  onAction
}: {
  approval: PermissionApproval;
  onAction: (action: PermissionApprovalAction) => void;
}) {
  const actions = Array.isArray(approval.actions) ? approval.actions : [];

  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h3 className="text-sm font-semibold text-ink">{approval.title}</h3>
          {approval.risk_level ? (
            <StatusPill tone={riskTone(approval.risk_level)}>
              {riskLevelLabel(approval.risk_level)}
            </StatusPill>
          ) : null}
          <StatusPill tone={approval.status === "pending" ? "warn" : "mute"}>
            {statusLabel(approval.status)}
          </StatusPill>
          <StatusPill tone="mute">
            {permissionResourceTypeLabel(approval.resource_type)}
          </StatusPill>
        </div>
        <p className="mt-1 line-clamp-2 text-sm text-ink-2">
          {approval.summary ?? "等待处理"}
        </p>
        <div className="mt-1 text-xs text-ink-3">
          申请人：
          <ObjectRef name={approval.requester_name} id={approval.requester_id} />
        </div>
      </div>
      <div className="flex flex-wrap gap-2">
        {actions.slice(0, 3).map((action) => (
          <Button
            key={action.key}
            size="sm"
            type="button"
            variant={
              action.tone === "destructive"
                ? "danger"
                : action.tone === "positive"
                  ? "primary"
                  : "outline"
            }
            onClick={() => onAction(action)}
          >
            {action.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

function FilterGroup({ children, label }: { children: React.ReactNode; label: string }) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="font-semibold text-ink-2">{label}</span>
      {children}
    </div>
  );
}

function FilterButton({
  active,
  children,
  onClick
}: {
  active: boolean;
  children: React.ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={
        active
          ? "inline-flex h-7 items-center rounded-inner bg-brand-soft px-2.5 font-semibold text-brand-deep"
          : "inline-flex h-7 items-center rounded-inner border border-line bg-card px-2.5 font-medium text-ink-2 hover:bg-card-soft hover:text-ink"
      }
    >
      {children}
    </button>
  );
}
