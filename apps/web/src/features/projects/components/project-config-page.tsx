import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient
} from "@tanstack/react-query";
import {
  Archive,
  Bot,
  Check,
  ChevronDown,
  ClipboardList,
  ExternalLink,
  GitBranch,
  UserRound
} from "lucide-react";
import { Link } from "@tanstack/react-router";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from "@/components/ui/collapsible";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import {
  IconTile,
  ObjectRef,
  SoftCard,
  StatusPill,
  Button,
  EmptyState,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";
import { listUsers } from "@/lib/api/auth";
import type { ApiClientOptions } from "@/lib/api/client";
import { listDigitalEmployees, type DigitalEmployee } from "@/lib/api/employees";
import {
  getProjectConfig,
  getProjectConfigRevision,
  listProjectConfigRevisions,
  listProjectTasks,
  replaceProjectMembers,
  updateProjectConfig,
  type ProjectConfig,
  type ProjectConfigRevision,
  type ProjectMember,
  type ProjectMemberInput,
  type ProjectTask,
  type UpdateProjectConfigInput
} from "@/lib/api/projects";
import {
  principalTypeLabel,
  projectRoleLabel,
  projectStatusLabel,
  statusLabel as genericStatusLabel,
  taskStatusLabel
} from "@/lib/status-labels";
import { compareIsoDesc, formatDateTime, formatRelativeTime } from "@/lib/format-time";
import { ProjectManagementShell } from "./project-management-shell";
import { ShellPageHeaderBack } from "@/components/layout/shell-page-header";
import { ProjectErrorState, ProjectLoadingState } from "./project-empty-states";
import { ProjectConfigRevisionHistory } from "./project-config-revision-history";

type ProjectConfigViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  projectId: string;
};

type ConfigDraft = {
  coordinationPolicy: string;
  description: string;
  goal: string;
  name: string;
};

type MemberDraft = {
  members: string;
};

type RevisionSelection = {
  projectId: string;
  revisionId?: string;
};

function statusTone(status: string): Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "paused" || status === "acceptance") return "warn";
  if (status === "configuring" || status === "draft") return "info";
  return "mute";
}

