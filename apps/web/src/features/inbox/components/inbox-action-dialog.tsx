import { useEffect, useRef, useState } from "react";
import type { ExecuteInboxActionInput, InboxAction, InboxItem } from "@/lib/api/inbox";
import { V3Button, V3ErrorState } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { formatInboxActionLabel } from "./action-format";
import { riskLabel } from "./inbox-item-list";

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
  const inFlightRef = useRef(false);
  const [comment, setComment] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const requiresComment = Boolean(action?.requires_comment);
  const isSubmitting = pending || inFlightRef.current;
  const canSubmit = Boolean(action && item && (!requiresComment || comment.trim()));

  useEffect(() => {
    if (open) {
      setComment("");
      setSubmitError(null);
      inFlightRef.current = false;
    }
  }, [open, item?.id, action?.key]);

  const submit = async () => {
    if (!action || !item || !canSubmit || isSubmitting) {
      return;
    }

    inFlightRef.current = true;
    setSubmitError(null);

    try {
      await onSubmit({
        action: action.key,
        comment,
        payload: {},
      });
    } catch (error) {
      inFlightRef.current = false;
      setSubmitError(error instanceof Error ? error.message : "操作提交失败");
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="border-v3-line bg-v3-card text-v3-ink shadow-v3-pop sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{action ? formatInboxActionLabel(action) : "处理事项"}</DialogTitle>
          <DialogDescription>{item?.title ?? "确认本次收件箱处理动作。"}</DialogDescription>
        </DialogHeader>
        {item ? <InboxActionContextSummary item={item} /> : null}
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
            placeholder="补充审批理由、补证要求或验收结论"
            value={comment}
          />
          {requiresComment && !comment.trim() ? (
            <p className="text-xs font-semibold text-v3-danger">该动作需要填写处理意见。</p>
          ) : null}
        </div>
        <DialogFooter>
          <V3Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            取消
          </V3Button>
          <V3Button type="button" onClick={submit} disabled={!canSubmit || isSubmitting}>
            {isSubmitting ? "提交中" : "提交"}
          </V3Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function InboxActionContextSummary({ item }: { item: InboxItem }) {
  const contextRows = readableContextRows(item.context);

  return (
    <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
      <div className="grid gap-2 text-xs sm:grid-cols-2">
        {item.risk_level ? (
          <ContextPair label="风险等级" value={riskLabel[item.risk_level] ?? item.risk_level} />
        ) : null}
        {item.source_project_id ? <ContextPair label="来源项目" value={item.source_project_id} /> : null}
        {item.source_task_id ? <ContextPair label="来源任务" value={item.source_task_id} /> : null}
        {item.source_approval_request_id ? (
          <ContextPair label="审批请求" value={item.source_approval_request_id} />
        ) : null}
      </div>
      {item.summary ? (
        <div className="mt-3 border-t border-v3-line pt-3">
          <div className="text-xs font-semibold text-v3-ink-2">摘要</div>
          <p className="mt-1 text-sm leading-5 text-v3-ink">{item.summary}</p>
        </div>
      ) : null}
      {contextRows.length > 0 ? (
        <div className="mt-3 border-t border-v3-line pt-3">
          <div className="text-xs font-semibold text-v3-ink-2">上下文摘要</div>
          <div className="mt-2 grid gap-1.5">
            {contextRows.map((row) => (
              <ContextPair key={row.label} label={row.label} value={row.value} />
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function ContextPair({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-start justify-between gap-3">
      <span className="shrink-0 font-semibold text-v3-ink-2">{label}</span>
      <span className="min-w-0 break-words text-right font-medium text-v3-ink">{value}</span>
    </div>
  );
}

function readableContextRows(context: Record<string, unknown>) {
  const preferredLabels: Record<string, string> = {
    approval_title: "审批标题",
    current_node: "当前节点",
    decision_type: "决策类型",
    node_title: "节点",
    project_name: "项目名称",
    project_title: "项目",
    source_title: "来源事项",
    stage: "阶段",
    task_title: "任务",
    workflow_node: "流程节点",
  };

  return Object.entries(context)
    .filter(([, value]) => typeof value === "string" && value.trim().length > 0)
    .slice(0, 6)
    .map(([key, value]) => ({
      label: preferredLabels[key] ?? key,
      value: String(value).trim(),
    }));
}
