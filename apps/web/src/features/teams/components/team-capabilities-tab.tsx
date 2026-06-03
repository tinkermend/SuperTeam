import { BookOpen, Boxes, Link2, Network, Save } from "lucide-react";
import { useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createTeamGovernanceDraft,
  listTeamGovernanceDrafts,
  updateTeamGovernanceDraft,
  type GovernanceDraftInput,
  type TeamConfigRevision,
} from "@/lib/api/teams";

type CapabilityKind = "skill_bindings" | "mcp_bindings" | "knowledge_base_bindings" | "external_capability_bindings";

type CapabilitySection = {
  icon: typeof Network;
  key: CapabilityKind;
  label: string;
  sample: string;
};

const capabilitySections: CapabilitySection[] = [
  { icon: Network, key: "skill_bindings", label: "Skills", sample: "incident-diagnosis" },
  { icon: Boxes, key: "mcp_bindings", label: "MCP", sample: "ops-mcp-server" },
  { icon: BookOpen, key: "knowledge_base_bindings", label: "知识库", sample: "运维知识库" },
  { icon: Link2, key: "external_capability_bindings", label: "外部能力", sample: "告警系统" },
];

type TeamCapabilitiesTabProps = {
  apiOptions: ApiClientOptions;
  canEdit: boolean;
  currentRevision?: TeamConfigRevision;
  teamId: string;
};

export function TeamCapabilitiesTab({ apiOptions, canEdit, currentRevision, teamId }: TeamCapabilitiesTabProps) {
  const drafts = useQuery({
    queryKey: ["team-governance-drafts", teamId],
    queryFn: () => listTeamGovernanceDrafts(apiOptions, teamId),
  });
  const draft = drafts.data?.[0];
  const [localPolicy, setLocalPolicy] = useState<Record<string, unknown>>(
    () => draft?.capability_policy ?? currentRevision?.capability_policy ?? defaultCapabilityPolicy(),
  );
  const sourcePolicy = useMemo(
    () => ({ ...defaultCapabilityPolicy(), ...(draft?.capability_policy ?? currentRevision?.capability_policy ?? {}) }),
    [currentRevision?.capability_policy, draft?.capability_policy],
  );
  const effectivePolicy = { ...sourcePolicy, ...localPolicy };
  const saveMutation = useMutation({
    mutationFn: () => saveCapabilityDraft(apiOptions, teamId, draft, {
      capability_policy: effectivePolicy,
      constitution: draft?.constitution ?? currentRevision?.constitution ?? { hard_rules: ["能力绑定必须经过负责人批准"] },
      human_owner_user_id: draft?.human_owner_user_id ?? currentRevision?.human_owner_user_id,
    }),
    onSuccess: () => {
      void drafts.refetch();
    },
  });

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
      <div className="flex flex-col gap-4">
        <div className="grid gap-3 md:grid-cols-4">
          {capabilitySections.map((section) => (
            <CapabilityMetric key={section.key} policy={effectivePolicy} section={section} />
          ))}
        </div>
        <Card>
          <CardHeader>
            <CardTitle className="text-base">能力与知识绑定</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900">
              绑定不会立即生效，变更会先进入治理草稿，需要负责人批准后才会成为当前配置。
            </p>
            {capabilitySections.map((section) => (
              <CapabilityBindingRow
                canEdit={canEdit}
                key={section.key}
                onAdd={(value) => {
                  setLocalPolicy((policy) => appendPolicyBinding(policy, section.key, value));
                }}
                policy={effectivePolicy}
                section={section}
              />
            ))}
          </CardContent>
        </Card>
      </div>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">治理草稿变更</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">草稿状态</span>
            <Badge variant="secondary">{draft ? "草稿中" : "未创建"}</Badge>
          </div>
          <div className="flex items-center justify-between">
            <span className="text-muted-foreground">绑定来源</span>
            <Badge variant="outline">{draft ? "当前草稿" : "当前生效配置"}</Badge>
          </div>
          <Button disabled={!canEdit || saveMutation.isPending} onClick={() => saveMutation.mutate()}>
            <Save data-icon="inline-start" />
            保存绑定草稿
          </Button>
          {saveMutation.isError ? <p className="text-sm text-destructive">绑定草稿保存失败</p> : null}
          {saveMutation.isSuccess ? <p className="text-sm text-muted-foreground">已写入治理草稿。</p> : null}
        </CardContent>
      </Card>
    </div>
  );
}

function CapabilityMetric({ policy, section }: { policy: Record<string, unknown>; section: CapabilitySection }) {
  const Icon = section.icon;
  return (
    <Card>
      <CardContent className="flex items-center gap-3 p-4">
        <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
          <Icon />
        </div>
        <div>
          <p className="text-sm text-muted-foreground">{section.label} 数量</p>
          <p className="text-xl font-semibold">{policyList(policy, section.key).length}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function CapabilityBindingRow({
  canEdit,
  onAdd,
  policy,
  section,
}: {
  canEdit: boolean;
  onAdd: (value: string) => void;
  policy: Record<string, unknown>;
  section: CapabilitySection;
}) {
  const [value, setValue] = useState(section.sample);
  const bindings = policyList(policy, section.key);
  const Icon = section.icon;

  return (
    <div className="grid gap-3 rounded-md border p-3 md:grid-cols-[120px_minmax(0,1fr)_240px] md:items-center">
      <div className="flex items-center gap-2 font-medium">
        <Icon />
        {section.label}
      </div>
      <div className="flex flex-wrap gap-2">
        {bindings.length > 0 ? (
          bindings.map((binding) => (
            <Badge key={binding} variant="secondary">
              {binding}
            </Badge>
          ))
        ) : (
          <span className="text-sm text-muted-foreground">暂无绑定</span>
        )}
      </div>
      <div className="flex gap-2">
        <Input disabled={!canEdit} onChange={(event) => setValue(event.target.value)} value={value} />
        <Button
          disabled={!canEdit || value.trim().length === 0}
          onClick={() => onAdd(value)}
          type="button"
          variant="outline"
        >
          绑定
        </Button>
      </div>
    </div>
  );
}

function saveCapabilityDraft(
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

function defaultCapabilityPolicy(): Record<string, unknown> {
  return {
    external_capability_bindings: ["告警系统"],
    knowledge_base_bindings: ["运维知识库"],
    mcp_bindings: ["ops-mcp-server"],
    skill_bindings: ["incident-diagnosis"],
  };
}

function appendPolicyBinding(policy: Record<string, unknown>, key: CapabilityKind, value: string): Record<string, unknown> {
  const trimmed = value.trim();
  if (!trimmed) {
    return policy;
  }
  const bindings = new Set(policyList(policy, key));
  bindings.add(trimmed);
  return { ...policy, [key]: Array.from(bindings) };
}

function policyList(policy: Record<string, unknown>, key: CapabilityKind): string[] {
  const value = policy[key];
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((item): item is string => typeof item === "string");
}
