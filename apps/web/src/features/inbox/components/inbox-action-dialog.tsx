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
          <DialogTitle>{action ? action.label : "处理事项"}</DialogTitle>
          <DialogDescription>{item?.title ?? "确认本次收件箱处理动作。"}</DialogDescription>
        </DialogHeader>
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