export function ProjectConfigView({
  apiBaseUrl,
  fetcher,
  projectId
}: ProjectConfigViewProps) {
  const queryClient = useQueryClient();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const configQuery = useQuery({
    queryKey: ["project-config", projectId],
    queryFn: () => getProjectConfig(apiOptions, projectId),
    placeholderData: keepPreviousData
});
  const tasksQuery = useQuery({
    queryKey: ["project-tasks", projectId],
    queryFn: () => listProjectTasks(apiOptions, projectId, { limit: 20 }),
    placeholderData: keepPreviousData
});
  const configRevisionsQuery = useQuery({
    queryKey: ["project-config-revisions", projectId],
    queryFn: () => listProjectConfigRevisions(apiOptions, projectId, { limit: 20 }),
    placeholderData: keepPreviousData
});
  const employeesQuery = useQuery({
    queryKey: ["digital-employees"],
    queryFn: () => listDigitalEmployees(apiOptions),
    placeholderData: keepPreviousData
});
  const employeeById = useMemo(() => {
    const map = new Map<string, DigitalEmployee>();
    for (const employee of employeesQuery.data ?? []) {
      map.set(employee.id, employee);
    }
    return map;
  }, [employeesQuery.data]);
  const usersQuery = useQuery({
    queryKey: ["auth-users", "member-name-lookup"],
    queryFn: () => listUsers({ ...apiOptions, limit: 200 }),
    placeholderData: keepPreviousData
});
  const userNameById = useMemo(() => {
    const map = new Map<string, string>();
    for (const user of usersQuery.data?.items ?? []) {
      const name = user.display_name?.trim() || user.username?.trim();
      if (name) {
        map.set(user.id, name);
      }
    }
    return map;
  }, [usersQuery.data]);

  const [draft, setDraft] = useState<ConfigDraft>(() => emptyConfigDraft());
  const [memberDraft, setMemberDraft] = useState<MemberDraft>({ members: "[]" });
  const [error, setError] = useState("");
  const [memberError, setMemberError] = useState("");
  const [hydratedProjectId, setHydratedProjectId] = useState("");
  const [isConfigDirty, setConfigDirty] = useState(false);
  const [isMembersDirty, setMembersDirty] = useState(false);
  const [revisionSelection, setRevisionSelection] = useState<RevisionSelection>(() => ({
    projectId
}));

  const projectConfigRevisions = useMemo(
    () =>
      (configRevisionsQuery.data ?? []).filter(
        (revision) => revision.project_id === projectId,
      ),
    [configRevisionsQuery.data, projectId],
  );
  const latestConfigRevision = useMemo(
    () => getLatestConfigRevision(projectConfigRevisions),
    [projectConfigRevisions],
  );

  useEffect(() => {
    if (!configRevisionsQuery.data) return;
    setRevisionSelection((current) => {
      const latestRevisionId = latestConfigRevision?.id;
      if (current.projectId !== projectId) {
        return { projectId, revisionId: latestRevisionId };
      }
      if (
        current.revisionId &&
        projectConfigRevisions.some((revision) => revision.id === current.revisionId)
      ) {
        return current;
      }
      if (current.revisionId === latestRevisionId) return current;
      return { projectId, revisionId: latestRevisionId };
    });
  }, [
    configRevisionsQuery.data,
    latestConfigRevision?.id,
    projectConfigRevisions,
    projectId,
  ]);

  const selectedRevisionId =
    revisionSelection.projectId === projectId ? revisionSelection.revisionId : undefined;
  const selectedRevisionFromList = projectConfigRevisions.find(
    (revision) => revision.id === selectedRevisionId,
  );
  const configRevisionDetailQuery = useQuery({
    queryKey: ["project-config-revision", projectId, selectedRevisionId],
    queryFn: () => {
      if (!selectedRevisionId) {
        throw new Error("未选择配置 revision");
      }
      return getProjectConfigRevision(apiOptions, projectId, selectedRevisionId);
    },
    enabled: Boolean(selectedRevisionId),
    placeholderData: keepPreviousData
});
  const selectedRevision =
    configRevisionDetailQuery.data?.id === selectedRevisionId
      ? configRevisionDetailQuery.data
      : selectedRevisionFromList;

  useEffect(() => {
    if (!configQuery.data) return;
    const projectChanged = hydratedProjectId !== projectId;
    if (projectChanged || !isConfigDirty) {
      setDraft(configToDraft(configQuery.data));
      setError("");
    }
    if (projectChanged || !isMembersDirty) {
      setMemberDraft(configToMemberDraft(configQuery.data));
      setMemberError("");
    }
    if (projectChanged) {
      setHydratedProjectId(projectId);
      setConfigDirty(false);
      setMembersDirty(false);
    }
  }, [
    configQuery.data,
    hydratedProjectId,
    isConfigDirty,
    isMembersDirty,
    projectId,
  ]);

  const config = configQuery.data;
  const isArchived = config?.project.status === "archived";

  const resolvePrincipalName = useMemo(() => {
    const byId = new Map<string, string>();
    for (const member of config?.members ?? []) {
      const name =
        member.display_name_snapshot?.trim() ||
        (member.principal_type === "digital_employee"
          ? employeeById.get(member.principal_id)?.name
          : userNameById.get(member.principal_id));
      if (name) {
        byId.set(member.principal_id, name);
      }
    }
    return (id: string | undefined | null): string | undefined => {
      if (!id) return undefined;
      const key = id.trim();
      return byId.get(key) ?? userNameById.get(key);
    };
  }, [config?.members, employeeById, userNameById]);
  const ownerIDs = useMemo<string[]>(() => {
    if (!config) return [];
    const ids = config.project.human_owner_user_ids;
    if (ids && ids.length > 0) return ids;
    return config.project.human_owner_user_id ? [config.project.human_owner_user_id] : [];
  }, [config]);

  const updateMutation = useMutation({
    mutationFn: (input: UpdateProjectConfigInput) =>
      updateProjectConfig(apiOptions, projectId, input),
    onSuccess: async (project) => {
      setConfigDirty(false);
      queryClient.setQueryData<ProjectConfig>(
        ["project-config", projectId],
        (current) =>
          current
            ? {
                ...current,
                coordination_policy: project.coordination_policy,
                project
}
            : current,
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-config", projectId] }),
        queryClient.invalidateQueries({
          queryKey: ["project-config-revisions", projectId]
}),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["projects"] }),
      ]);
    }
});

  const replaceMembersMutation = useMutation({
    mutationFn: (members: ProjectMemberInput[]) =>
      replaceProjectMembers(apiOptions, projectId, members),
    onSuccess: async (members) => {
      setMembersDirty(false);
      queryClient.setQueryData<ProjectConfig>(
        ["project-config", projectId],
        (current) =>
          current
            ? {
                ...current,
                digital_employee_pool: members.filter(
                  (member) => member.principal_type === "digital_employee",
                ),
                human_roles: members.filter(
                  (member) => member.principal_type === "human_user",
                ),
                members
}
            : current,
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-config", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["project-overview", projectId] }),
      ]);
    }
});
  const isConfigSaving = updateMutation.isPending;
  const isMembersSaving = replaceMembersMutation.isPending;
  const configFieldsDisabled = isArchived || isConfigSaving;

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

  function updateDraft(update: (current: ConfigDraft) => ConfigDraft) {
    setConfigDirty(true);
    setDraft(update);
  }

  function updateMemberDraft(members: string) {
    setMembersDirty(true);
    setMemberDraft({ members });
  }

  return (
    <ProjectManagementShell
      title="项目配置"
      description="成员、协调策略与配置修订历史"
      back={
        <ShellPageHeaderBack
          ariaLabel="返回项目运行详情"
          params={{ projectId }}
          to="/projects/$projectId"
        />
      }
    >
      {configQuery.isLoading ? <ProjectLoadingState /> : null}
      {configQuery.isError ? (
        <ProjectErrorState onRetry={() => void configQuery.refetch()} />
      ) : null}
      {config ? (
        <div className="grid gap-4">
          <SoftCard className="p-5">
            <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div className="flex min-w-0 items-start gap-3">
                <IconTile tone="brand" size="lg">
                  <GitBranch />
                </IconTile>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="truncate text-xl font-bold tracking-normal text-ink">
                      {config.project.name}
                    </h2>
                    <StatusPill tone={statusTone(config.project.status)}>
                      {projectStatusLabel(config.project.status)}
                    </StatusPill>
                  </div>
                  <p className="mt-1 max-w-3xl text-sm text-ink-2">
                    {config.project.goal}
                  </p>
                </div>
              </div>
              <Button
                disabled={configFieldsDisabled}
                type="button"
                onClick={saveConfig}
              >
                <Check data-icon="inline-start" />
                保存配置
              </Button>
            </div>
            {isArchived ? (
              <Alert className="mt-4 border-warn/30 bg-warn-soft text-ink">
                <Archive className="text-warn" />
                <AlertTitle>项目已归档</AlertTitle>
                <AlertDescription>配置页只读，保存与成员替换已禁用。</AlertDescription>
              </Alert>
            ) : null}
            {isConfigDirty ? (
              <Alert className="mt-4 border-info/30 bg-info-soft text-ink">
                <GitBranch className="text-info" />
                <AlertTitle>协调 Workflow 将收到配置变更</AlertTitle>
                <AlertDescription>
                  保存后会向当前项目协调 Workflow 发送策略变更 signal，新的项目任务将使用最新策略。
                </AlertDescription>
              </Alert>
            ) : null}
            {error || updateMutation.error ? (
              <p className="mt-3 text-sm text-destructive">
                {error || updateMutation.error?.message}
              </p>
            ) : null}
          </SoftCard>

          <ProjectConfigRevisionHistory
            error={
              configRevisionsQuery.error?.message ||
              configRevisionDetailQuery.error?.message
            }
            isDetailLoading={configRevisionDetailQuery.isFetching}
            isLoading={configRevisionsQuery.isLoading}
            isRefreshing={configRevisionsQuery.isFetching}
            revisions={projectConfigRevisions}
            selectedRevision={selectedRevision}
            selectedRevisionId={selectedRevisionId}
            resolveUserName={resolvePrincipalName}
            onSelectRevision={(revisionId) =>
              setRevisionSelection({ projectId, revisionId })
            }
          />

          <Tabs defaultValue="overview" className="gap-4">
            <TabsList
              className="inline-flex h-auto w-fit flex-wrap gap-1 rounded-[14px] bg-card p-1.5 text-ink-2 shadow-card"
              data-slot="page-tab-list"
            >
              <ProjectConfigTab value="overview">概览</ProjectConfigTab>
              <ProjectConfigTab value="members">成员</ProjectConfigTab>
              <ProjectConfigTab value="coordination">协调策略</ProjectConfigTab>
              <ProjectConfigTab value="history">任务历史</ProjectConfigTab>
            </TabsList>

            <TabsContent value="overview">
              <SoftCard className="p-5">
                <div className="grid gap-4 lg:grid-cols-2">
                  <Field label="项目名称">
                    <Input
                      disabled={configFieldsDisabled}
                      value={draft.name}
                      onChange={(event) =>
                        updateDraft((current) => ({
                          ...current,
                          name: event.target.value
}))
                      }
                    />
                  </Field>
                  <Field label="项目负责人">
                    <div className="flex flex-wrap gap-2">
                      {ownerIDs.length === 0 ? (
                        <span className="text-[13px] text-ink-3">暂无负责人</span>
                      ) : (
                        ownerIDs.map((id) => (
                          <span
                            key={id}
                            className="inline-flex items-center rounded-[10px] border border-line bg-card-soft px-2.5 py-1 text-[13px] text-ink"
                          >
                            <ObjectRef id={id} name={resolvePrincipalName(id)} />
                          </span>
                        ))
                      )}
                    </div>
                    <span className="mt-1 block text-[12px] text-ink-3">
                      多个平级负责人,任一可审批/验收;在「成员」标签页增删负责人(至少保留一位)。
                    </span>
                  </Field>
                  <Field label="目标">
                    <Textarea
                      disabled={configFieldsDisabled}
                      value={draft.goal}
                      onChange={(event) =>
                        updateDraft((current) => ({
                          ...current,
                          goal: event.target.value
}))
                      }
                    />
                  </Field>
                  <Field label="协调线程">
                    <Input readOnly value={config.coordination_workflow.workflow_id} />
                  </Field>
                  <Field label="描述">
                    <Textarea
                      disabled={configFieldsDisabled}
                      value={draft.description}
                      onChange={(event) =>
                        updateDraft((current) => ({
                          ...current,
                          description: event.target.value
}))
                      }
                    />
                  </Field>
                </div>
              </SoftCard>
            </TabsContent>

            <TabsContent value="members">
              <div className="grid gap-4">
                <MembersHumanizedPanel
                  digitalMembers={config.digital_employee_pool}
                  employeeById={employeeById}
                  humanMembers={config.human_roles}
                  ownerUserIDs={ownerIDs}
                  resolveName={resolvePrincipalName}
                />
                <Collapsible className="grid gap-3">
                  <CollapsibleTrigger className="group flex w-full items-center justify-between gap-2 rounded-[14px] border border-line bg-card px-5 py-3 text-left shadow-card">
                    <div className="flex items-center gap-2">
                      <UserRound className="size-4 text-ink-3" />
                      <span className="font-semibold text-ink">高级：成员完整替换 JSON</span>
                      <span className="text-[12px] text-ink-3">按 principal_id 全量覆盖，仅供排障</span>
                    </div>
                    <ChevronDown className="size-4 text-ink-3 transition-transform group-data-[state=open]:rotate-180" />
                  </CollapsibleTrigger>
                  <CollapsibleContent>
                    <MemberJsonPanel
                      disabled={isArchived || isMembersSaving}
                      error={memberError || replaceMembersMutation.error?.message}
                      isSaving={isMembersSaving}
                      members={memberDraft.members}
                      showWorkflowImpactNotice={isMembersDirty}
                      onMembersChange={updateMemberDraft}
                      onSave={saveMembers}
                    />
                  </CollapsibleContent>
                </Collapsible>
              </div>
            </TabsContent>

            <TabsContent value="coordination">
              <CoordinationPolicyPanel
                disabled={configFieldsDisabled}
                value={draft.coordinationPolicy}
                onChange={(value) =>
                  updateDraft((current) => ({ ...current, coordinationPolicy: value }))
                }
              />
            </TabsContent>

            <TabsContent value="history">
              <TaskHistoryPanel tasks={tasksQuery.data ?? []} />
            </TabsContent>
          </Tabs>
        </div>
      ) : null}
    </ProjectManagementShell>
  );
}

