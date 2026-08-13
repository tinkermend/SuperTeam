import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Plus } from "lucide-react";
import {
  Button,
  Callout,
  CopyableMono,
  DataTable,
  EmptyState,
  ErrorState,
  LoadingState,
  Segmented,
  SoftDialog,
  SoftDialogBody,
  SoftDialogContent,
  SoftDialogDescription,
  SoftDialogFooter,
  SoftDialogHeader,
  SoftDialogTitle,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface,
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { Input } from "@/components/ui/input";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ApiRequestError } from "@/lib/api/client";
import {
  createExternalIntegration,
  issueExternalIntegrationToken,
  listExternalIntegrations,
  listExternalIntegrationTokens,
  revokeExternalIntegrationToken,
  updateExternalIntegration,
  type ExternalIntegration,
  type ExternalIntegrationAutonomyTier,
} from "@/lib/api/external-integrations";
import { getProjectConfig, listProjectMembers, listProjects } from "@/lib/api/projects";
import { listProjectSkillBindings } from "@/lib/api/skills";
import { listScenarioTemplates } from "@/lib/api/scenario-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { formatRelativeTime } from "@/lib/format-time";
import { statusLabel } from "@/lib/status-labels";

function errorMessage(error: unknown): string {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error) return error.message;
  return "操作失败";
}

function verbSummary(integration: ExternalIntegration): string {
  const verbs: string[] = [];
  if (integration.allow_chat_run) verbs.push("对话");
  if (integration.allow_demand_submit) verbs.push("发需求");
  return verbs.join(" / ") || "—";
}

export function ExternalIntegrationsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [tokenTarget, setTokenTarget] = useState<ExternalIntegration | null>(null);
  const [pendingDisable, setPendingDisable] = useState<ExternalIntegration | null>(null);
  const [actionError, setActionError] = useState("");

  const integrationsQuery = useQuery({
    queryKey: ["external-integrations", apiBaseUrl],
    queryFn: () => listExternalIntegrations({ baseUrl: apiBaseUrl }),
  });

  const projectsQuery = useQuery({
    queryKey: ["external-integrations-projects", apiBaseUrl],
    queryFn: () => listProjects({ baseUrl: apiBaseUrl }),
  });

  const projectNames = useMemo(() => {
    const map = new Map<string, string>();
    for (const project of projectsQuery.data ?? []) {
      map.set(project.id, project.name);
    }
    return map;
  }, [projectsQuery.data]);

  const statusMutation = useMutation({
    mutationFn: (input: { id: string; status: "active" | "disabled" }) =>
      updateExternalIntegration({ baseUrl: apiBaseUrl }, input.id, { status: input.status }),
    onSuccess: () => {
      setActionError("");
      void queryClient.invalidateQueries({ queryKey: ["external-integrations", apiBaseUrl] });
    },
    onError: (error) => setActionError(errorMessage(error)),
  });

  const integrations = integrationsQuery.data?.integrations ?? [];

  return (
    <>
      <ShellPageHeader
        title="协作集成"
        subtitle="外部系统的预授权执行入口：绑定项目、员工、技能信封与自治档；调用出界即拒，项目/剧本收紧即时生效。"
      />
      <Main width="wide">
        <div className="mb-4 flex items-center justify-between gap-3">
          <p className="text-sm text-ink-2">
            两个动词：信封内对话（产出旁路隔离）与提交 plan/loop 需求（撞闸按生效档处理）。签发的 Token 与飞书通道凭据相互独立。
          </p>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus aria-hidden className="size-4" />
            新建集成
          </Button>
        </div>
        {actionError ? (
          <div className="mb-3">
            <Callout tone="danger">{actionError}</Callout>
          </div>
        ) : null}
        {integrationsQuery.isLoading ? (
          <LoadingState title="加载集成列表…" />
        ) : integrationsQuery.isError ? (
          <ErrorState
            title="集成列表加载失败"
            description={errorMessage(integrationsQuery.error)}
            onRetry={() => void integrationsQuery.refetch()}
          />
        ) : integrations.length === 0 ? (
          <EmptyState
            title="尚无外部集成"
            description="新建集成后签发专用 Token，外部系统即可在预授权信封内发起对话或提交需求。"
            action={<Button onClick={() => setCreateOpen(true)}>新建集成</Button>}
          />
        ) : (
          <WorkSurface>
            <DataTable>
              <thead>
                <tr>
                  <Th>名称 / 项目</Th>
                  <Th>动词</Th>
                  <Th>自治档位</Th>
                  <Th>技能信封</Th>
                  <Th>每小时预算</Th>
                  <Th>状态</Th>
                  <Th>创建时间</Th>
                  <Th>操作</Th>
                </tr>
              </thead>
              <tbody>
                {integrations.map((integration) => (
                  <Tr key={integration.id}>
                    <Td>
                      <div className="flex flex-col">
                        <span className="font-medium text-ink">{integration.name}</span>
                        <span className="text-xs text-ink-3">
                          {projectNames.get(integration.project_id) ?? "未命名项目"}
                        </span>
                      </div>
                    </Td>
                    <Td>{verbSummary(integration)}</Td>
                    <Td>{statusLabel(integration.autonomy_tier)}</Td>
                    <Td>
                      {integration.skill_ids.length > 0
                        ? `${integration.skill_ids.length} 个技能`
                        : "空信封"}
                    </Td>
                    <Td className="tabular-nums">{integration.max_calls_per_hour}</Td>
                    <Td>
                      <StatusPill tone={integration.status === "active" ? "ok" : "mute"}>
                        {statusLabel(integration.status)}
                      </StatusPill>
                    </Td>
                    <Td>
                      <time className="tabular-nums" dateTime={integration.created_at}>
                        {formatRelativeTime(integration.created_at)}
                      </time>
                    </Td>
                    <Td>
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => setTokenTarget(integration)}
                        >
                          <KeyRound aria-hidden className="size-3.5" />
                          Token
                        </Button>
                        {integration.status === "active" ? (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setPendingDisable(integration)}
                          >
                            停用
                          </Button>
                        ) : (
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() =>
                              statusMutation.mutate({ id: integration.id, status: "active" })
                            }
                          >
                            启用
                          </Button>
                        )}
                      </div>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </DataTable>
          </WorkSurface>
        )}
      </Main>

      <CreateIntegrationDialog
        apiBaseUrl={apiBaseUrl}
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => {
          setCreateOpen(false);
          void queryClient.invalidateQueries({ queryKey: ["external-integrations", apiBaseUrl] });
        }}
      />

      <IntegrationTokensDialog
        apiBaseUrl={apiBaseUrl}
        integration={tokenTarget}
        onOpenChange={(open) => {
          if (!open) setTokenTarget(null);
        }}
      />

      <ConfirmDialog
        open={Boolean(pendingDisable)}
        onOpenChange={(open) => {
          if (!open) setPendingDisable(null);
        }}
        title="停用集成"
        desc={`停用「${pendingDisable?.name ?? ""}」后，所有 Token 调用将被拒绝；已在途的运行不受影响。`}
        confirmText="停用"
        destructive
        handleConfirm={() => {
          if (pendingDisable) {
            statusMutation.mutate({ id: pendingDisable.id, status: "disabled" });
          }
          setPendingDisable(null);
        }}
      />
    </>
  );
}

