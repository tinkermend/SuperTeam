import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Bot,
  CheckCircle2,
  LayoutTemplate,
  ShieldAlert,
  Sparkles,
} from "lucide-react";
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
  type V3Tone,
} from "@/components/superteam";
import {
  getDigitalEmployeeCreateOptions,
  type DigitalEmployeeCreateOptions,
  type DigitalEmployeeTypeOption,
} from "@/lib/api/employees";
import { listTeams, type TeamListItem } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  findTemplateByType,
  templateAvailabilityStatus,
  orderedEmployeeTypes,
  templateCapabilityPreview,
  templateCapabilitySummary,
  templateDefaultInjectionLine,
  templateDefaultInjectionSummary,
  templateRisk,
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
    () => orderedEmployeeTypes(state.options?.employee_types ?? []),
    [state.options?.employee_types],
  );

  return (
    <TemplateShell
      title="数字员工模板"
      subtitle="只读查看内置模板、能力默认值和团队继承基线"
      actions={
        <V3Button asChild>
          <Link to="/employees/new">
            <Bot className="size-4" />
            创建数字员工
          </Link>
        </V3Button>
      }
    >
      <TemplateQuerySurface state={state}>
        {templates.length === 0 ? (
          <SoftCard>
            <V3EmptyState title="暂无模板" description="当前 create-options 没有返回可用模板。" />
          </SoftCard>
        ) : (
          <WorkSurface>
            <div className="flex flex-col gap-1 border-b border-v3-line px-5 py-4 md:flex-row md:items-end md:justify-between">
              <div>
                <h2 className="text-[17px] font-bold text-v3-ink">内置模板目录</h2>
                <p className="mt-1 text-[13px] text-v3-ink-2">
                  数据来自数字员工创建选项，当前为只读目录。
                </p>
              </div>
              <p className="text-[13px] text-v3-ink-3">
                适用范围：{teamScopeLabel(state.activeTeam)}
              </p>
            </div>
            <V3Table>
              <thead>
                <tr>
                  <V3Th>模板</V3Th>
                  <V3Th>默认角色</V3Th>
                  <V3Th>模板能力</V3Th>
                  <V3Th>默认注入</V3Th>
                  <V3Th>适用范围</V3Th>
                  <V3Th>状态</V3Th>
                  <V3Th>操作</V3Th>
                </tr>
              </thead>
              <tbody>
                {templates.map((template) => (
                  <TemplateTableRow
                    key={template.type}
                    options={state.options}
                    scopeLabel={teamScopeLabel(state.activeTeam)}
                    template={template}
                  />
                ))}
              </tbody>
            </V3Table>
          </WorkSurface>
        )}
      </TemplateQuerySurface>
    </TemplateShell>
  );
}