function emptyConfigDraft(): ConfigDraft {
  return {
    coordinationPolicy: "{}",
    description: "",
    goal: "",
    name: ""
};
}

function getLatestConfigRevision(revisions: ProjectConfigRevision[]) {
  return revisions.reduce<ProjectConfigRevision | undefined>((latest, revision) => {
    if (!latest || revision.revision_number > latest.revision_number) return revision;
    return latest;
  }, undefined);
}

function configToDraft(config: ProjectConfig): ConfigDraft {
  return {
    coordinationPolicy: JSON.stringify(config.coordination_policy ?? {}, null, 2),
    description: config.project.description ?? "",
    goal: config.project.goal,
    name: config.project.name
};
}

function configToMemberDraft(config: ProjectConfig): MemberDraft {
  return {
    members: JSON.stringify(
      config.members.map((member) => ({
        display_name_snapshot: member.display_name_snapshot,
        principal_id: member.principal_id,
        principal_type: member.principal_type,
        project_role: member.project_role,
        settings: member.settings
})),
      null,
      2,
    )
};
}

function draftToInput(draft: ConfigDraft): UpdateProjectConfigInput {
  return {
    coordination_policy: parseJsonObject(draft.coordinationPolicy, "协调策略"),
    description: draft.description.trim() || undefined,
    goal: draft.goal.trim() || undefined,
    name: draft.name.trim() || undefined
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
  return parsed.map((member, index) => {
    if (!member || typeof member !== "object" || Array.isArray(member)) {
      throw new Error(`第 ${index + 1} 个成员必须是 JSON object`);
    }
    const candidate = member as Record<string, unknown>;
    if (
      candidate.principal_type !== "human_user" &&
      candidate.principal_type !== "digital_employee" &&
      candidate.principal_type !== "team"
    ) {
      throw new Error(`第 ${index + 1} 个成员 principal_type 无效`);
    }
    if (
      candidate.project_role !== "owner" &&
      candidate.project_role !== "executor" &&
      candidate.project_role !== "reviewer" &&
      candidate.project_role !== "observer"
    ) {
      throw new Error(`第 ${index + 1} 个成员 project_role 无效`);
    }
    if (typeof candidate.principal_id !== "string" || !candidate.principal_id.trim()) {
      throw new Error(`第 ${index + 1} 个成员 principal_id 不能为空`);
    }
    if (
      candidate.display_name_snapshot !== undefined &&
      typeof candidate.display_name_snapshot !== "string"
    ) {
      throw new Error(`第 ${index + 1} 个成员 display_name_snapshot 必须是字符串`);
    }
    if (
      candidate.settings !== undefined &&
      (!candidate.settings ||
        typeof candidate.settings !== "object" ||
        Array.isArray(candidate.settings))
    ) {
      throw new Error(`第 ${index + 1} 个成员 settings 必须是 JSON object`);
    }

    return {
      display_name_snapshot: candidate.display_name_snapshot as string | undefined,
      principal_id: candidate.principal_id.trim(),
      principal_type: candidate.principal_type,
      project_role: candidate.project_role,
      settings: candidate.settings as Record<string, unknown> | undefined
};
  });
}

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Label className="grid gap-2 text-ink">
      <span className="text-[13px] font-semibold">{label}</span>
      {children}
    </Label>
  );
}

