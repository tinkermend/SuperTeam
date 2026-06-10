import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  Archive,
  Bot,
  Check,
  FileArchive,
  GitBranch,
  ShieldCheck,
  UserRound,
} from "lucide-react";
import { Link } from "@tanstack/react-router";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  LiquidCard,
  LiquidTabsList,
  LiquidTabsTrigger,
  SemanticIconTile,
  StatusBadge,
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getProjectConfig,
  replaceProjectMembers,
  updateProjectConfig,
  type ProjectConfig,
  type ProjectMemberInput,
  type UpdateProjectConfigInput,
} from "@/lib/api/projects";
import { statusLabel, statusTone } from "./project-switcher-pane";
import { ProjectManagementShell } from "./project-management-shell";
import { ProjectErrorState, ProjectLoadingState } from "./project-empty-states";

type ProjectConfigViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  projectId: string;
};

type ConfigDraft = {
  approvalPolicy: string;
  coordinationPolicy: string;
  description: string;
  evidencePolicy: string;
  goal: string;
  name: string;
};

type MemberDraft = {
  members: string;
};

export function ProjectConfigView({
  apiBaseUrl,
  fetcher,
  projectId,
}: ProjectConfigViewProps) {
  const queryClient = useQueryClient();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const configQuery = useQuery({
    queryKey: ["project-config", projectId],
    queryFn: () => getProjectConfig(apiOptions, projectId),
    placeholderData: keepPreviousData,
  });

  const [draft, setDraft] = useState<ConfigDraft>(() => emptyConfigDraft());
  const [memberDraft, setMemberDraft] = useState<MemberDraft>({ members: "[]" });
  const [error, setError] = useState("");
  const [memberError, setMemberError] = useState("");

  useEffect(() => {
    if (!configQuery.data) return;
    setDraft(configToDraft(configQuery.data));
    setMemberDraft({
      members: JSON.stringify(
        configQuery.data.members.map((member) => ({
          display_name_snapshot: member.display_name_snapshot,
          principal_id: member.principal_id,
          principal_type: member.principal_type,
          project_role: member.project_role,
          settings: member.settings,
        })),
        null,
        2,
      ),
    });
    setError("");
    setMemberError("");
  }, [configQuery.data]);

  const config = configQuery.data;
  const isArchived = config?.project.status === "archived";

  const updateMutation = useMutation({
    mutationFn: (input: UpdateProjectConfigInput) =>
      updateProjectConfig(apiOptions, projectId, input),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-config", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
    },
  });

  const replaceMembersMutation = useMutation({
    mutationFn: (members: ProjectMemberInput[]) =>
      replaceProjectMembers(apiOptions, projectId, members),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-config", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
      ]);
    },
  });

  function saveConfig() {
    if (isArchived) return;
    try {
      const input = draftToInput(draft);
      setError("");
      updateMutation.mutate(input);
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : "配置 JSON 无效");
    }
  }

  function saveMembers() {
    if (isArchived) return;
    try {
      const members = parseMembers(memberDraft.members);
      setMemberError("");
      replaceMembersMutation.mutate(members);
    } catch (saveError) {
      setMemberError(saveError instanceof Error ? saveError.message : "成员 JSON 无效");
    }
  }

  return (
    <ProjectManagementShell
      title="项目配置"
      description="成员、数字员工池、协调策略、审批规则和证据归档"
      actions={
        <Button asChild variant="outline">
          <Link params={{ projectId }} to="/projects/$projectId">
            返回运行详情
          </Link>
        </Button>
      }
    >
      {configQuery.isLoading ? <ProjectLoadingState /> : null}
      {configQuery.isError ? (
        <ProjectErrorState onRetry={() => void configQuery.refetch()} />
      ) : null}
      {config ? (
        <div className="grid gap-4">
          <LiquidCard className="rounded-xl p-5">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div className="flex min-w-0 items-start gap-3">
                <SemanticIconTile tone="primary" size="lg">
                  <GitBranch />
                </SemanticIconTile>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="truncate text-xl font-semibold tracking-normal">
                      {config.project.name}
                    </h2>
                    <StatusBadge tone={statusTone(config.project.status)}>
                      {statusLabel(config.project.status)}
                    </StatusBadge>
                  </div>
                  <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
                    {config.project.goal}
                  </p>
                </div>
              </div>
              <Button
                disabled={isArchived || updateMutation.isPending}
                type="button"
                onClick={saveConfig}
              >
                <Check data-icon="inline-start" />
                保存配置
              </Button>
            </div>
            {isArchived ? (
              <Alert className="mt-4 border-[color:var(--superteam-warning)]/30 bg-white/70">
                <Archive className="text-[color:var(--superteam-warning)]" />
                <AlertTitle>项目已归档</AlertTitle>
                <AlertDescription>配置页只读，保存与成员替换已禁用。</AlertDescription>
              </Alert>
            ) : null}
            {error || updateMutation.error ? (
              <p className="mt-3 text-sm text-destructive">
                {error || updateMutation.error?.message}
              </p>
            ) : null}
          </LiquidCard>

          <Tabs defaultValue="overview" className="gap-4">
            <LiquidTabsList>
              <LiquidTabsTrigger value="overview">概览</LiquidTabsTrigger>
              <LiquidTabsTrigger value="members">成员</LiquidTabsTrigger>
              <LiquidTabsTrigger value="digital">数字员工池</LiquidTabsTrigger>
              <LiquidTabsTrigger value="coordination">协调策略</LiquidTabsTrigger>
              <LiquidTabsTrigger value="approval">审批规则</LiquidTabsTrigger>
              <LiquidTabsTrigger value="evidence">证据归档</LiquidTabsTrigger>
            </LiquidTabsList>

            <TabsContent value="overview">
              <LiquidCard className="rounded-xl p-5">
                <div className="grid gap-4 lg:grid-cols-2">
                  <Field label="项目名称">
                    <Input
                      disabled={isArchived}
                      value={draft.name}
                      onChange={(event) =>
                        setDraft((current) => ({ ...current, name: event.target.value }))
                      }
                    />
                  </Field>
                  <Field label="协调线程">
                    <Input readOnly value={config.coordination_workflow.workflow_id} />
                  </Field>
                  <Field label="目标">
                    <Textarea
                      disabled={isArchived}
                      value={draft.goal}
                      onChange={(event) =>
                        setDraft((current) => ({ ...current, goal: event.target.value }))
                      }
                    />
                  </Field>
                  <Field label="描述">
                    <Textarea
                      disabled={isArchived}
                      value={draft.description}
                      onChange={(event) =>
                        setDraft((current) => ({
                          ...current,
                          description: event.target.value,
                        }))
                      }
                    />
                  </Field>
                </div>
              </LiquidCard>
            </TabsContent>

            <TabsContent value="members">
              <MemberJsonPanel
                disabled={isArchived}
                error={memberError || replaceMembersMutation.error?.message}
                isSaving={replaceMembersMutation.isPending}
                members={memberDraft.members}
                onMembersChange={(members) => setMemberDraft({ members })}
                onSave={saveMembers}
              />
            </TabsContent>

            <TabsContent value="digital">
              <MembersPanel
                icon={<Bot />}
                title="数字员工池"
                members={config.digital_employee_pool}
              />
            </TabsContent>

            <TabsContent value="coordination">
              <PolicyPanel
                disabled={isArchived}
                icon={<GitBranch />}
                label="协调策略 JSON"
                value={draft.coordinationPolicy}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, coordinationPolicy: value }))
                }
              />
            </TabsContent>

            <TabsContent value="approval">
              <PolicyPanel
                disabled={isArchived}
                icon={<ShieldCheck />}
                label="审批规则 JSON"
                value={draft.approvalPolicy}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, approvalPolicy: value }))
                }
              />
            </TabsContent>

            <TabsContent value="evidence">
              <PolicyPanel
                disabled={isArchived}
                icon={<FileArchive />}
                label="证据归档 JSON"
                value={draft.evidencePolicy}
                onChange={(value) =>
                  setDraft((current) => ({ ...current, evidencePolicy: value }))
                }
              />
            </TabsContent>
          </Tabs>
        </div>
      ) : null}
    </ProjectManagementShell>
  );
}

