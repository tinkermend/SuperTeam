import { useEffect, useState } from "react";
import type { ExecuteInboxActionInput, InboxAction, InboxItem } from "@/lib/api/inbox";
import { Button } from "@/components/ui/button";
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
  const [comment, setComment] = useState("");
  const requiresComment = Boolean(action?.requires_comment);
  const canSubmit = Boolean(action && item && (!requiresComment || comment.trim()));

  useEffect(() => {
    if (open) {
      setComment("");
    }
  }, [open, item?.id, action?.key]);

  const submit = async () => {
    if (!action || !item || !canSubmit || pending) {
      return;
    }

    await onSubmit({
      action: action.key,
      comment,
      payload: {},
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="border-[color:var(--superteam-glass-border)] bg-[color:var(--superteam-glass-strong-bg)] sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{action ? action.label : "处理事项"}</DialogTitle>
          <DialogDescription>{item?.title ?? "确认本次收件箱处理动作。"}</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <label className="text-sm font-medium" htmlFor="inbox-action-comment">
            处理意见{requiresComment ? "（必填）" : "（可选）"}
          </label>
          <Textarea
            aria-invalid={requiresComment && !comment.trim()}
            disabled={pending}
            id="inbox-action-comment"
            onChange={(event) => setComment(event.target.value)}
            placeholder="补充审批理由、补证要求或验收结论"
            value={comment}
          />
          {requiresComment && !comment.trim() ? (
            <p className="text-xs text-destructive">该动作需要填写处理意见。</p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            取消
          </Button>
          <Button type="button" onClick={submit} disabled={!canSubmit || pending}>
            {pending ? "提交中" : "提交"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
