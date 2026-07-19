import { useEffect, useRef, useState } from "react";
import { ObjectRef, StatusPill, V3Button, V3ErrorState } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import type {
  PermissionApproval,
  PermissionApprovalAction,
  PermissionApprovalDecisionInput,
} from "@/lib/api/permission-approvals";
import {
  permissionResourceTypeLabel,
  riskLevelLabel,
} from "@/lib/status-labels";

type PermissionApprovalDialogProps = {
  action: PermissionApprovalAction | null;
  approval: PermissionApproval | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: PermissionApprovalDecisionInput) => Promise<unknown>;
  open: boolean;
  pending?: boolean;
};

function riskTone(level: string | undefined): "danger" | "warn" | "ok" | "mute" {
  switch (level?.trim().toLowerCase()) {
    case "blocked":
      return "danger";
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

/** 补证是强约束动作,提交意见必填;同意/驳回意见可选。 */
function requiresNote(action: PermissionApprovalAction | null): boolean {
  return action?.key === "needs_more_evidence" || action?.key === "rejected";
}

export function PermissionApprovalDialog({
  action,
  approval,
  onOpenChange,
  onSubmit,
  open,
  pending = false,
}: PermissionApprovalDialogProps) {
  // 同 inbox 弹窗:按(事项,动作)键记录在飞状态,跨事项复用不泄漏。
  const inFlightKeyRef = useRef<string | null>(null);
  const currentKeyRef = useRef<string | null>(null);
  const [note, setNote] = useState("");
  const [submitError, setSubmitError] = useState<string | null>(null);
  const currentKey = open && approval && action ? `${approval.id}:${action.key}` : null;
  currentKeyRef.current = currentKey;
  const noteRequired = requiresNote(action);
  const isSubmitting = pending || (currentKey !== null && inFlightKeyRef.current === currentKey);
  const canSubmit = Boolean(action && approval && (!noteRequired || note.trim()));

  useEffect(() => {
    if (open) {
      setNote("");
      setSubmitError(null);
    }
  }, [open, approval?.id, action?.key]);

  const submit = async () => {
    if (!action || !approval || !canSubmit || pending) {
      return;
    }
    const submittedKey = currentKeyRef.current;
    if (!submittedKey || inFlightKeyRef.current === submittedKey) {
      return;
    }
    inFlightKeyRef.current = submittedKey;
    setSubmitError(null);

    try {
      await onSubmit({
        decision: action.key as PermissionApprovalDecisionInput["decision"],
        note,
      });
    } catch (error) {
      if (currentKeyRef.current === submittedKey) {
        setSubmitError(error instanceof Error ? error.message : "决策提交失败");
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
          <DialogTitle>{action?.label ?? "处理权限审批"}</DialogTitle>
          <DialogDescription>{approval?.title ?? "确认本次权限审批决策。"}</DialogDescription>
        </DialogHeader>
        {approval ? <PermissionApprovalContext approval={approval} /> : null}
        {submitError ? (
          <V3ErrorState title="决策未完成" description={submitError} className="py-4" />
        ) : null}
        <div className="space-y-2">
          <label className="text-sm font-semibold text-v3-ink" htmlFor="permission-approval-note">
            处理意见{noteRequired ? "（必填）" : "（可选）"}
          </label>
          <Textarea
            aria-invalid={noteRequired && !note.trim()}
            className="min-h-28 rounded-v3-inner border-v3-line-strong bg-v3-card text-v3-ink shadow-none placeholder:text-v3-ink-3 focus-visible:border-v3-brand focus-visible:ring-2 focus-visible:ring-v3-brand/25 aria-invalid:border-v3-danger"
            disabled={isSubmitting}
            id="permission-approval-note"
            onChange={(event) => setNote(event.target.value)}
            placeholder="补充授权理由、驳回原因或补证要求"
            value={note}
          />
          {noteRequired && !note.trim() ? (
            <p className="text-xs font-semibold text-v3-danger">该决策需要填写处理意见。</p>
          ) : null}
        </div>
        {isSubmitting ? (
          <p className="text-xs leading-5 text-v3-ink-3">
            正在提交，关闭弹窗后提交会在后台继续，可先处理其他审批。
          </p>
        ) : null}
        <DialogFooter>
          <V3Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {isSubmitting ? "关闭" : "取消"}
          </V3Button>
          <V3Button
            type="button"
            variant={action?.tone === "destructive" ? "danger" : "primary"}
            onClick={submit}
            disabled={!canSubmit || isSubmitting}
          >
            {isSubmitting ? "提交中" : "提交"}
          </V3Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function PermissionApprovalContext({ approval }: { approval: PermissionApproval }) {
  return (
    <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
      <div className="grid gap-3 text-xs sm:grid-cols-2">
        {approval.risk_level ? (
          <ContextField label="风险等级">
            <StatusPill tone={riskTone(approval.risk_level)} className="px-2 py-0.5 text-[11px]">
              {riskLevelLabel(approval.risk_level)}
            </StatusPill>
          </ContextField>
        ) : null}
        <ContextField label="审批主体">
          <span className="min-w-0 break-words">
            {permissionResourceTypeLabel(approval.resource_type)}
          </span>
        </ContextField>
        <ContextField label="申请人">
          <ObjectRef name={approval.requester_name} id={approval.requester_id} />
        </ContextField>
      </div>
      {approval.summary ? (
        <div className="mt-3 border-t border-v3-line pt-3">
          <div className="text-xs font-semibold text-v3-ink-2">摘要</div>
          <p className="mt-1 text-sm leading-5 text-v3-ink">{approval.summary}</p>
        </div>
      ) : null}
      <PermissionContextBody context={approval.context} />
    </div>
  );
}

/**
 * 上下文渲染：优先识别权限差异语义(current / after / diff / permission / role),
 * 有则渲染「当前 vs 变更后」对照；否则回退键值 pretty-print。
 */
function PermissionContextBody({ context }: { context: Record<string, unknown> }) {
  if (!context || Object.keys(context).length === 0) {
    return null;
  }

  const current = context.current;
  const after = context.after;
  const hasDiff =
    (current !== undefined && current !== null) || (after !== undefined && after !== null);

  const permissionItems = extractStringList(context.permission ?? context.permissions);
  const roleValue = context.role;

  return (
    <div className="mt-3 space-y-3 border-t border-v3-line pt-3">
      {hasDiff ? (
        <div>
          <div className="text-xs font-semibold text-v3-ink-2">当前 vs 变更后</div>
          <div className="mt-2 grid gap-3 sm:grid-cols-2">
            <div className="min-w-0 rounded-v3-inner border border-v3-line bg-v3-card p-2.5">
              <div className="text-[11px] font-semibold text-v3-ink-3">当前</div>
              <PrettyValue value={current} empty="（无）" />
            </div>
            <div className="min-w-0 rounded-v3-inner border border-v3-line bg-v3-brand-soft/40 p-2.5">
              <div className="text-[11px] font-semibold text-v3-brand-deep">变更后</div>
              <PrettyValue value={after} empty="（无）" />
            </div>
          </div>
        </div>
      ) : null}

      {typeof roleValue === "string" && roleValue.trim() ? (
        <div>
          <div className="text-xs font-semibold text-v3-ink-2">申请角色</div>
          <p className="mt-1 text-[13px] font-medium text-v3-ink">{roleValue}</p>
        </div>
      ) : null}

      {permissionItems.length > 0 ? (
        <div>
          <div className="text-xs font-semibold text-v3-ink-2">权限项</div>
          <ul className="mt-1.5 flex flex-wrap gap-1.5">
            {permissionItems.map((permission) => (
              <li
                key={permission}
                className="rounded-v3-inner border border-v3-line bg-v3-card px-2 py-0.5 font-mono text-[11px] text-v3-ink-2"
              >
                {permission}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {!hasDiff && permissionItems.length === 0 && !(typeof roleValue === "string") ? (
        <div>
          <div className="text-xs font-semibold text-v3-ink-2">上下文</div>
          <pre className="mt-1.5 max-h-56 overflow-auto rounded-v3-inner border border-v3-line bg-v3-card p-2.5 font-mono text-[11px] leading-5 text-v3-ink-2">
            {JSON.stringify(context, null, 2)}
          </pre>
        </div>
      ) : null}
    </div>
  );
}

function extractStringList(value: unknown): string[] {
  if (Array.isArray(value)) {
    return value.filter((entry): entry is string => typeof entry === "string" && entry.trim().length > 0);
  }
  if (typeof value === "string" && value.trim()) {
    return [value.trim()];
  }
  return [];
}

function PrettyValue({ value, empty }: { value: unknown; empty: string }) {
  if (value === undefined || value === null || value === "") {
    return <p className="mt-1 text-[13px] text-v3-ink-3">{empty}</p>;
  }
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") {
    return <p className="mt-1 break-words text-[13px] font-medium text-v3-ink">{String(value)}</p>;
  }
  if (Array.isArray(value) && value.every((entry) => typeof entry === "string")) {
    return (
      <ul className="mt-1 flex flex-wrap gap-1.5">
        {(value as string[]).map((entry) => (
          <li
            key={entry}
            className="rounded-v3-inner border border-v3-line bg-v3-card px-2 py-0.5 font-mono text-[11px] text-v3-ink-2"
          >
            {entry}
          </li>
        ))}
      </ul>
    );
  }
  return (
    <pre className="mt-1 max-h-40 overflow-auto font-mono text-[11px] leading-5 text-v3-ink-2">
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

function ContextField({ children, label }: { children: React.ReactNode; label: string }) {
  return (
    <div className="min-w-0 space-y-1">
      <div className="font-semibold text-v3-ink-2">{label}</div>
      <div className="min-w-0 text-[13px] leading-5 font-medium text-v3-ink">{children}</div>
    </div>
  );
}