function ProjectConfigTab({
  children,
  value
}: {
  children: ReactNode;
  value: string;
}) {
  return (
    <TabsTrigger
      className="h-auto rounded-[10px] px-4 py-2 text-[13px] font-semibold text-ink-2 shadow-none transition-colors hover:bg-card-soft hover:text-ink data-[state=active]:bg-brand-soft data-[state=active]:text-brand-deep data-[state=active]:shadow-none"
      data-slot="page-tab"
      value={value}
    >
      {children}
    </TabsTrigger>
  );
}

function CoordinationPolicyPanel({
  disabled,
  onChange,
  value
}: {
  disabled?: boolean;
  onChange: (value: string) => void;
  value: string;
}) {
  const parsed = useMemo<Record<string, unknown> | null>(() => {
    try {
      const result = JSON.parse(value || "{}") as unknown;
      if (!result || typeof result !== "object" || Array.isArray(result)) {
        return null;
      }
      return result as Record<string, unknown>;
    } catch {
      return null;
    }
  }, [value]);
  const invalid = parsed === null;

  function setKey(key: string, next: unknown) {
    const base: Record<string, unknown> = { ...(parsed ?? {}) };
    if (next === undefined) {
      delete base[key];
    } else {
      base[key] = next;
    }
    onChange(JSON.stringify(base, null, 2));
  }

  const requireReview = parsed?.require_human_review_for_new_demands === true;
  const maxIterationsRaw = parsed?.max_plan_iterations;
  const maxIterations =
    typeof maxIterationsRaw === "number" && Number.isFinite(maxIterationsRaw)
      ? String(maxIterationsRaw)
      : "";
  const controlsDisabled = disabled || invalid;

  return (
    <div className="grid gap-4">
      <SoftCard className="p-5">
        <div className="mb-4 flex items-center gap-2">
          <span className="text-brand [&_svg]:size-4">
            <GitBranch />
          </span>
          <h3 className="font-semibold text-ink">协调策略</h3>
        </div>
        <p className="mb-4 text-sm text-ink-2">
          驱动项目协调线程的规划与门禁行为，保存后对后续新任务生效。
        </p>
        {invalid ? (
          <Alert className="mb-4 border-warn/30 bg-warn-soft text-ink">
            <GitBranch className="text-warn" />
            <AlertTitle>协调策略 JSON 无法解析</AlertTitle>
            <AlertDescription>请在下方「高级」区修正 JSON 后再使用上方开关。</AlertDescription>
          </Alert>
        ) : null}
        <div className="grid gap-4">
          <label className="flex items-start justify-between gap-4 rounded-[12px] border border-line bg-card-soft p-4">
            <div className="min-w-0">
              <span className="text-[13px] font-semibold text-ink">新需求需人工复核</span>
              <p className="mt-0.5 text-[12px] text-ink-3">
                开启后，协调线程对新提交需求强制人工复核，并将新任务标记为需审批。
              </p>
            </div>
            <Switch
              aria-label="新需求需人工复核"
              checked={requireReview}
              disabled={controlsDisabled}
              onCheckedChange={(next) =>
                setKey("require_human_review_for_new_demands", next)
              }
            />
          </label>
          <div className="grid gap-2 rounded-[12px] border border-line bg-card-soft p-4">
            <span className="text-[13px] font-semibold text-ink">最大规划迭代次数</span>
            <p className="text-[12px] text-ink-3">
              对抗式返工的规划迭代上限（正整数）；留空使用平台默认。
            </p>
            <Input
              className="max-w-[200px]"
              disabled={controlsDisabled}
              inputMode="numeric"
              min={1}
              type="number"
              value={maxIterations}
              onChange={(event) => {
                const raw = event.target.value.trim();
                if (!raw) {
                  setKey("max_plan_iterations", undefined);
                  return;
                }
                const parsedNumber = Number(raw);
                if (!Number.isInteger(parsedNumber) || parsedNumber < 1) {
                  return;
                }
                setKey("max_plan_iterations", parsedNumber);
              }}
            />
          </div>
        </div>
      </SoftCard>
      <SoftCard className="p-5">
        <div className="mb-1.5 flex items-center gap-2">
          <GitBranch className="size-4 text-ink-3" />
          <h3 className="font-semibold text-ink">完整协调策略 JSON</h3>
        </div>
        <p className="mb-3 text-[12px] text-ink-3">
          上方开关会同步写入此处；其它识别键（adversarial_review_judges / review_gate_conditions /
          selection_score_threshold 等）在此直接编辑。
        </p>
        <Textarea
          aria-label="协调策略 JSON"
          className="min-h-[240px] font-mono text-xs"
          disabled={disabled}
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
      </SoftCard>
    </div>
  );
}