export function TemplateDetailView({ apiBaseUrl, fetcher, templateType }: TemplateDetailViewProps) {
  const state = useTemplateCatalog(apiBaseUrl, fetcher);
  const template = findTemplateByType(state.options, templateType);

  return (
    <TemplateShell
      title="模板详情"
      subtitle="查看模板默认画像、注入策略和团队继承基线"
      back={<ShellPageHeaderBack ariaLabel="返回数字员工模板列表" to="/employees/templates" />}
    >
      <TemplateQuerySurface state={state}>
        {!template ? (
          <SoftCard>
            <V3EmptyState
              title="模板不存在"
              description="当前 create-options 中没有找到这个模板。"
              action={
                <V3Button asChild variant="outline">
                  <Link to="/employees/templates">返回模板管理</Link>
                </V3Button>
              }
            />
          </SoftCard>
        ) : (
          <TemplateDetailContent
            options={state.options}
            scopeLabel={teamScopeLabel(state.activeTeam)}
            template={template}
          />
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
  options,
  scopeLabel,
  template,
}: {
  options?: DigitalEmployeeCreateOptions;
  scopeLabel: string;
  template: DigitalEmployeeTypeOption;
}) {
  const availability = templateAvailabilityStatus(options);
  const tone: V3Tone = availability.label === "继承团队基线" ? "brand" : "ok";

  return (
    <V3Tr>
      <V3Td className="min-w-[240px]">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone="brand" size="sm">
            <Sparkles />
          </IconTile>
          <div className="min-w-0">
            <p className="font-bold text-v3-ink">{template.label}</p>
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
      <V3Td>{scopeLabel}</V3Td>
      <V3Td>
        <StatusPill tone={tone}>{availability.label}</StatusPill>
      </V3Td>
      <V3Td>
        <V3Button asChild variant="outline" size="sm">
          <Link
            aria-label={`查看${template.label}模板详情`}
            to="/employees/templates/$templateType"
            params={{ templateType: template.type }}
          >
            查看详情
          </Link>
        </V3Button>
      </V3Td>
    </V3Tr>
  );
}

function TemplateDetailContent({
  options,
  scopeLabel,
  template,
}: {
  options?: DigitalEmployeeCreateOptions;
  scopeLabel: string;
  template: DigitalEmployeeTypeOption;
}) {
  const capability = templateCapabilitySummary(template);
  const defaultInjection = templateDefaultInjectionSummary(template);
  const availability = templateAvailabilityStatus(options);
  const inherited = availability.label === "继承团队基线";

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
                <h2 className="text-[24px] font-extrabold text-v3-ink">{template.label}</h2>
                <p className="mt-1 max-w-2xl text-[13px] text-v3-ink-2">{template.description}</p>
              </div>
            </div>
            <StatusPill tone={inherited ? "brand" : "ok"}>{availability.label}</StatusPill>
          </div>
          <dl className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <DetailFact label="模板标识" value={template.type} monospace />
            <DetailFact label="默认角色" value={template.default_role} monospace />
            <DetailFact label="风险等级" value={templateRisk(template)} />
            <DetailFact label="适用范围" value={scopeLabel} />
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
            <IconTile tone={inherited ? "brand" : "ok"} size="sm">
              {inherited ? <ShieldAlert /> : <CheckCircle2 />}
            </IconTile>
            <div className="min-w-0">
              <h3 className="text-[17px] font-bold text-v3-ink">继承基线</h3>
              <p className="mt-1 text-[13px] text-v3-ink-2">
                模板默认值基于平台目录展示；创建时仍会叠加当前团队的只读继承基线。
              </p>
            </div>
          </div>
          {availability.notes.length > 0 ? (
            <ul className="mt-4 space-y-2">
              {availability.notes.map((note) => (
                <li
                  key={note}
                  className="rounded-v3-inner bg-v3-brand-soft px-3 py-2 text-[13px] font-semibold text-v3-brand"
                >
                  {note}
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-4 rounded-v3-inner bg-v3-ok-soft px-3 py-2 text-[13px] font-semibold text-v3-ok">
              当前模板直接使用平台默认值，不会被旧治理 allow-list 过滤。
            </p>
          )}
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
  const teams = useQuery({
    queryKey: ["teams", "digital-employee-template-catalog"],
    queryFn: () => listTeams(apiOptions),
  });
  const activeTeam = useMemo(
    () => teams.data?.find((team) => team.status === "active"),
    [teams.data],
  );
  const createOptions = useQuery({
    enabled: teams.isSuccess,
    queryKey: ["digital-employee-create-options", activeTeam?.id ?? "team-less"],
    queryFn: () => getDigitalEmployeeCreateOptions(apiOptions, activeTeam?.id),
  });

  return {
    activeTeam,
    error: teams.error ?? createOptions.error,
    isLoading: teams.isLoading || (teams.isSuccess && createOptions.isLoading),
    options: createOptions.data,
    refetch: async () => {
      if (teams.isError) {
        await teams.refetch();
        return;
      }
      if (teams.isSuccess) {
        await createOptions.refetch();
      }
    },
  };
}

function teamScopeLabel(team?: TeamListItem) {
  return team ? team.name : "未绑定团队";
}
