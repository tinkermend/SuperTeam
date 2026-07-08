import { Save, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
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
  updateTeamConstitution,
  type Team,
  type UpdateTeamConstitutionInput,
} from "@/lib/api/teams";

type TeamGovernanceTabProps = {
  apiOptions: ApiClientOptions;
  canEdit: boolean;
  onSaved?: () => void;
  team: Team;
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
  changed: boolean,
): Record<string, unknown> {
  if (!changed) {
    return approvalPolicyObject(sourcePolicy);
  }
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
  canEdit,
  onSaved,
  team,
  teamId,
}: TeamGovernanceTabProps) {
  const [hardRulesText, setHardRulesText] = useState(() => arrayText(team.constitution?.hard_rules));
  const [approvalPolicy, setApprovalPolicy] = useState<ApprovalPolicyForm>(() =>
    parseApprovalPolicy(team.constitution?.approval_policy),
  );
  const [approvalPolicyChanged, setApprovalPolicyChanged] = useState(false);

  useEffect(() => {
    setHardRulesText(arrayText(team.constitution?.hard_rules));
    setApprovalPolicy(parseApprovalPolicy(team.constitution?.approval_policy));
    setApprovalPolicyChanged(false);
  }, [team.constitution]);

  const constitutionInput = useMemo<UpdateTeamConstitutionInput>(
    () => ({
      ...(team.constitution ?? {}),
      approval_policy: serializeApprovalPolicy(
        approvalPolicy,
        team.constitution?.approval_policy,
        approvalPolicyChanged,
      ),
      hard_rules: lineList(hardRulesText),
    }),
    [approvalPolicy, approvalPolicyChanged, hardRulesText, team.constitution],
  );
  const preview = JSON.stringify(constitutionInput, null, 2);
  const saveMutation = useMutation({
    mutationFn: () => updateTeamConstitution(apiOptions, teamId, constitutionInput),
    onSuccess: () => {
      onSaved?.();
    },
  });
  const hardRuleCount = lineList(hardRulesText).length;

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <Card>
        <CardHeader>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <CardTitle className="text-base">治理策略编辑</CardTitle>
            <Badge variant="secondary">团队宪法</Badge>
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
            onChange={(next) => {
              setApprovalPolicy(next);
              setApprovalPolicyChanged(true);
            }}
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
              <Badge variant="outline">{hardRuleCount} 条</Badge>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">审批策略</span>
              <Badge variant="outline">{approvalPolicy.enabled ? "已启用" : "未启用"}</Badge>
            </div>
            <div className="flex flex-wrap gap-2">
              <V3Button disabled={!canEdit || saveMutation.isPending} onClick={() => saveMutation.mutate()}>
                <Save data-icon="inline-start" />
                保存宪法
              </V3Button>
            </div>
            {saveMutation.isSuccess ? <p className="text-muted-foreground">团队宪法已保存。</p> : null}
            {saveMutation.isError ? <p className="text-destructive">团队宪法保存失败。</p> : null}
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
