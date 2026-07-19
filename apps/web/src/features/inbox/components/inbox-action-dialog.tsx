import { useEffect, useRef, useState } from "react";
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
import { formatInboxActionLabel } from "./action-format";
import { riskLabel, riskTone } from "./inbox-item-list";

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
        setSubmitError(error instanceof Error ? error.message : "操作提交失败");
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

function InboxActionContextSummary({ item }: { item: InboxItem }) {
  const contextRows = readableContextRows(item.context);

  return (
    <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
      <div className="grid gap-3 text-xs sm:grid-cols-2">
        {item.risk_level ? (
          <ContextField label="风险等级">
            <StatusPill tone={riskTone[item.risk_level] ?? "mute"} className="px-2 py-0.5 text-[11px]">
              {riskLabel[item.risk_level] ?? item.risk_level}
            </StatusPill>
          </ContextField>
        ) : null}
        {item.source_project_id ? (
          <ContextField label="来源项目">
            <ObjectRef name={item.source_project_name} id={item.source_project_id} />
          </ContextField>
        ) : null}
        {item.source_task_id ? (
          <ContextField label="来源任务">
            <ObjectRef name={item.source_task_name} id={item.source_task_id} />
          </ContextField>
        ) : null}
        {item.source_approval_request_id ? (
          <ContextField label="审批请求">
            <ObjectIdChip id={item.source_approval_request_id} />
          </ContextField>
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
          <div className="mt-2 grid gap-3 sm:grid-cols-2">
            {contextRows.map((row) => (
              <ContextField key={row.label} label={row.label}>
                <span className="min-w-0 break-words">{row.value}</span>
              </ContextField>
            ))}
          </div>
        </div>
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
