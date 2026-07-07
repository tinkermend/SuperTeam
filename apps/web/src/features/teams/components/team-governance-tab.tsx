import { Send, ShieldCheck, Save, XCircle } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { V3Button } from "@/components/superteam";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  approveTeamGovernanceDraft,
  createTeamGovernanceDraft,
  listTeamGovernanceDrafts,
  previewTeamGovernanceDiff,
  rejectTeamGovernanceDraft,
  updateTeamGovernanceDraft,
  type GovernanceDraftInput,
  type TeamConfigRevision,
} from "@/lib/api/teams";

type TeamGovernanceTabProps = {
  apiOptions: ApiClientOptions;
  canApprove: boolean;
  canEdit: boolean;
  currentRevision?: TeamConfigRevision;
  teamId: string;
};

type ApprovalPolicyForm = {
  enabled: boolean;
  risk_threshold: "low" | "medium" | "high";
  required_actions: string[];
  min_approvers: number;
};

const DEFAULT_APPROVAL_POLICY: ApprovalPolicyForm = {
  enabled: false,
  risk_threshold: "medium",
  required_actions: [],
  min_approvers: 1,
};

function parseApprovalPolicy(value: unknown): ApprovalPolicyForm {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return { ...DEFAULT_APPROVAL_POLICY };
  }
  const obj = value as Record<string, unknown>;
  const threshold = obj.risk_threshold;
  return {
    enabled: typeof obj.enabled === "boolean" ? obj.enabled : false,
    risk_threshold:
      threshold === "low" || threshold === "medium" || threshold === "high"
        ? threshold
        : "medium",
    required_actions: Array.isArray(obj.required_actions)
      ? obj.required_actions.filter((item): item is string => typeof item === "string")
      : [],
    min_approvers:
      typeof obj.min_approvers === "number" && obj.min_approvers >= 1
        ? Math.floor(obj.min_approvers)
        : 1,
  };
}

function approvalPolicyObject(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}

function serializeApprovalPolicy(
  form: ApprovalPolicyForm,
  sourcePolicy: unknown,
): Record<string, unknown> {
  return {
    ...approvalPolicyObject(sourcePolicy),
    enabled: form.enabled,
    risk_threshold: form.risk_threshold,
    required_actions: form.required_actions,
    min_approvers: form.min_approvers,
  };
}

