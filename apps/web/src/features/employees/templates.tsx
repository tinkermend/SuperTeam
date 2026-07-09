import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Bot,
  CheckCircle2,
  LayoutTemplate,
  Sparkles,
} from "lucide-react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  createEmployeeTemplate,
  deleteEmployeeTemplate,
  listEmployeeTemplates,
  setEmployeeTemplateStatus,
  updateEmployeeTemplate,
  type EmployeeTemplate,
} from "@/lib/api/employee-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  templateCapabilityPreview,
  templateCapabilitySummary,
  templateDefaultInjectionLine,
  templateDefaultInjectionSummary,
  templateRisk,
  templateStatusLabel,
  templateStatusTone,
} from "./template-utils";

type TemplateViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

type TemplateDetailViewProps = TemplateViewProps & {
  templateType: string;
};

export function TemplateListPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <TemplateListView apiBaseUrl={apiBaseUrl} />;
}

export function TemplateDetailPage({ templateType }: { templateType: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <TemplateDetailView apiBaseUrl={apiBaseUrl} templateType={templateType} />;
}

export function TemplateListView({ apiBaseUrl, fetcher }: TemplateViewProps) {
  const state = useTemplateCatalog(apiBaseUrl, fetcher);
  const templates = useMemo(
    () => [...state.templates].sort((left, right) => left.label.localeCompare(right.label)),
    [state.templates],
  );
  const [formState, setFormState] = useState<{ mode: "create" | "edit"; template?: EmployeeTemplate } | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<EmployeeTemplate | null>(null);
  const [actionError, setActionError] = useState("");

  const statusMutation = useMutation({
    mutationFn: (template: EmployeeTemplate) =>
      setEmployeeTemplateStatus(
        state.apiOptions,
        template.id,
        template.status === "active" ? "disabled" : "active",
      ),
    onSuccess: () => {
      setActionError("");
      state.invalidate();
    },
    onError: (mutationError: unknown) => {
      setActionError(mutationError instanceof Error ? mutationError.message : "更新模板状态失败");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (template: EmployeeTemplate) => deleteEmployeeTemplate(state.apiOptions, template.id),
    onSuccess: () => {
      setActionError("");
      setDeleteTarget(null);
      state.invalidate();
    },
    onError: (mutationError: unknown) => {
      setActionError(mutationError instanceof Error ? mutationError.message : "删除模板失败");
    },
  });

  return (
    <TemplateShell
      title="数字员工模板"
      subtitle="创建和管理数字员工模板，用于在创建数字员工时带入默认画像与能力建议"
      actions={
        <>
          <V3Button variant="outline" onClick={() => setFormState({ mode: "create" })}>
            <LayoutTemplate className="size-4" />
            新建模板
          </V3Button>
          <V3Button asChild>
            <Link to="/employees/new">
              <Bot className="size-4" />
              创建数字员工
            </Link>
          </V3Button>
        </>
      }
    >
      <TemplateQuerySurface state={state}>
        <div className="flex flex-col gap-4">
          {actionError ? (
            <div className="flex items-start justify-between gap-3 rounded-v3-inner bg-v3-danger-soft px-4 py-3 text-[13px] font-semibold text-v3-danger">
              <span>{actionError}</span>
              <button
                type="button"
                className="shrink-0 text-v3-danger underline underline-offset-2"
                onClick={() => setActionError("")}
              >
                关闭
              </button>
            </div>
          ) : null}
          {templates.length === 0 ? (
            <SoftCard>
              <V3EmptyState
                title="还没有数字员工模板"
                description="点击“新建模板”创建第一个模板，定义默认角色、能力建议和治理策略默认值。"
                action={
                  <V3Button onClick={() => setFormState({ mode: "create" })}>
                    <LayoutTemplate className="size-4" />
                    新建模板
                  </V3Button>
                }
              />
            </SoftCard>
          ) : (
            <WorkSurface>
              <div className="flex flex-col gap-1 border-b border-v3-line px-5 py-4 md:flex-row md:items-end md:justify-between">
                <div>
                  <h2 className="text-[17px] font-bold text-v3-ink">模板目录</h2>
                  <p className="mt-1 text-[13px] text-v3-ink-2">
                    当前租户下可用于创建数字员工的模板。
                  </p>
                </div>
              </div>
              <V3Table>
                <thead>
                  <tr>
                    <V3Th>模板</V3Th>
                    <V3Th>默认角色</V3Th>
                    <V3Th>模板能力</V3Th>
                    <V3Th>默认注入</V3Th>
                    <V3Th>状态</V3Th>
                    <V3Th>操作</V3Th>
                  </tr>
                </thead>
                <tbody>
                  {templates.map((template) => (
                    <TemplateTableRow
                      key={template.id}
                      template={template}
                      onConfigure={() => setFormState({ mode: "edit", template })}
                      onToggleStatus={() => statusMutation.mutate(template)}
                      onDelete={() => setDeleteTarget(template)}
                    />
                  ))}
                </tbody>
              </V3Table>
            </WorkSurface>
          )}
        </div>
      </TemplateQuerySurface>

      <TemplateFormDialog
        open={formState !== null}
        mode={formState?.mode ?? "create"}
        template={formState?.template}
        apiOptions={state.apiOptions}
        onOpenChange={(open) => !open && setFormState(null)}
        onSaved={() => {
          setFormState(null);
          state.invalidate();
        }}
      />
      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        title="删除模板"
        desc={`确认删除模板"${deleteTarget?.label}"？删除后不可恢复，已创建的数字员工不受影响。`}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget)}
      />
    </TemplateShell>
  );
}

