import { useEffect, useRef, useState } from "react";
import { Link } from "@tanstack/react-router";
import { ApiRequestError } from "@/lib/api/client";
import type { ExecuteInboxActionInput, InboxAction, InboxItem } from "@/lib/api/inbox";
import { ObjectIdChip, ObjectRef, StatusPill, V3Button, V3ErrorState } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { decisionTypeLabel } from "@/lib/status-labels";
import { formatInboxActionLabel } from "./action-format";
import {
  primaryDemandLabel,
  primaryTaskLabel,
  readContextText,
  readDemandRefs,
  resolveInboxHref,
  riskLabel,
  riskTone,
} from "./inbox-item-list";

type InboxActionDialogProps = {
  action: InboxAction | null;
  item: InboxItem | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: ExecuteInboxActionInput) => Promise<unknown>;
  open: boolean;
  pending?: boolean;
};

export function InboxActionDialog({
  action,
  item,
  onOpenChange,
  onSubmit,
  open,
  pending = false,
}: InboxActionDialogProps) {
  // in-flight 记录按(事项,动作)键而非布尔:弹窗组件跨事项复用,布尔 ref 会把
  // 上一事项的在飞状态泄漏给下一事项(按钮永久禁用);键不匹配即自然失效。
  const inFlightKeyRef = useRef<string | null>(null);
  // 当前键镜像:提交发出后用户可能切换事项或关闭弹窗,迟到的失败不得回写到别的事项上。
  const currentKeyRef = useRef<string | null>(null);
  const [comment, setComment] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const currentKey = open && item && action ? `${item.id}:${action.key}` : null;
  currentKeyRef.current = currentKey;
  const requiresComment = Boolean(action?.requires_comment);
  const isSubmitting = pending || (currentKey !== null && inFlightKeyRef.current === currentKey);
  const canSubmit = Boolean(action && item && (!requiresComment || comment.trim()));
  const decisionType = readDecisionType(item);
  const commentPlaceholder = commentPlaceholderFor(action, decisionType);

  useEffect(() => {
    if (open) {
      setComment("");
      setSubmitError(null);
    }
  }, [open, item?.id, action?.key]);

  const submit = async () => {
    if (!action || !item || !canSubmit || pending) {
      return;
    }

    const submittedKey = currentKeyRef.current;
    // 同键同步防重:双击在父层状态更新前到达时,第二次直接落空。
    if (!submittedKey || inFlightKeyRef.current === submittedKey) {
      return;
    }
    inFlightKeyRef.current = submittedKey;
    setSubmitError(null);

    try {
      await onSubmit({
        action: action.key,
        comment,
        payload: {},
      });
    } catch (error) {
      // 用户已切换到其他事项或关闭弹窗:失败由页面横幅承接,不写进当前弹窗。
      if (currentKeyRef.current === submittedKey) {
        // 优先展示服务端 detail(如"该事项由 X 处理"),避免裸 "request failed with status 403"。
        setSubmitError(
          error instanceof ApiRequestError && error.detail
            ? error.detail
            : error instanceof Error
              ? error.message
              : "操作提交失败",
        );
      }
    } finally {
      if (inFlightKeyRef.current === submittedKey) {
        inFlightKeyRef.current = null;
      }
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="border-v3-line bg-v3-card text-v3-ink shadow-v3-pop sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{action ? formatInboxActionLabel(action) : "处理事项"}</DialogTitle>
          <DialogDescription>{item?.title ?? "确认本次收件箱处理动作。"}</DialogDescription>
        </DialogHeader>
        {item && action ? <InboxActionContextSummary action={action} item={item} /> : null}
        {submitError ? (
          <V3ErrorState title="操作未完成" description={submitError} className="py-4" />
        ) : null}
        <div className="space-y-2">
          <label className="text-sm font-semibold text-v3-ink" htmlFor="inbox-action-comment">
            处理意见{requiresComment ? "（必填）" : "（可选）"}
          </label>
          <Textarea
            aria-invalid={requiresComment && !comment.trim()}
            className="min-h-28 rounded-v3-inner border-v3-line-strong bg-v3-card text-v3-ink shadow-none placeholder:text-v3-ink-3 focus-visible:border-v3-brand focus-visible:ring-2 focus-visible:ring-v3-brand/25 aria-invalid:border-v3-danger"
            disabled={isSubmitting}
            id="inbox-action-comment"
            onChange={(event) => setComment(event.target.value)}
            placeholder={commentPlaceholder}
            value={comment}
          />
          {requiresComment && !comment.trim() ? (
            <p className="text-xs font-semibold text-v3-danger">该动作需要填写处理意见。</p>
          ) : null}
        </div>
        {isSubmitting ? (
          <p className="text-xs leading-5 text-v3-ink-3">
            正在提交，关闭弹窗后提交会在后台继续，可先处理其他事项。
          </p>
        ) : null}
        <DialogFooter>
          <V3Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {isSubmitting ? "关闭" : "取消"}
          </V3Button>
          <V3Button type="button" onClick={submit} disabled={!canSubmit || isSubmitting}>
            {isSubmitting ? "提交中" : "提交"}
          </V3Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function InboxActionContextSummary({ action, item }: { action: InboxAction; item: InboxItem }) {
  const decisionType = readDecisionType(item);
  const demandLabel = primaryDemandLabel(item);
  const taskLabel = primaryTaskLabel(item);
  const demandRefs = readDemandRefs(item);
  const primaryDemandId = demandRefs[0]?.id;
  const projectName =
    item.source_project_name ??
    readContextText(item.context, ["project_name", "project", "project_title"]);
  const framing = decisionFraming(decisionType);
  const consequence = actionConsequence(action, decisionType);
  const riskReason = riskReasonFor(decisionType, item.risk_level);
  const showSummary = shouldShowSummary(item, demandLabel, projectName, taskLabel);
  // F3(§5.4.3): 走服务端 primary_surface 的唯一落点,不再本地拼 href。仅在有需求/项目
  // 归属时展示"查看证据"入口(审批类纯事项无归属则不展示)。
  const reviewHref = primaryDemandId || item.source_project_id ? resolveInboxHref(item) : undefined;
  const technicalRows = technicalContextRows(item);

  return (
    <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
      {framing ? (
        <div className="space-y-1.5">
          <p className="text-[13px] font-bold leading-5 text-v3-ink">{framing.headline}</p>
          <p className="text-xs leading-5 text-v3-ink-2">{framing.scope}</p>
        </div>
      ) : null}

      {consequence ? (
        <p
          className={`text-xs leading-5 text-v3-ink-2 ${framing ? "mt-3 border-t border-v3-line pt-3" : ""}`}
        >
          <span className="font-semibold text-v3-ink">本次「{formatInboxActionLabel(action)}」后：</span>
          {consequence}
        </p>
      ) : null}

      <div className={`${framing || consequence ? "mt-3 border-t border-v3-line pt-3" : ""} space-y-2.5`}>
        {demandLabel ? (
          <ContextField label="触发需求">
            {primaryDemandId ? <ObjectRef name={demandLabel} id={primaryDemandId} /> : <span>{demandLabel}</span>}
          </ContextField>
        ) : null}
        {taskLabel ? (
          <ContextField label="相关任务">
            {item.source_task_id ? (
              <ObjectRef name={taskLabel} id={item.source_task_id} />
            ) : (
              <span>{taskLabel}</span>
            )}
          </ContextField>
        ) : item.source_task_id ? (
          <ContextField label="相关任务">
            <ObjectRef name={item.source_task_name} id={item.source_task_id} />
          </ContextField>
        ) : null}
        {item.source_project_id || projectName ? (
          <ContextField label="所属项目">
            {item.source_project_id ? (
              <ObjectRef name={projectName} id={item.source_project_id} />
            ) : (
              <span>{projectName}</span>
            )}
          </ContextField>
        ) : null}
        {item.risk_level ? (
          <ContextField label="风险提示">
            <div className="flex flex-wrap items-center gap-2">
              <StatusPill tone={riskTone[item.risk_level] ?? "mute"} className="px-2 py-0.5 text-[11px]">
                {riskLabel[item.risk_level] ?? item.risk_level}
              </StatusPill>
              {riskReason ? <span className="text-xs font-medium text-v3-ink-2">{riskReason}</span> : null}
            </div>
          </ContextField>
        ) : null}
      </div>

      {showSummary && item.summary ? (
        <div className="mt-3 border-t border-v3-line pt-3">
          <div className="text-xs font-semibold text-v3-ink-2">补充说明</div>
          <p className="mt-1 text-sm leading-5 text-v3-ink">{item.summary}</p>
        </div>
      ) : null}

      {reviewHref ? (
        <div className="mt-3 border-t border-v3-line pt-3">
          <Link
            className="text-[13px] font-semibold text-v3-brand hover:underline"
            to={reviewHref}
          >
            {primaryDemandId ? "查看需求流程与产出" : "查看项目详情"}
          </Link>
          <p className="mt-1 text-xs leading-5 text-v3-ink-3">
            建议先核对证据与结论，再提交本次处理。
          </p>
        </div>
      ) : null}

      {technicalRows.length > 0 || item.source_approval_request_id ? (
        <details className="mt-3 border-t border-v3-line pt-3">
          <summary className="cursor-pointer text-xs font-semibold text-v3-ink-3 hover:text-v3-ink-2">
            技术详情
          </summary>
          <div className="mt-2 grid gap-3 sm:grid-cols-2">
            {decisionType ? (
              <ContextField label="决策类型">
                <span>{decisionTypeLabel(decisionType)}</span>
              </ContextField>
            ) : null}
            {item.source_approval_request_id ? (
              <ContextField label="审批请求">
                <ObjectIdChip id={item.source_approval_request_id} />
              </ContextField>
            ) : null}
            {technicalRows.map((row) => (
              <ContextField key={row.label} label={row.label}>
                <span className="min-w-0 break-words font-mono text-[12px]">{row.value}</span>
              </ContextField>
            ))}
          </div>
        </details>
      ) : null}
    </div>
  );
}

/** 键值字段：标签在上、值在下左对齐，长值折行仍齐整（替代旧 justify-between 右对齐形态）。 */
function ContextField({ children, label }: { children: React.ReactNode; label: string }) {
  return (
    <div className="min-w-0 space-y-1">
      <div className="font-semibold text-v3-ink-2">{label}</div>
      <div className="min-w-0 text-[13px] leading-5 font-medium text-v3-ink">{children}</div>
    </div>
  );
}

function readDecisionType(item: InboxItem | null): string | undefined {
  if (!item) return undefined;
  return readContextText(item.context, ["decision_type"]);
}

function decisionFraming(decisionType: string | undefined): { headline: string; scope: string } | null {
  switch (decisionType) {
    case "project_acceptance":
      return {
        headline: "你在确认：项目整体可关闭",
        scope: "这是项目级验收闸。触发它的是下方需求（及任务）已终态；同意后项目归档，不是单独再验收一次需求。",
      };
    case "demand_acceptance":
      return {
        headline: "你在确认：该需求是否验收通过",
        scope: "签署需求验收后，若项目内全部需求已终态，可能会继续打开项目验收。",
      };
    case "plan_review":
      return {
        headline: "你在确认：项目计划版本是否可执行",
        scope: "同意后计划生效并进入派发；驳回或要求修改将回到规划调整。",
      };
    case "planning_gap":
      return {
        headline: "你在处理：规划缺口",
        scope: "选择补员、豁免或关闭，将决定需求是否重新规划。",
      };
    default:
      return null;
  }
}

function actionConsequence(action: InboxAction, decisionType: string | undefined): string | null {
  const key = action.key.trim().toLowerCase();
  if (decisionType === "project_acceptance") {
    if (key === "approved" || key === "approve") {
      return "项目将归档关闭，协调线程结束；任一负责人处理即生效。";
    }
    if (key === "rejected" || key === "reject") {
      return "项目回到运行中，可继续返工；任一负责人处理即生效。";
    }
    if (key === "needs_more_evidence" || key === "request_evidence") {
      return "项目回到运行中，等待补充证据后再验收；任一负责人处理即生效。";
    }
  }
  if (decisionType === "demand_acceptance") {
    if (key === "approved" || key === "approve") {
      return "需求标记为已完成；若其为项目最后一条终态需求，可能接着打开项目验收。";
    }
    if (key === "rejected" || key === "reject") {
      return "需求标记为失败，需按协调策略返工或重新规划。";
    }
    if (key === "needs_more_evidence" || key === "request_evidence") {
      return "需求保持待验收，需按意见补充证据后再签。";
    }
  }
  return null;
}

function riskReasonFor(decisionType: string | undefined, riskLevel: string | undefined): string | null {
  if (!riskLevel || (riskLevel !== "high" && riskLevel !== "blocked")) {
    return null;
  }
  if (decisionType === "project_acceptance") {
    return "项目级终态决策，同意后项目归档，影响面大。";
  }
  if (decisionType === "demand_acceptance") {
    return "需求验收结论会影响后续项目是否进入关闭闸。";
  }
  return "高影响决策，请确认证据后再提交。";
}

function shouldShowSummary(
  item: InboxItem,
  demandLabel: string | undefined,
  projectName: string | undefined,
  taskLabel: string | undefined,
): boolean {
  const summary = item.summary?.trim();
  if (!summary) return false;
  // 项目验收摘要通常只是标题/对象名的再拼接，主区已展示则不再重复。
  if (readDecisionType(item) === "project_acceptance") {
    return false;
  }
  const needles = [item.title, demandLabel, projectName, taskLabel].filter(Boolean) as string[];
  const compact = summary.replace(/[「」·（）()\s]/g, "");
  const overlap = needles.some((needle) => compact.includes(needle.replace(/[「」·（）()\s]/g, "")));
  // 摘要几乎只是在复述主身份时隐藏。
  return !overlap || summary.length > 80;
}

function commentPlaceholderFor(action: InboxAction | null, decisionType: string | undefined): string {
  const key = action?.key.trim().toLowerCase() ?? "";
  if (decisionType === "project_acceptance") {
    if (key === "approved" || key === "approve") return "可选：补充验收结论";
    if (key === "rejected" || key === "reject") return "说明驳回原因与返工方向";
    if (key === "needs_more_evidence" || key === "request_evidence") {
      return "说明需要补充的证据";
    }
  }
  if (key === "rejected" || key === "reject" || key === "needs_more_evidence" || key === "request_evidence") {
    return "补充驳回理由、补证要求或处理说明";
  }
  return "可选：补充处理意见或验收结论";
}

function technicalContextRows(item: InboxItem): Array<{ label: string; value: string }> {
  const preferredLabels: Record<string, string> = {
    primary_demand_id: "主需求 ID",
    project_id: "项目 ID",
    plan_revision_id: "计划版本 ID",
    demand_id: "需求 ID",
  };
  const hidden = new Set([
    "demands",
    "pending_criteria",
    "pending_criteria_detail",
    "decision_type",
    "project_name",
    "project_title",
    "demand_title",
    "task_title",
    "source_title",
    "approval_title",
    "current_node",
    "node_title",
    "stage",
    "workflow_node",
  ]);

  return Object.entries(item.context ?? {})
    .filter(([key, value]) => {
      if (hidden.has(key)) return false;
      return typeof value === "string" && value.trim().length > 0;
    })
    .slice(0, 6)
    .map(([key, value]) => ({
      label: preferredLabels[key] ?? key,
      value: String(value).trim(),
    }));
}