function MemberJsonPanel({
  disabled,
  error,
  isSaving,
  members,
  showWorkflowImpactNotice,
  onMembersChange,
  onSave
}: {
  disabled?: boolean;
  error?: string;
  isSaving?: boolean;
  members: string;
  showWorkflowImpactNotice?: boolean;
  onMembersChange: (members: string) => void;
  onSave: () => void;
}) {
  return (
    <SoftCard className="p-5">
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2">
          <UserRound className="size-4 text-brand" />
          <h3 className="font-semibold text-ink">成员完整替换 JSON</h3>
        </div>
        <Button disabled={disabled || isSaving} type="button" onClick={onSave}>
          保存成员池
        </Button>
      </div>
      {showWorkflowImpactNotice ? (
        <Alert className="mb-4 border-ok/30 bg-ok-soft text-ink">
          <UserRound className="text-ok" />
          <AlertTitle>数字员工池变更将影响新任务</AlertTitle>
          <AlertDescription>
            保存成员后会向当前项目协调 Workflow 发送成员变更 signal，后续分派只能使用最新 active 数字员工池。
          </AlertDescription>
        </Alert>
      ) : null}
      <Textarea
        aria-label="项目成员 JSON"
        className="min-h-[320px] font-mono text-xs"
        disabled={disabled}
        value={members}
        onChange={(event) => onMembersChange(event.target.value)}
      />
      {error ? <p className="mt-3 text-sm text-destructive">{error}</p> : null}
    </SoftCard>
  );
}