export function TemplateDetailView({ apiBaseUrl, fetcher, templateType }: TemplateDetailViewProps) {
  const state = useTemplateCatalog(apiBaseUrl, fetcher);
  const template = useMemo(
    () => state.templates.find((item) => item.type === templateType),
    [state.templates, templateType],
  );

  return (
    <TemplateShell
      title="模板详情"
      subtitle="查看模板默认画像、注入策略和治理策略默认值"
      back={<ShellPageHeaderBack ariaLabel="返回数字员工模板列表" to="/employees/templates" />}
    >
      <TemplateQuerySurface state={state}>
        {!template ? (
          <SoftCard>
            <V3EmptyState
              title="模板不存在"
              description="当前租户下没有找到这个模板。"
              action={
                <V3Button asChild variant="outline">
                  <Link to="/employees/templates">返回模板管理</Link>
                </V3Button>
              }
            />
          </SoftCard>
        ) : (
          <TemplateDetailContent template={template} />
        )}
      </TemplateQuerySurface>
    </TemplateShell>
  );
}

function TemplateShell({
  actions,
  back,
  children,
  subtitle,
  title,
}: {
  actions?: React.ReactNode;
  back?: React.ReactNode;
  children: React.ReactNode;
  subtitle: string;
  title: string;
}) {
  return (
    <>
      <ShellPageHeader
        back={back}
        icon={back ? undefined : <LayoutTemplate />}
        iconTone="brand"
        subtitle={subtitle}
        title={title}
      />
      <Main>
        <div className="flex flex-col gap-5">
          {actions ? (
            <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
              {actions}
            </div>
          ) : null}
          {children}
        </div>
      </Main>
    </>
  );
}

function TemplateQuerySurface({
  children,
  state,
}: {
  children: React.ReactNode;
  state: ReturnType<typeof useTemplateCatalog>;
}) {
  if (state.isLoading) {
    return (
      <SoftCard>
        <V3LoadingState label="加载模板目录..." />
      </SoftCard>
    );
  }

  if (state.error) {
    return (
      <V3ErrorState
        title="加载模板失败"
        description={state.error instanceof Error ? state.error.message : undefined}
        onRetry={() => {
          void state.refetch();
        }}
      />
    );
  }

  return <>{children}</>;
}