function emptyConfigDraft(): ConfigDraft {
  return {
    approvalPolicy: "{}",
    coordinationPolicy: "{}",
    description: "",
    evidencePolicy: "{}",
    goal: "",
    name: "",
  };
}

function configToDraft(config: ProjectConfig): ConfigDraft {
  return {
    approvalPolicy: JSON.stringify(config.approval_policy ?? {}, null, 2),
    coordinationPolicy: JSON.stringify(config.coordination_policy ?? {}, null, 2),
    description: config.project.description ?? "",
    evidencePolicy: JSON.stringify(config.evidence_policy ?? {}, null, 2),
    goal: config.project.goal,
    name: config.project.name,
  };
}

function draftToInput(draft: ConfigDraft): UpdateProjectConfigInput {
  return {
    approval_policy: parseJsonObject(draft.approvalPolicy, "审批规则"),
    coordination_policy: parseJsonObject(draft.coordinationPolicy, "协调策略"),
    description: draft.description.trim() || undefined,
    evidence_policy: parseJsonObject(draft.evidencePolicy, "证据归档"),
    goal: draft.goal.trim() || undefined,
    name: draft.name.trim() || undefined,
  };
}

function parseJsonObject(value: string, label: string): Record<string, unknown> {
  const parsed = JSON.parse(value || "{}") as unknown;
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error(`${label}必须是 JSON object`);
  }
  return parsed as Record<string, unknown>;
}