function MembersHumanizedPanel({
  digitalMembers,
  employeeById,
  humanMembers,
  ownerUserIDs,
  resolveName
}: {
  digitalMembers: ProjectMember[];
  employeeById: Map<string, DigitalEmployee>;
  humanMembers: ProjectMember[];
  ownerUserIDs: string[];
  resolveName: (id: string | undefined | null) => string | undefined;
}) {
  return (
    <div className="grid gap-4">
      <MemberGroup
        count={humanMembers.length}
        emptyLabel="暂无人类成员"
        icon={<UserRound />}
        title="人类成员"
      >
        {humanMembers.map((member) => (
          <ConfigMemberRow
            key={member.id}
            isOwner={ownerUserIDs.includes(member.principal_id)}
            member={member}
            resolvedName={resolveName(member.principal_id)}
          />
        ))}
      </MemberGroup>
      <MemberGroup
        count={digitalMembers.length}
        emptyLabel="暂无数字员工"
        icon={<Bot />}
        title="数字员工"
      >
        {digitalMembers.map((member) => (
          <ConfigMemberRow
            key={member.id}
            employee={employeeById.get(member.principal_id)}
            member={member}
            resolvedName={resolveName(member.principal_id)}
          />
        ))}
      </MemberGroup>
    </div>
  );
}