function TemplateTableRow({
  template,
  onConfigure,
  onDelete,
  onToggleStatus,
}: {
  template: EmployeeTemplate;
  onConfigure: () => void;
  onDelete: () => void;
  onToggleStatus: () => void;
}) {
  return (
    <V3Tr>
      <V3Td className="min-w-[240px]">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone="brand" size="sm">
            <Sparkles />
          </IconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="font-bold text-v3-ink">{template.label}</p>
              {template.is_system ? (
                <StatusPill tone="info" showDot={false}>
                  内置
                </StatusPill>
              ) : null}
            </div>
            <p className="mt-1 line-clamp-2 max-w-md text-[12px] text-v3-ink-2">
              {template.description}
            </p>
          </div>
        </div>
      </V3Td>
      <V3Td>
        <code className="rounded-md bg-v3-card-soft px-2 py-1 font-mono text-[12px] text-v3-ink">
          {template.default_role}
        </code>
      </V3Td>
      <V3Td>{templateCapabilityPreview(template)}</V3Td>
      <V3Td>{templateDefaultInjectionLine(template)}</V3Td>
      <V3Td>
        <StatusPill tone={templateStatusTone(template)}>{templateStatusLabel(template)}</StatusPill>
      </V3Td>
      <V3Td>
        <div className="flex items-center gap-2">
          <V3Button asChild variant="outline" size="sm">
            <Link
              aria-label={`查看${template.label}模板详情`}
              to="/employees/templates/$templateType"
              params={{ templateType: template.type }}
            >
              查看详情
            </Link>
          </V3Button>
          <V3Button variant="outline" size="sm" onClick={onConfigure}>
            配置
          </V3Button>
          <V3Button variant="outline" size="sm" onClick={onToggleStatus}>
            {template.status === "active" ? "禁用" : "启用"}
          </V3Button>
          <V3Button variant="outline" size="sm" onClick={onDelete}>
            删除
          </V3Button>
        </div>
      </V3Td>
    </V3Tr>
  );
}

function TemplateDetailContent({ template }: { template: EmployeeTemplate }) {
  const capability = templateCapabilitySummary(template);
  const defaultInjection = templateDefaultInjectionSummary(template);

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="flex min-w-0 flex-col gap-4">
        <SoftCard className="p-5">
          <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
            <div className="flex min-w-0 items-start gap-3">
              <IconTile tone="brand" size="lg">
                <LayoutTemplate />
              </IconTile>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-[24px] font-extrabold text-v3-ink">{template.label}</h2>
                  {template.is_system ? (
                    <StatusPill tone="info" showDot={false}>
                      内置
                    </StatusPill>
                  ) : null}
                </div>
                <p className="mt-1 max-w-2xl text-[13px] text-v3-ink-2">{template.description}</p>
              </div>
            </div>
            <StatusPill tone={templateStatusTone(template)}>{templateStatusLabel(template)}</StatusPill>
          </div>
          <dl className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <DetailFact label="模板标识" value={template.type} monospace />
            <DetailFact label="默认角色" value={template.default_role} monospace />
            <DetailFact label="风险等级" value={templateRisk(template)} />
            <DetailFact label="状态" value={templateStatusLabel(template)} />
          </dl>
        </SoftCard>

        <WorkSurface>
          <div className="border-b border-v3-line px-5 py-4">
            <h3 className="text-[17px] font-bold text-v3-ink">模板能力</h3>
            <p className="mt-1 text-[13px] text-v3-ink-2">
              模板已定义的技能、MCP 与 Provider 能力，不代表默认全部启用。
            </p>
          </div>
          <div className="grid gap-4 p-5 md:grid-cols-3">
            <CapabilityBlock title="技能" values={capability.skills} />
            <CapabilityBlock title="MCP" values={capability.mcpServers} />
            <CapabilityBlock title="Provider" values={capability.providerTypes} />
          </div>
        </WorkSurface>

        <WorkSurface>
          <div className="border-b border-v3-line px-5 py-4">
            <h3 className="text-[17px] font-bold text-v3-ink">默认注入</h3>
            <p className="mt-1 text-[13px] text-v3-ink-2">
              创建时由模板带入的默认能力选择。
            </p>
          </div>
          <div className="grid gap-3 p-5 sm:grid-cols-2 xl:grid-cols-4">
            <InjectionCount label="技能" value={defaultInjection.skills.length} />
            <InjectionCount label="MCP" value={defaultInjection.mcpServers.length} />
            <InjectionCount label="Provider" value={defaultInjection.providerTypes.length} />
          </div>
        </WorkSurface>
      </div>

      <div className="flex min-w-0 flex-col gap-4">
        <SoftCard className="p-5">
          <div className="flex items-start gap-3">
            <IconTile tone="brand" size="sm">
              <CheckCircle2 />
            </IconTile>
            <div className="min-w-0">
              <h3 className="text-[17px] font-bold text-v3-ink">继承基线</h3>
              <p className="mt-1 text-[13px] text-v3-ink-2">
                模板默认值以此为准；创建时可能叠加团队治理基线。
              </p>
            </div>
          </div>
        </SoftCard>

        <SoftCard className="p-5">
          <h3 className="text-[17px] font-bold text-v3-ink">创建入口</h3>
          <p className="mt-1 text-[13px] text-v3-ink-2">
            使用此模板进入创建向导，模板只负责带入默认画像和能力建议。
          </p>
          <V3Button asChild className="mt-4 w-full">
            <Link to="/employees/new" search={{ template: template.type }}>
              用此模板创建数字员工
            </Link>
          </V3Button>
        </SoftCard>
      </div>
    </div>
  );
}