function CreateIntegrationDialog({
  apiBaseUrl,
  open,
  onOpenChange,
  onCreated,
}: {
  apiBaseUrl: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [projectId, setProjectId] = useState("");
  const [employeeId, setEmployeeId] = useState("");
  const [allowChat, setAllowChat] = useState(true);
  const [allowDemand, setAllowDemand] = useState(false);
  const [skillIds, setSkillIds] = useState<string[]>([]);
  const [scenarioTemplateKey, setScenarioTemplateKey] = useState("");
  const [autonomyTier, setAutonomyTier] = useState<ExternalIntegrationAutonomyTier>("pause_at_gate");
  const [maxCallsPerHour, setMaxCallsPerHour] = useState("60");
  const [formError, setFormError] = useState("");

  useEffect(() => {
    if (!open) return;
    setName("");
    setDescription("");
    setProjectId("");
    setEmployeeId("");
    setAllowChat(true);
    setAllowDemand(false);
    setSkillIds([]);
    setScenarioTemplateKey("");
    setAutonomyTier("pause_at_gate");
    setMaxCallsPerHour("60");
    setFormError("");
  }, [open]);

  const projectsQuery = useQuery({
    queryKey: ["external-integration-form-projects", apiBaseUrl],
    queryFn: () => listProjects({ baseUrl: apiBaseUrl }),
    enabled: open,
  });

  const membersQuery = useQuery({
    queryKey: ["external-integration-form-members", apiBaseUrl, projectId],
    queryFn: () => listProjectMembers({ baseUrl: apiBaseUrl }, projectId),
    enabled: open && Boolean(projectId),
  });

  const skillBindingsQuery = useQuery({
    queryKey: ["external-integration-form-skills", apiBaseUrl, projectId],
    queryFn: () => listProjectSkillBindings({ baseUrl: apiBaseUrl }, projectId),
    enabled: open && Boolean(projectId),
  });

  const projectConfigQuery = useQuery({
    queryKey: ["external-integration-form-project-config", apiBaseUrl, projectId],
    queryFn: () => getProjectConfig({ baseUrl: apiBaseUrl }, projectId),
    enabled: open && Boolean(projectId),
  });

  const scenarioTemplatesQuery = useQuery({
    queryKey: ["external-integration-form-scenarios", apiBaseUrl],
    queryFn: () => listScenarioTemplates({ baseUrl: apiBaseUrl }),
    enabled: open,
  });

  const employees = useMemo(() => {
    const members = membersQuery.data ?? [];
    return members
      .filter((member) => member.principal_type === "digital_employee")
      .map((member) => ({
        id: member.principal_id,
        name: member.display_name_snapshot?.trim() || member.principal_id,
      }));
  }, [membersQuery.data]);

  const skillOptions = useMemo(() => {
    const rows = skillBindingsQuery.data ?? [];
    return rows.map((row) => ({
      id: row.skill_id,
      label: row.skill?.name?.trim() || row.skill?.slug?.trim() || row.skill_id.slice(0, 8),
    }));
  }, [skillBindingsQuery.data]);

  const projectCeiling = useMemo(() => {
    const raw = projectConfigQuery.data?.coordination_policy?.autonomy_ceiling;
    return raw === "pause_at_gate" || raw === "full_auto" ? raw : "";
  }, [projectConfigQuery.data]);

  const playbookCeiling = useMemo(() => {
    if (!scenarioTemplateKey) return "";
    const match = (scenarioTemplatesQuery.data ?? []).find(
      (template) => template.template_key === scenarioTemplateKey,
    );
    const raw = match?.spec?.autonomy_ceiling;
    return raw === "pause_at_gate" || raw === "full_auto" ? raw : "";
  }, [scenarioTemplateKey, scenarioTemplatesQuery.data]);

  const fullAutoBlocked =
    projectCeiling === "pause_at_gate" || playbookCeiling === "pause_at_gate";

  useEffect(() => {
    if (fullAutoBlocked && autonomyTier === "full_auto") {
      setAutonomyTier("pause_at_gate");
    }
  }, [fullAutoBlocked, autonomyTier]);

  const createMutation = useMutation({
    mutationFn: () =>
      createExternalIntegration(
        { baseUrl: apiBaseUrl },
        {
          name: name.trim(),
          description: description.trim() || undefined,
          project_id: projectId,
          digital_employee_id: employeeId,
          allow_chat_run: allowChat,
          allow_demand_submit: allowDemand,
          skill_ids: skillIds.length > 0 ? skillIds : undefined,
          scenario_template_key: scenarioTemplateKey || undefined,
          autonomy_tier: autonomyTier,
          max_calls_per_hour: Math.max(1, Number(maxCallsPerHour) || 60),
        },
      ),
    onSuccess: () => onCreated(),
    onError: (error) => setFormError(errorMessage(error)),
  });

  const canSubmit =
    Boolean(name.trim()) &&
    Boolean(projectId) &&
    Boolean(employeeId) &&
    (allowChat || allowDemand) &&
    (!allowChat || skillIds.length > 0) &&
    !createMutation.isPending;

  function toggleSkill(skillId: string) {
    setSkillIds((prev) =>
      prev.includes(skillId) ? prev.filter((id) => id !== skillId) : [...prev, skillId],
    );
  }

  return (
    <SoftDialog open={open} onOpenChange={onOpenChange}>
      <SoftDialogContent size="lg">
        <SoftDialogHeader>
          <SoftDialogTitle>新建外部集成</SoftDialogTitle>
          <SoftDialogDescription>
            开通即授权：绑定与档位在此拍板，之后调用无审批单。项目/剧本上限收紧会即时收紧本集成，放松不自动升档。
          </SoftDialogDescription>
        </SoftDialogHeader>
        <SoftDialogBody className="space-y-5">
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-2">集成名称</span>
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="例如：工单系统直通"
              />
            </label>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-2">说明（可选）</span>
              <Input
                value={description}
                onChange={(event) => setDescription(event.target.value)}
                placeholder="调用方与用途"
              />
            </label>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-2">项目</span>
              <Select
                value={projectId}
                onValueChange={(value) => {
                  setProjectId(value);
                  setEmployeeId("");
                  setSkillIds([]);
                }}
              >
                <SelectTrigger aria-label="项目">
                  <SelectValue placeholder="选择有发起权的项目" />
                </SelectTrigger>
                <SelectContent>
                  {(projectsQuery.data ?? []).map((project) => (
                    <SelectItem key={project.id} value={project.id}>
                      {project.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-2">数字员工</span>
              <Select value={employeeId} onValueChange={setEmployeeId}>
                <SelectTrigger aria-label="数字员工" disabled={!projectId}>
                  <SelectValue placeholder={projectId ? "选择项目内员工" : "先选择项目"} />
                </SelectTrigger>
                <SelectContent>
                  {employees.map((employee) => (
                    <SelectItem key={employee.id} value={employee.id}>
                      {employee.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
          </div>
          <div className="flex flex-col gap-2 text-sm">
            <span className="text-ink-2">开放动词（至少一个）</span>
            <div className="flex flex-wrap gap-5">
              <label className="flex items-center gap-2">
                <Checkbox
                  checked={allowChat}
                  onCheckedChange={(checked) => setAllowChat(checked === true)}
                />
                信封内对话（产出旁路隔离）
              </label>
              <label className="flex items-center gap-2">
                <Checkbox
                  checked={allowDemand}
                  onCheckedChange={(checked) => setAllowDemand(checked === true)}
                />
                提交 plan/loop 需求（进主轨）
              </label>
            </div>
          </div>
          <div className="flex flex-col gap-2 text-sm">
            <span className="text-ink-2">
              技能信封{allowChat ? "（对话动词必选）" : "（可选；仅对话动词使用）"}
            </span>
            {!projectId ? (
              <p className="text-xs text-ink-3">先选择项目后显示可绑定技能</p>
            ) : skillOptions.length === 0 ? (
              <p className="text-xs text-ink-3">
                项目尚未绑定技能
                {allowChat ? "；请先在项目配置中绑定技能，再开通对话动词" : ""}
              </p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {skillOptions.map((option) => (
                  <button
                    key={option.id}
                    type="button"
                    aria-pressed={skillIds.includes(option.id)}
                    onClick={() => toggleSkill(option.id)}
                    className={
                      skillIds.includes(option.id)
                        ? "rounded-lg border border-brand bg-brand/10 px-2.5 py-1 text-xs text-brand"
                        : "rounded-lg border border-line px-2.5 py-1 text-xs text-ink-2"
                    }
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            )}
            {allowChat && skillIds.length === 0 ? (
              <p className="text-xs text-danger">空信封不会展开为项目默认面；对话动词须显式勾选技能。</p>
            ) : null}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-2">场景剧本（可选，作用于需求动词）</span>
              <Select
                value={scenarioTemplateKey || "__none__"}
                onValueChange={(value) =>
                  setScenarioTemplateKey(value === "__none__" ? "" : value)
                }
              >
                <SelectTrigger aria-label="场景剧本">
                  <SelectValue placeholder="不绑定剧本" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__none__">不绑定剧本</SelectItem>
                  {(scenarioTemplatesQuery.data ?? [])
                    .filter((template) => template.status === "active")
                    .map((template) => (
                      <SelectItem key={template.template_key} value={template.template_key}>
                        {template.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </label>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="text-ink-2">每小时调用上限</span>
              <Input
                type="number"
                min={1}
                value={maxCallsPerHour}
                onChange={(event) => setMaxCallsPerHour(event.target.value)}
              />
            </label>
          </div>
          <div className="flex flex-col gap-2 text-sm">
            <span className="text-ink-2">自治档位</span>
            <Segmented
              aria-label="自治档位"
              value={autonomyTier}
              onChange={(next) => {
                if (fullAutoBlocked && next === "full_auto") {
                  return;
                }
                setAutonomyTier(next);
              }}
              options={[
                { value: "pause_at_gate", label: "遇闸暂停" },
                {
                  value: "full_auto",
                  label: fullAutoBlocked ? "完全自动化（受上限）" : "完全自动化",
                },
              ]}
            />
            <p className="text-xs text-ink-3">
              {fullAutoBlocked
                ? "项目或剧本上限为「遇闸暂停」，完全自动化不可选。"
                : "完全自动化仍会触发闸与决策记录，由策略自动放行（resolved_by=policy:集成）。"}
            </p>
          </div>
          {formError ? <Callout tone="danger">{formError}</Callout> : null}
        </SoftDialogBody>
        <SoftDialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button disabled={!canSubmit} onClick={() => createMutation.mutate()}>
            {createMutation.isPending ? "创建中…" : "创建集成"}
          </Button>
        </SoftDialogFooter>
      </SoftDialogContent>
    </SoftDialog>
  );
}

function IntegrationTokensDialog({
  apiBaseUrl,
  integration,
  onOpenChange,
}: {
  apiBaseUrl: string;
  integration: ExternalIntegration | null;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [issuedToken, setIssuedToken] = useState("");
  const [tokenError, setTokenError] = useState("");
  const integrationId = integration?.id ?? "";

  useEffect(() => {
    setIssuedToken("");
    setTokenError("");
  }, [integrationId]);

  const tokensQuery = useQuery({
    queryKey: ["external-integration-tokens", apiBaseUrl, integrationId],
    queryFn: () => listExternalIntegrationTokens({ baseUrl: apiBaseUrl }, integrationId),
    enabled: Boolean(integrationId),
  });

  const issueMutation = useMutation({
    mutationFn: () => issueExternalIntegrationToken({ baseUrl: apiBaseUrl }, integrationId),
    onSuccess: (result) => {
      setTokenError("");
      setIssuedToken(result.token);
      void queryClient.invalidateQueries({
        queryKey: ["external-integration-tokens", apiBaseUrl, integrationId],
      });
    },
    onError: (error) => setTokenError(errorMessage(error)),
  });

  const revokeMutation = useMutation({
    mutationFn: (tokenId: string) =>
      revokeExternalIntegrationToken({ baseUrl: apiBaseUrl }, integrationId, tokenId),
    onSuccess: () => {
      setTokenError("");
      void queryClient.invalidateQueries({
        queryKey: ["external-integration-tokens", apiBaseUrl, integrationId],
      });
    },
    onError: (error) => setTokenError(errorMessage(error)),
  });

  const tokens = tokensQuery.data?.tokens ?? [];

  return (
    <SoftDialog open={Boolean(integration)} onOpenChange={onOpenChange}>
      <SoftDialogContent size="md">
        <SoftDialogHeader>
          <SoftDialogTitle>集成 Token · {integration?.name ?? ""}</SoftDialogTitle>
          <SoftDialogDescription>
            专用凭据，只用于本集成两动词；与飞书 connector 凭据独立，可随时吊销。
          </SoftDialogDescription>
        </SoftDialogHeader>
        <SoftDialogBody className="space-y-4">
          {issuedToken ? (
            <Callout tone="warn">
              <div className="space-y-2">
                <p>明文只显示这一次，请立即复制保存：</p>
                <CopyableMono value={issuedToken} />
                <p className="text-xs">
                  调用方式：请求头 <span className="font-mono">Authorization: Bearer {"<token>"}</span>，
                  动词端点 <span className="font-mono">POST /api/v1/external/chat-runs</span> 与{" "}
                  <span className="font-mono">POST /api/v1/external/demands</span>。
                </p>
              </div>
            </Callout>
          ) : null}
          {tokenError ? <Callout tone="danger">{tokenError}</Callout> : null}
          {tokensQuery.isLoading ? (
            <LoadingState title="加载 Token…" />
          ) : tokens.length === 0 ? (
            <p className="text-sm text-ink-3">尚未签发 Token。</p>
          ) : (
            <ul className="space-y-2">
              {tokens.map((token) => (
                <li
                  key={token.id}
                  className="flex items-center justify-between gap-3 rounded-xl border border-line px-3 py-2 text-sm"
                >
                  <div className="flex flex-col">
                    <span className="font-mono text-xs text-ink-2">{token.id.slice(0, 8)}</span>
                    <span className="text-xs text-ink-3">
                      签发于 {formatRelativeTime(token.created_at)}
                      {token.last_used_at
                        ? ` · 最近使用 ${formatRelativeTime(token.last_used_at)}`
                        : " · 未使用"}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <StatusPill tone={token.status === "active" ? "ok" : "mute"}>
                      {statusLabel(token.status)}
                    </StatusPill>
                    {token.status === "active" ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={revokeMutation.isPending}
                        onClick={() => revokeMutation.mutate(token.id)}
                      >
                        吊销
                      </Button>
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
          )}
        </SoftDialogBody>
        <SoftDialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
          <Button disabled={issueMutation.isPending} onClick={() => issueMutation.mutate()}>
            {issueMutation.isPending ? "签发中…" : "签发新 Token"}
          </Button>
        </SoftDialogFooter>
      </SoftDialogContent>
    </SoftDialog>
  );
}