function MemberGroup({
  children,
  count,
  emptyLabel,
  icon,
  title
}: {
  children: ReactNode;
  count: number;
  emptyLabel: string;
  icon: ReactNode;
  title: string;
}) {
  return (
    <WorkSurface>
      <div className="flex items-center justify-between gap-3 border-b border-line p-4">
        <div className="flex items-center gap-2">
          <span className="text-brand [&_svg]:size-4">{icon}</span>
          <h3 className="font-semibold text-ink">{title}</h3>
        </div>
        <StatusPill tone="mute">{count} 个</StatusPill>
      </div>
      {count === 0 ? (
        <EmptyState title={emptyLabel} />
      ) : (
        <div className="divide-y divide-line">{children}</div>
      )}
    </WorkSurface>
  );
}

function ConfigMemberRow({
  employee,
  isOwner,
  member,
  resolvedName
}: {
  employee?: DigitalEmployee;
  isOwner?: boolean;
  member: ProjectMember;
  resolvedName?: string;
}) {
  const isDigital = member.principal_type === "digital_employee";
  const name =
    member.display_name_snapshot?.trim() || employee?.name || resolvedName || undefined;
  const description = employee?.description?.trim();
  return (
    <div className="flex items-start gap-3 p-4">
      {isDigital ? (
        <EmployeeAvatar
          asset={employeeAvatarAsset({
            id: member.principal_id,
            metadata: employee?.metadata
})}
          name={name || member.principal_id}
          size="sm"
        />
      ) : (
        <IconTile tone="brand" size="sm">
          <UserRound />
        </IconTile>
      )}
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="min-w-0 font-bold text-ink">
            <ObjectRef id={member.principal_id} name={name} />
          </span>
          {isOwner ? <StatusPill tone="info">负责人</StatusPill> : null}
          <StatusPill tone="mute">{projectRoleLabel(member.project_role)}</StatusPill>
        </div>
        {description ? (
          <p className="mt-1 line-clamp-2 text-sm text-ink-2">{description}</p>
        ) : null}
        <div className="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[12px] text-ink-3">
          <span>{principalTypeLabel(member.principal_type)}</span>
          <span aria-hidden>·</span>
          <span>{genericStatusLabel(member.status)}</span>
          <span aria-hidden>·</span>
          <span>加入于 {member.created_at ? formatDateTime(member.created_at) : "—"}</span>
          {isDigital ? (
            <>
              <span aria-hidden>·</span>
              <Link
                className="inline-flex items-center gap-1 text-brand hover:underline"
                params={{ employeeId: member.principal_id }}
                to="/employees/$employeeId"
              >
                查看详情
                <ExternalLink className="size-3" />
              </Link>
            </>
          ) : null}
        </div>
      </div>
    </div>
  );
}