function DetailFact({
  label,
  monospace,
  value,
}: {
  label: string;
  monospace?: boolean;
  value: string;
}) {
  return (
    <div className="rounded-v3-inner bg-v3-card-soft p-3">
      <dt className="text-[12px] font-semibold text-v3-ink-3">{label}</dt>
      <dd className={monospace ? "mt-1 font-mono text-[13px] text-v3-ink" : "mt-1 text-[13px] text-v3-ink"}>
        {value}
      </dd>
    </div>
  );
}

function CapabilityBlock({ title, values }: { title: string; values: string[] }) {
  return (
    <div className="min-w-0 rounded-v3-inner bg-v3-card-soft p-3">
      <p className="text-[12px] font-semibold text-v3-ink-3">{title}</p>
      {values.length > 0 ? (
        <div className="mt-2 flex flex-wrap gap-2">
          {values.map((value) => (
            <code
              key={value}
              className="rounded-md bg-v3-card px-2 py-1 font-mono text-[12px] text-v3-ink"
            >
              {value}
            </code>
          ))}
        </div>
      ) : (
        <p className="mt-2 text-[13px] text-v3-ink-3">无</p>
      )}
    </div>
  );
}

function InjectionCount({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-v3-inner bg-v3-card-soft p-3">
      <p className="text-[12px] font-semibold text-v3-ink-3">{label}</p>
      <p className="mt-1 text-[18px] font-extrabold tabular-nums text-v3-ink">
        {label} {value}
      </p>
    </div>
  );
}

function useTemplateCatalog(apiBaseUrl: string, fetcher?: typeof fetch) {
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const queryClient = useQueryClient();
  const templatesQuery = useQuery({
    queryKey: ["employee-templates"],
    queryFn: () => listEmployeeTemplates(apiOptions),
  });

  return {
    apiOptions,
    error: templatesQuery.error,
    isLoading: templatesQuery.isLoading,
    templates: templatesQuery.data ?? [],
    refetch: async () => {
      await templatesQuery.refetch();
    },
    invalidate: () => queryClient.invalidateQueries({ queryKey: ["employee-templates"] }),
  };
}

type TemplateFormDialogProps = {
  open: boolean;
  mode: "create" | "edit";
  template?: EmployeeTemplate;
  apiOptions: { baseUrl: string; fetcher?: typeof fetch };
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
};

type TemplateFormDraft = {
  type: string;
  label: string;
  description: string;
  defaultRole: string;
  recommendedSkills: string;
  recommendedMcpServers: string;
  recommendedProviderTypes: string;
  defaultCapabilitySelection: string;
  defaultContextPolicyOverride: string;
  defaultApprovalPolicy: string;
};

function draftFromTemplate(template?: EmployeeTemplate): TemplateFormDraft {
  return {
    type: template?.type ?? "",
    label: template?.label ?? "",
    description: template?.description ?? "",
    defaultRole: template?.default_role ?? "",
    recommendedSkills: (template?.recommended_skills ?? []).join(", "),
    recommendedMcpServers: (template?.recommended_mcp_servers ?? []).join(", "),
    recommendedProviderTypes: (template?.recommended_provider_types ?? []).join(", "),
    defaultCapabilitySelection: JSON.stringify(template?.default_capability_selection ?? {}, null, 2),
    defaultContextPolicyOverride: JSON.stringify(template?.default_context_policy_override ?? {}, null, 2),
    defaultApprovalPolicy: JSON.stringify(template?.default_approval_policy ?? {}, null, 2),
  };
}