function parseMembers(value: string): ProjectMemberInput[] {
  const parsed = JSON.parse(value || "[]") as unknown;
  if (!Array.isArray(parsed)) {
    throw new Error("成员必须是 JSON array");
  }
  return parsed as ProjectMemberInput[];
}

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Label className="grid gap-2">
      <span>{label}</span>
      {children}
    </Label>
  );
}

function PolicyPanel({
  disabled,
  icon,
  label,
  onChange,
  value,
}: {
  disabled?: boolean;
  icon: ReactNode;
  label: string;
  onChange: (value: string) => void;
  value: string;
}) {
  return (
    <LiquidCard className="rounded-xl p-5">
      <div className="mb-4 flex items-center gap-2">
        <span className="text-primary [&_svg]:size-4">{icon}</span>
        <h3 className="font-semibold">{label}</h3>
      </div>
      <Textarea
        aria-label={label}
        className="min-h-[280px] font-mono text-xs"
        disabled={disabled}
        value={value}
        onChange={(event) => onChange(event.target.value)}
      />
    </LiquidCard>
  );
}

function MemberJsonPanel({
  disabled,
  error,
  isSaving,
  members,
  onMembersChange,
  onSave,
}: {
  disabled?: boolean;
  error?: string;
  isSaving?: boolean;
  members: string;
  onMembersChange: (members: string) => void;
  onSave: () => void;
}) {
  return (
    <LiquidCard className="rounded-xl p-5">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2">
          <UserRound className="size-4 text-primary" />
          <h3 className="font-semibold">成员完整替换 JSON</h3>
        </div>
        <Button disabled={disabled || isSaving} type="button" onClick={onSave}>
          保存成员池
        </Button>
      </div>
      <Textarea
        aria-label="成员 JSON"
        className="min-h-[320px] font-mono text-xs"
        disabled={disabled}
        value={members}
        onChange={(event) => onMembersChange(event.target.value)}
      />
      {error ? <p className="mt-3 text-sm text-destructive">{error}</p> : null}
    </LiquidCard>
  );
}

function MembersPanel({
  icon,
  members,
  title,
}: {
  icon: ReactNode;
  members: ProjectConfig["members"];
  title: string;
}) {
  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex items-center gap-2">
          <span className="text-primary [&_svg]:size-4">{icon}</span>
          <h3 className="font-semibold">{title}</h3>
        </div>
        <span className="text-xs text-muted-foreground">{members.length} 个</span>
      </div>
      <div className="divide-y">
        {members.length === 0 ? (
          <div className="p-5 text-sm text-muted-foreground">暂无成员</div>
        ) : (
          members.map((member) => (
            <div className="flex items-center justify-between gap-3 p-4" key={member.id}>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">
                  {member.display_name_snapshot || member.principal_id}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {member.project_role} · {member.principal_type}
                </p>
              </div>
              <StatusBadge tone="neutral">{member.status}</StatusBadge>
            </div>
          ))
        )}
      </div>
    </LiquidCard>
  );
}