function TaskHistoryPanel({ tasks }: { tasks: ProjectTask[] }) {
  const orderedTasks = [...tasks].sort((left, right) =>
    compareIsoDesc(left.updated_at ?? left.created_at, right.updated_at ?? right.created_at),
  );

  return (
    <WorkSurface>
      <div className="flex items-center justify-between gap-3 border-b border-line p-4">
        <div className="flex items-center gap-2">
          <ClipboardList className="size-4 text-brand" />
          <h3 className="font-semibold text-ink">任务历史</h3>
        </div>
        <StatusPill tone="mute">{tasks.length} 条</StatusPill>
      </div>
      <DataTable>
        <thead>
          <tr>
            <Th>任务</Th>
            <Th>状态</Th>
            <Th>更新</Th>
            <Th>摘要</Th>
          </tr>
        </thead>
        <tbody>
          {orderedTasks.length === 0 ? (
            <tr>
              <Td colSpan={4}>
                <EmptyState title="暂无任务历史" />
              </Td>
            </tr>
          ) : (
            orderedTasks.map((task) => {
              const activityAt = task.updated_at ?? task.created_at;
              return (
              <Tr key={task.id}>
                <Td className="min-w-[220px]">
                  <p className="truncate font-bold text-ink">{task.title}</p>
                  <p className="mt-0.5 truncate font-mono text-[12px] text-ink-3">
                    {task.id}
                  </p>
                </Td>
                <Td>
                  <StatusPill tone="info">{taskStatusLabel(task.status)}</StatusPill>
                </Td>
                <Td className="whitespace-nowrap tabular-nums text-xs text-ink-2">
                  {activityAt ? (
                    <time dateTime={activityAt} title={formatDateTime(activityAt)}>
                      {formatRelativeTime(activityAt)}
                    </time>
                  ) : (
                    "—"
                  )}
                </Td>
                <Td className="min-w-[320px] whitespace-normal text-ink-2">
                  <p className="line-clamp-2">{task.summary || "暂无摘要"}</p>
                </Td>
              </Tr>
              );
            })
          )}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}
