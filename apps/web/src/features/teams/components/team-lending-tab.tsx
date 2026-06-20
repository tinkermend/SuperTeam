import { ArrowUpRight, Check, ShieldQuestion, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  approveTeamLendingRequest,
  getTeamLendingPolicy,
  listTeamLendingRequests,
  rejectTeamLendingRequest,
  revokeTeamLendingRequest,
  upsertTeamLendingPolicy,
  type TeamLendingApprovalMode,
  type TeamLendingPolicy,
  type TeamLendingRequest,
  type TeamLendingRequestStatus,
} from "@/lib/api/teams";

type TeamLendingTabProps = {
  apiOptions: ApiClientOptions;
  canEdit: boolean;
  canDecide: boolean;
  teamId: string;
};

const STATUS_LABEL: Record<TeamLendingRequestStatus, string> = {
  pending: "待审批",
  auto_approved: "自动放行",
  approved: "已通过",
  rejected: "已驳回",
  revoked: "已撤销",
};

function statusTone(status: TeamLendingRequestStatus): "default" | "secondary" | "destructive" {
  if (status === "approved" || status === "auto_approved") return "default";
  if (status === "rejected") return "destructive";
  return "secondary";
}

export function TeamLendingTab({ apiOptions, canEdit, canDecide, teamId }: TeamLendingTabProps) {
  const queryClient = useQueryClient();
  const policyQuery = useQuery({
    queryKey: ["team-lending-policy", teamId],
    queryFn: () => getTeamLendingPolicy(apiOptions, teamId),
    retry: false,
  });
  const requestsQuery = useQuery({
    queryKey: ["team-lending-requests", teamId],
    queryFn: () => listTeamLendingRequests(apiOptions, teamId),
  });

  const policy: TeamLendingPolicy | undefined = policyQuery.data;
  const policyNotConfigured = policyQuery.isError && !policyQuery.isLoading;

  const [allowLending, setAllowLending] = useState(false);
  const [approvalMode, setApprovalMode] = useState<TeamLendingApprovalMode>("manual");
  const [budgetCeiling, setBudgetCeiling] = useState("");
  const [capabilityText, setCapabilityText] = useState("{}");

  useEffect(() => {
    if (policy) {
      setAllowLending(policy.allow_lending);
      setApprovalMode(policy.approval_mode);
      setBudgetCeiling(policy.budget_ceiling ?? "");
      setCapabilityText(JSON.stringify(policy.capability_ceiling ?? {}, null, 0));
    }
  }, [policy]);

  useEffect(() => {
    if (policyNotConfigured) {
      setAllowLending(false);
      setApprovalMode("manual");
      setBudgetCeiling("");
      setCapabilityText("{}");
    }
  }, [policyNotConfigured]);

  const saveMutation = useMutation({
    mutationFn: (input: { allow_lending: boolean; approval_mode: TeamLendingApprovalMode; budget_ceiling: string; capability: Record<string, unknown> }) =>
      upsertTeamLendingPolicy(apiOptions, teamId, {
        allow_lending: input.allow_lending,
        approval_mode: input.approval_mode,
        budget_ceiling: input.budget_ceiling,
        capability_ceiling: input.capability,
      }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["team-lending-policy", teamId] }),
  });

  const decideMutation = useMutation({
    mutationFn: async (args: { action: "approve" | "reject" | "revoke"; requestId: string; reason: string; grantedBudget: string }) => {
      const input = { decision_reason: args.reason, granted_budget: args.grantedBudget || undefined };
      if (args.action === "approve") return approveTeamLendingRequest(apiOptions, teamId, args.requestId, input);
      if (args.action === "reject") return rejectTeamLendingRequest(apiOptions, teamId, args.requestId, { decision_reason: args.reason });
      return revokeTeamLendingRequest(apiOptions, teamId, args.requestId, { decision_reason: args.reason });
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: ["team-lending-requests", teamId] }),
  });

  const parsedCapability = useMemo<Record<string, unknown>>(() => {
    try {
      const value = JSON.parse(capabilityText);
      return value && typeof value === "object" && !Array.isArray(value) ? (value as Record<string, unknown>) : {};
    } catch {
      return {};
    }
  }, [capabilityText]);

  const onSave = () => {
    saveMutation.mutate({
      allow_lending: allowLending,
      approval_mode: approvalMode,
      budget_ceiling: budgetCeiling.trim(),
      capability: parsedCapability,
    });
  };

  const requests = requestsQuery.data ?? [];
  const pending = requests.filter((r) => r.status === "pending");
  const history = requests.filter((r) => r.status !== "pending");

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-2">
          <CardTitle className="text-base">借调策略</CardTitle>
          {policy ? (
            <Badge variant={policy.allow_lending ? "default" : "secondary"}>
              {policy.allow_lending ? `允许借调 · ${policy.approval_mode === "auto" ? "自动放行" : "人工审批"}` : "已关闭借调"}
            </Badge>
          ) : policyNotConfigured ? (
            <Badge variant="secondary">待设置</Badge>
          ) : null}
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {policyQuery.isLoading ? (
            <p className="text-sm text-muted-foreground">加载中…</p>
          ) : (
            <>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={allowLending}
                  disabled={!canEdit}
                  onChange={(event) => setAllowLending(event.target.checked)}
                />
                允许本团队数字员工被项目借调
              </label>
              <div className="grid gap-2">
                <Label htmlFor="lending-approval-mode">审批模式</Label>
                <select
                  id="lending-approval-mode"
                  className="h-9 rounded-md border bg-background px-2 text-sm disabled:opacity-50"
                  value={approvalMode}
                  disabled={!canEdit}
                  onChange={(event) => setApprovalMode(event.target.value as TeamLendingApprovalMode)}
                >
                  <option value="manual">manual · 每次借调需团队负责人审批</option>
                  <option value="auto">auto · 符合天花板自动放行，超纲转人工</option>
                </select>
              </div>
              <div className="grid gap-2">
                <Label htmlFor="lending-budget-ceiling">预算天花板（十进制字符串，留空表示不限）</Label>
                <Input
                  id="lending-budget-ceiling"
                  value={budgetCeiling}
                  disabled={!canEdit}
                  placeholder="例如 1000"
                  onChange={(event) => setBudgetCeiling(event.target.value)}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="lending-capability">能力天花板（JSON，键存在性校验）</Label>
                <Textarea
                  id="lending-capability"
                  className="font-mono text-xs"
                  rows={3}
                  value={capabilityText}
                  disabled={!canEdit}
                  onChange={(event) => setCapabilityText(event.target.value)}
                />
              </div>
              {canEdit ? (
                <div className="flex items-center gap-3">
                  <Button onClick={onSave} disabled={saveMutation.isPending} size="sm">
                    {saveMutation.isPending ? "保存中…" : "保存策略"}
                  </Button>
                  {saveMutation.isError ? (
                    <span className="text-xs text-destructive">保存失败：{(saveMutation.error as Error).message}</span>
                  ) : null}
                  {saveMutation.isSuccess ? <span className="text-xs text-muted-foreground">已保存</span> : null}
                </div>
              ) : null}
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">待审批借调请求 {pending.length > 0 ? `(${pending.length})` : ""}</CardTitle>
        </CardHeader>
        <CardContent>
          {requestsQuery.isLoading ? (
            <p className="text-sm text-muted-foreground">加载中…</p>
          ) : pending.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无待审批请求。</p>
          ) : (
            <ul className="flex flex-col gap-3">
              {pending.map((request) => (
                <PendingRequestRow
                  key={request.id}
                  canDecide={canDecide}
                  deciding={decideMutation.isPending}
                  request={request}
                  onDecide={(action, reason, grantedBudget) =>
                    decideMutation.mutate({ action, requestId: request.id, reason, grantedBudget })
                  }
                />
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">借调历史</CardTitle>
        </CardHeader>
        <CardContent>
          {history.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无历史记录。</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {history.map((request) => (
                <li key={request.id} className="flex flex-wrap items-center gap-2 text-sm">
                  <Badge variant={statusTone(request.status)}>{STATUS_LABEL[request.status]}</Badge>
                  {request.is_exception ? <Badge variant="secondary">例外</Badge> : null}
                  <span className="text-muted-foreground">项目 {shortId(request.project_id)}</span>
                  <span>申请 {request.requested_budget || "—"}</span>
                  {request.granted_budget ? <span>· 授予 {request.granted_budget}</span> : null}
                  {request.decision_reason ? <span className="text-muted-foreground">· {request.decision_reason}</span> : null}
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

type PendingRequestRowProps = {
  canDecide: boolean;
  deciding: boolean;
  request: TeamLendingRequest;
  onDecide: (action: "approve" | "reject", reason: string, grantedBudget: string) => void;
};

function PendingRequestRow({ canDecide, deciding, request, onDecide }: PendingRequestRowProps) {
  const [reason, setReason] = useState("");
  const [grantedBudget, setGrantedBudget] = useState(request.requested_budget ?? "");
  return (
    <li className="flex flex-col gap-2 rounded-md border p-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        {request.is_exception ? (
          <Badge variant="destructive" className="gap-1">
            <ShieldQuestion className="h-3 w-3" />
            超纲例外
          </Badge>
        ) : (
          <Badge variant="secondary">常规</Badge>
        )}
        <span className="text-muted-foreground">项目 {shortId(request.project_id)}</span>
        <span>申请预算 {request.requested_budget || "—"}</span>
        <ArrowUpRight className="h-3 w-3 text-muted-foreground" />
        <span>事由：{request.request_reason || "（无）"}</span>
      </div>
      {canDecide ? (
        <div className="flex flex-col gap-2 md:flex-row md:items-end">
          <div className="grid flex-1 gap-1">
            <Label className="text-xs" htmlFor={`reason-${request.id}`}>裁决说明</Label>
            <Input id={`reason-${request.id}`} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="可选" />
          </div>
          <div className="grid gap-1">
            <Label className="text-xs" htmlFor={`grant-${request.id}`}>授予预算</Label>
            <Input id={`grant-${request.id}`} value={grantedBudget} onChange={(event) => setGrantedBudget(event.target.value)} placeholder="例如 1500" />
          </div>
          <div className="flex gap-2">
            <Button size="sm" disabled={deciding} onClick={() => onDecide("approve", reason, grantedBudget)}>
              <Check className="h-3 w-3" /> 通过
            </Button>
            <Button size="sm" variant="destructive" disabled={deciding} onClick={() => onDecide("reject", reason, grantedBudget)}>
              <X className="h-3 w-3" /> 驳回
            </Button>
          </div>
        </div>
      ) : null}
    </li>
  );
}

function shortId(value: string): string {
  return value.length > 8 ? `${value.slice(0, 8)}…` : value;
}