function parseCommaList(value: string): string[] {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function TemplateFormDialog({
  open,
  mode,
  template,
  apiOptions,
  onOpenChange,
  onSaved,
}: TemplateFormDialogProps) {
  const [draft, setDraft] = useState<TemplateFormDraft>(() => draftFromTemplate(template));
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setDraft(draftFromTemplate(template));
      setError("");
    }
  }, [open, template]);

  const mutation = useMutation({
    mutationFn: async () => {
      let capabilitySelection: Record<string, unknown>;
      let contextPolicyOverride: Record<string, unknown>;
      let approvalPolicy: Record<string, unknown>;
      try {
        capabilitySelection = JSON.parse(draft.defaultCapabilitySelection || "{}");
        contextPolicyOverride = JSON.parse(draft.defaultContextPolicyOverride || "{}");
        approvalPolicy = JSON.parse(draft.defaultApprovalPolicy || "{}");
      } catch {
        throw new Error("默认能力选择 / 上下文策略 / 审批策略必须是合法 JSON");
      }
      const payload = {
        label: draft.label.trim(),
        description: draft.description.trim(),
        default_role: draft.defaultRole.trim(),
        recommended_skills: parseCommaList(draft.recommendedSkills),
        recommended_mcp_servers: parseCommaList(draft.recommendedMcpServers),
        recommended_provider_types: parseCommaList(draft.recommendedProviderTypes),
        default_capability_selection: capabilitySelection,
        default_context_policy_override: contextPolicyOverride,
        default_approval_policy: approvalPolicy,
      };
      if (mode === "create") {
        await createEmployeeTemplate(apiOptions, { ...payload, type: draft.type.trim() });
      } else if (template) {
        await updateEmployeeTemplate(apiOptions, template.id, payload);
      }
    },
    onError: (mutationError: unknown) => {
      setError(mutationError instanceof Error ? mutationError.message : "保存失败");
    },
    onSuccess: () => {
      onSaved();
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] w-[90vw] max-w-[640px] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{mode === "create" ? "新建模板" : `配置模板：${template?.label ?? ""}`}</DialogTitle>
          <DialogDescription>
            模板用于在创建数字员工时带入默认角色、能力建议和治理策略默认值。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          {mode === "create" ? (
            <div className="grid gap-2">
              <Label>模板标识（type）</Label>
              <Input
                value={draft.type}
                onChange={(e) => setDraft((prev) => ({ ...prev, type: e.target.value }))}
                placeholder="例如 custom_reviewer"
              />
            </div>
          ) : null}
          <div className="grid gap-2">
            <Label>名称</Label>
            <Input
              value={draft.label}
              onChange={(e) => setDraft((prev) => ({ ...prev, label: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>描述</Label>
            <Textarea
              value={draft.description}
              onChange={(e) => setDraft((prev) => ({ ...prev, description: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认角色</Label>
            <Input
              value={draft.defaultRole}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultRole: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>推荐技能（逗号分隔）</Label>
            <Input
              value={draft.recommendedSkills}
              onChange={(e) => setDraft((prev) => ({ ...prev, recommendedSkills: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>推荐 MCP（逗号分隔）</Label>
            <Input
              value={draft.recommendedMcpServers}
              onChange={(e) => setDraft((prev) => ({ ...prev, recommendedMcpServers: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>推荐 Provider（逗号分隔）</Label>
            <Input
              value={draft.recommendedProviderTypes}
              onChange={(e) => setDraft((prev) => ({ ...prev, recommendedProviderTypes: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认能力选择（JSON）</Label>
            <Textarea
              className="font-mono text-xs"
              rows={4}
              value={draft.defaultCapabilitySelection}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultCapabilitySelection: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认上下文策略覆盖（JSON）</Label>
            <Textarea
              className="font-mono text-xs"
              rows={4}
              value={draft.defaultContextPolicyOverride}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultContextPolicyOverride: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认审批策略（JSON）</Label>
            <Textarea
              className="font-mono text-xs"
              rows={4}
              value={draft.defaultApprovalPolicy}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultApprovalPolicy: e.target.value }))}
            />
          </div>
          {error ? <p className="text-sm font-semibold text-destructive">{error}</p> : null}
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <V3Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </V3Button>
          <V3Button
            disabled={mutation.isPending || !draft.label.trim() || (mode === "create" && !draft.type.trim())}
            onClick={() => mutation.mutate()}
          >
            保存
          </V3Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
