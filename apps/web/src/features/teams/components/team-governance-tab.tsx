import { Send, ShieldCheck, Save, XCircle } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
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
  const [principlesText, setPrinciplesText] = useState(() => arrayText(sourceRevision?.constitution.principles));
  const [approvalText, setApprovalText] = useState(() => jsonText(sourceRevision?.approval_policy));
  const [runtimeText, setRuntimeText] = useState(() => jsonText(sourceRevision?.runtime_scope_policy));

  useEffect(() => {
    if (!sourceRevision) {
      return;
    }
    setHardRulesText(arrayText(sourceRevision.constitution.hard_rules));
    setPrinciplesText(arrayText(sourceRevision.constitution.principles));
    setApprovalText(jsonText(sourceRevision.approval_policy));
    setRuntimeText(jsonText(sourceRevision.runtime_scope_policy));
  }, [sourceRevision]);

  const draftInput = useMemo<GovernanceDraftInput>(
    () => ({
      approval_policy: parseObjectText(approvalText),
      artifact_contract: sourceRevision?.artifact_contract ?? {},
      capability_policy: sourceRevision?.capability_policy ?? {},
      constitution: {
        ...(sourceRevision?.constitution ?? {}),
        hard_rules: lineList(hardRulesText),
        principles: lineList(principlesText),
      },
      context_policy: sourceRevision?.context_policy ?? {},
      human_owner_user_id: sourceRevision?.human_owner_user_id,
      internal_collaboration_policy: sourceRevision?.internal_collaboration_policy ?? {},
      runtime_scope_policy: parseObjectText(runtimeText),
    }),
    [approvalText, hardRulesText, principlesText, runtimeText, sourceRevision],
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
          <PolicyTextArea
            description="用于约束团队协作风格和工作边界。"
            disabled={!canEdit}
            label="原则"
            onChange={setPrinciplesText}
            value={principlesText}
          />
          <PolicyTextArea
            description="JSON 对象，定义风险阈值和必须人工审批的动作。"
            disabled={!canEdit}
            label="审批策略"
            onChange={setApprovalText}
            value={approvalText}
          />
          <PolicyTextArea
            description="JSON 对象，定义 Runtime、Provider 和环境范围。"
            disabled={!canEdit}
            label="Runtime 范围"
            onChange={setRuntimeText}
            value={runtimeText}
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
              <Badge variant="outline">{diff.data?.changed_approval_rules ? "有变更" : approvalText.trim() ? "已配置" : "未配置"}</Badge>
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
              <Button disabled={!canEdit || saveMutation.isPending} onClick={() => saveMutation.mutate()}>
                <Save data-icon="inline-start" />
                保存草稿
              </Button>
              <Button
                disabled={!canApprove || approveMutation.isPending || !draftID}
                onClick={() => approveMutation.mutate()}
                variant="outline"
              >
                <Send data-icon="inline-start" />
                提交负责人批准
              </Button>
              <Button
                disabled={!canApprove || rejectMutation.isPending || !draftID}
                onClick={() => rejectMutation.mutate()}
                variant="outline"
              >
                <XCircle data-icon="inline-start" />
                驳回草稿
              </Button>
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

function jsonText(value: unknown): string {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return "{}";
  }
  return JSON.stringify(value, null, 2);
}

function parseObjectText(value: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(value || "{}") as unknown;
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    return {};
  }
  return {};
}