export function TeamGovernanceTab({
  apiOptions,
  canApprove,
  canEdit,
  currentRevision,
  teamId,
}: TeamGovernanceTabProps) {
  const drafts = useQuery({
    queryKey: ["team-governance-drafts", teamId],
    queryFn: () => listTeamGovernanceDrafts(apiOptions, teamId),
  });
  const draft = drafts.data?.[0];
  const sourceRevision = draft ?? currentRevision;
  const [hardRulesText, setHardRulesText] = useState(() => arrayText(sourceRevision?.constitution.hard_rules));
  const [approvalPolicy, setApprovalPolicy] = useState<ApprovalPolicyForm>(() =>
    parseApprovalPolicy(sourceRevision?.approval_policy),
  );

  useEffect(() => {
    if (!sourceRevision) {
      return;
    }
    setHardRulesText(arrayText(sourceRevision.constitution.hard_rules));
    setApprovalPolicy(parseApprovalPolicy(sourceRevision.approval_policy));
  }, [sourceRevision]);

  const draftInput = useMemo<GovernanceDraftInput>(
    () => ({
      approval_policy: serializeApprovalPolicy(approvalPolicy, sourceRevision?.approval_policy),
      artifact_contract: sourceRevision?.artifact_contract ?? {},
      capability_policy: sourceRevision?.capability_policy ?? {},
      constitution: {
        ...(sourceRevision?.constitution ?? {}),
        hard_rules: lineList(hardRulesText),
      },
      context_policy: sourceRevision?.context_policy ?? {},
      human_owner_user_ids: sourceRevision?.human_owner_user_ids,
      internal_collaboration_policy: sourceRevision?.internal_collaboration_policy ?? {},
      runtime_scope_policy: sourceRevision?.runtime_scope_policy ?? {},
    }),
    [approvalPolicy, hardRulesText, sourceRevision],
  );
  const preview = JSON.stringify(draftInput, null, 2);
  const saveMutation = useMutation({
    mutationFn: () => saveGovernanceDraft(apiOptions, teamId, draft, draftInput),
    onSuccess: () => {
      void drafts.refetch();
    },
  });
  const approveMutation = useMutation({
    mutationFn: () => approveTeamGovernanceDraft(apiOptions, teamId, draft?.id ?? saveMutation.data?.id ?? ""),
    onSuccess: () => {
      void drafts.refetch();
    },
  });
  const draftID = draft?.id ?? saveMutation.data?.id;
  const diff = useQuery({
    enabled: Boolean(draftID),
    queryKey: ["team-governance-diff", teamId, draftID],
    queryFn: () => previewTeamGovernanceDiff(apiOptions, teamId, draftID ?? ""),
  });
  const rejectMutation = useMutation({
    mutationFn: () => rejectTeamGovernanceDraft(apiOptions, teamId, draftID ?? ""),
    onSuccess: () => {
      void drafts.refetch();
    },
  });

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <CardTitle className="text-base">治理策略编辑</CardTitle>
            <Badge variant="secondary">{draft ? "草稿版本" : "基于当前版本"}</Badge>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <PolicyTextArea
            description="每行一条负责人必须确认的硬性规则。"
            disabled={!canEdit}
            label="团队宪法"
            onChange={setHardRulesText}
            value={hardRulesText}
          />
          <ApprovalPolicyEditor
            disabled={!canEdit}
            onChange={setApprovalPolicy}
            value={approvalPolicy}
          />
        </CardContent>
      </Card>
      <div className="flex flex-col gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">JSON 快照预览</CardTitle>
          </CardHeader>
          <CardContent>
            <pre className="max-h-[460px] overflow-auto rounded-md border bg-muted p-3 text-xs">{preview}</pre>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base">相对当前版本的变更</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">硬性规则</span>
              <Badge variant="outline">{diff.data ? `+${diff.data.added_hard_rules}` : `${lineList(hardRulesText).length} 条`}</Badge>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">能力绑定</span>
              <Badge variant="outline">{diff.data?.changed_capabilities ? "有变更" : "无变更"}</Badge>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">审批策略</span>
              <Badge variant="outline">{diff.data?.changed_approval_rules ? "有变更" : approvalPolicy.enabled ? "已启用" : "未启用"}</Badge>
            </div>
            {diff.data?.warnings.length ? (
              <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-amber-900">
                {diff.data.warnings.map((issue) => (
                  <p key={`${issue.field}-${issue.message}`}>{issue.message}</p>
                ))}
              </div>
            ) : null}
            {diff.data?.blocking_errors.length ? (
              <div className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-destructive">
                {diff.data.blocking_errors.map((issue) => (
                  <p key={`${issue.field}-${issue.message}`}>{issue.message}</p>
                ))}
              </div>
            ) : null}
            <div className="flex flex-wrap gap-2">
              <V3Button disabled={!canEdit || saveMutation.isPending} onClick={() => saveMutation.mutate()}>
                <Save data-icon="inline-start" />
                保存草稿
              </V3Button>
              <V3Button
                disabled={!canApprove || approveMutation.isPending || !draftID}
                onClick={() => approveMutation.mutate()}
                variant="outline"
              >
                <Send data-icon="inline-start" />
                提交负责人批准
              </V3Button>
              <V3Button
                disabled={!canApprove || rejectMutation.isPending || !draftID}
                onClick={() => rejectMutation.mutate()}
                variant="outline"
              >
                <XCircle data-icon="inline-start" />
                驳回草稿
              </V3Button>
            </div>
            {saveMutation.isSuccess ? <p className="text-muted-foreground">治理草稿已保存。</p> : null}
            {saveMutation.isError ? <p className="text-destructive">治理草稿保存失败。</p> : null}
            {approveMutation.isSuccess ? <p className="text-muted-foreground">治理草稿已提交批准。</p> : null}
            {approveMutation.isError ? <p className="text-destructive">治理草稿提交失败。</p> : null}
            {rejectMutation.isSuccess ? <p className="text-muted-foreground">治理草稿已驳回。</p> : null}
            {rejectMutation.isError ? <p className="text-destructive">治理草稿驳回失败。</p> : null}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function PolicyTextArea({
  description,
  disabled,
  label,
  onChange,
  value,
}: {
  description: string;
  disabled: boolean;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  const id = `team-governance-${label}`;
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <ShieldCheck />
        <Label htmlFor={id}>{label}</Label>
      </div>
      <Textarea disabled={disabled} id={id} onChange={(event) => onChange(event.target.value)} rows={4} value={value} />
      <p className="text-xs text-muted-foreground">{description}</p>
    </div>
  );
}

function ApprovalPolicyEditor({
  disabled,
  onChange,
  value,
}: {
  disabled: boolean;
  onChange: (value: ApprovalPolicyForm) => void;
  value: ApprovalPolicyForm;
}) {
  return (
    <div className="flex flex-col gap-4 rounded-md border border-v3-line bg-v3-card-soft p-4">
      <div className="flex items-center gap-2">
        <ShieldCheck className="size-4" />
        <Label>审批策略</Label>
      </div>

      <label className="flex items-center gap-2 text-sm text-v3-ink">
        <Switch
          checked={value.enabled}
          disabled={disabled}
          onCheckedChange={(checked) => onChange({ ...value, enabled: checked })}
        />
        启用审批策略
      </label>

      <div className="grid gap-2">
        <Label htmlFor="approval-risk-threshold">风险阈值触发审批</Label>
        <Select
          disabled={disabled || !value.enabled}
          onValueChange={(next) =>
            onChange({ ...value, risk_threshold: next as ApprovalPolicyForm["risk_threshold"] })
          }
          value={value.risk_threshold}
        >
          <SelectTrigger aria-label="风险阈值" id="approval-risk-threshold">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="low">low - 低风险即触发</SelectItem>
            <SelectItem value="medium">medium - 中风险及以上触发</SelectItem>
            <SelectItem value="high">high - 仅高风险触发</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div className="grid gap-2">
        <Label htmlFor="approval-required-actions">必须审批的动作（每行一条）</Label>
        <Textarea
          aria-label="必须审批的动作（每行一条）"
          disabled={disabled || !value.enabled}
          id="approval-required-actions"
          onChange={(event) =>
            onChange({ ...value, required_actions: lineList(event.target.value) })
          }
          placeholder={"deploy\ndelete"}
          rows={3}
          value={value.required_actions.join("\n")}
        />
      </div>

      <div className="grid gap-2">
        <Label htmlFor="approval-min-approvers">最小审批人数</Label>
        <Input
          disabled={disabled || !value.enabled}
          id="approval-min-approvers"
          min={1}
          onChange={(event) =>
            onChange({
              ...value,
              min_approvers: Math.max(1, Math.floor(Number(event.target.value) || 1)),
            })
          }
          type="number"
          value={value.min_approvers}
        />
      </div>
    </div>
  );
}

function saveGovernanceDraft(
  apiOptions: ApiClientOptions,
  teamId: string,
  draft: TeamConfigRevision | undefined,
  input: GovernanceDraftInput,
) {
  if (draft) {
    return updateTeamGovernanceDraft(apiOptions, teamId, draft.id, input);
  }
  return createTeamGovernanceDraft(apiOptions, teamId, input);
}

function lineList(value: string): string[] {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function arrayText(value: unknown): string {
  if (!Array.isArray(value)) {
    return "";
  }
  return value.filter((item): item is string => typeof item === "string").join("\n");
}
