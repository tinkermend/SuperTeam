import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Bot,
  Network,
  Plus,
  Search,
  Sparkles,
  UserRoundCheck,
  X
} from "lucide-react";
import {
  GlassCard,
  IconTile,
  SoftCard,
  StatusPill,
  UserIdentity,
  UserSearchSelect,
  Pagination,
  WorkSurface,
  type Tone,
  Button
} from "@/components/superteam";
import { TeamIconPicker } from "@/components/superteam/team-icon-picker";
import { TeamIconTile } from "@/components/superteam/team-icon-tile";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";
import type { UserSummary } from "@/lib/api/auth";
import {
  listDigitalEmployees,
  type DigitalEmployee,
  type DigitalEmployeeStatus
} from "@/lib/api/employees";
import { cn } from "@/lib/utils";
import { employeeStatusLabel } from "@/lib/status-labels";
import {
  type CreateTeamDraft,
  inferTeamDisplay,
  slugify
} from "./create-team-draft";

const THEME_COLORS = [
  { value: "blue", class: "bg-blue-500", label: "蓝" },
  { value: "cyan", class: "bg-cyan-500", label: "青" },
  { value: "neutral", class: "bg-slate-500", label: "灰" },
  { value: "teal", class: "bg-teal-500", label: "青绿" },
  { value: "violet", class: "bg-violet-500", label: "紫" },
] as const;

const EMPLOYEE_STATUS_TONE: Record<DigitalEmployeeStatus, Tone> = {
  draft: "mute",
  ready: "ok",
  active: "info",
  disabled: "mute",
  error: "danger"
};

const EMPLOYEE_LIBRARY_PAGE_SIZE = 5;

type CreateTeamConfigurationCanvasProps = {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  errors: Record<string, string>;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
};

/**
 * 新建团队的配置画布：左侧建立身份与人类责任，中央呈现组织关系，右侧从员工库挑选执行者。
 * 人类负责人和数字员工保持两种独立对象，不把人类职责伪装成数字员工能力。
 */
export function CreateTeamConfigurationCanvas({
  apiBaseUrl,
  draft,
  errors,
  fetcher,
  onChange
}: CreateTeamConfigurationCanvasProps) {
  const [employeeQuery, setEmployeeQuery] = useState("");
  const [employeePage, setEmployeePage] = useState(1);
  const employeesQuery = useQuery({
    queryKey: ["unassigned-digital-employees"],
    queryFn: () =>
      listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }, { assignment: "unassigned" })
});

  const selectedEmployeeIds = useMemo(
    () => new Set(draft.initial_digital_employees.map((employee) => employee.id)),
    [draft.initial_digital_employees],
  );
  const normalizedEmployeeQuery = employeeQuery.trim().toLowerCase();
  const candidateEmployees = (employeesQuery.data ?? []).filter((employee) => {
    if (selectedEmployeeIds.has(employee.id)) return false;
    if (!normalizedEmployeeQuery) return true;
    return [employee.name, employee.role]
      .filter(Boolean)
      .some((value) => value.toLowerCase().includes(normalizedEmployeeQuery));
  });
  const employeePageCount = Math.max(
    1,
    Math.ceil(candidateEmployees.length / EMPLOYEE_LIBRARY_PAGE_SIZE),
  );
  const currentEmployeePage = Math.min(employeePage, employeePageCount);
  const visibleEmployeeCandidates = candidateEmployees.slice(
    (currentEmployeePage - 1) * EMPLOYEE_LIBRARY_PAGE_SIZE,
    currentEmployeePage * EMPLOYEE_LIBRARY_PAGE_SIZE,
  );

  function updateEmployeeQuery(query: string) {
    setEmployeeQuery(query);
    setEmployeePage(1);
  }

  function updateName(name: string) {
    onChange({
      ...draft,
      display: draft.displayTouched
        ? draft.display
        : inferTeamDisplay(`${name} ${draft.slug}`),
      name,
      slug: draft.slugTouched ? draft.slug : slugify(name)
});
  }

  function updateSlug(rawSlug: string) {
    const slug = rawSlug.toLowerCase().replace(/[^a-z0-9-]/g, "");
    onChange({
      ...draft,
      display: draft.displayTouched
        ? draft.display
        : inferTeamDisplay(`${draft.name} ${slug}`),
      slug,
      slugTouched: true
});
  }

  function updateDescription(description: string) {
    onChange({ ...draft, description });
  }

  function updateIcon(iconKey: string) {
    onChange({
      ...draft,
      display: { ...draft.display, icon_key: iconKey },
      displayTouched: true
});
  }

  function updateColor(color: CreateTeamDraft["display"]["color_tone"]) {
    onChange({
      ...draft,
      display: { ...draft.display, color_tone: color },
      displayTouched: true
});
  }

  function addOwner(owner: UserSummary) {
    if (draft.owners.some((candidate) => candidate.id === owner.id)) return;
    onChange({ ...draft, owners: [...draft.owners, owner] });
  }

  function removeOwner(ownerId: string) {
    onChange({ ...draft, owners: draft.owners.filter((owner) => owner.id !== ownerId) });
  }

  function addEmployee(employee: DigitalEmployee) {
    if (selectedEmployeeIds.has(employee.id)) return;
    onChange({
      ...draft,
      initial_digital_employees: [...draft.initial_digital_employees, employee]
});
  }

  function removeEmployee(employeeId: string) {
    onChange({
      ...draft,
      initial_digital_employees: draft.initial_digital_employees.filter(
        (employee) => employee.id !== employeeId,
      )
});
  }

  return (
    <div className="grid min-w-0 items-start gap-4 xl:grid-cols-[minmax(15rem,0.82fr)_minmax(28rem,1.42fr)_minmax(17rem,0.9fr)]">
      <div className="flex min-w-0 flex-col gap-4">
        <GlassCard className="p-4 sm:p-5">
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <IconTile tone="brand">
                <Sparkles />
              </IconTile>
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-ink">团队名片</h2>
                <p className="mt-0.5 text-xs text-ink-3">定义团队的展示身份</p>
              </div>
            </div>
            <TeamIconTile
              className="size-10 rounded-[12px] [&_svg]:size-5"
              metadata={{ display: draft.display }}
            />
          </div>

          <div className="glass-inner mt-4 grid gap-4 p-3.5">
            <div className="grid gap-1.5">
              <Label htmlFor="team-name">团队名称</Label>
              <Input
                className="bg-card/90"
                id="team-name"
                onChange={(event) => updateName(event.target.value)}
                placeholder="例如：安全响应组"
                value={draft.name}
              />
              {errors.name ? (
                <span className="text-xs text-destructive">{errors.name}</span>
              ) : (
                <span className="text-[11px] text-ink-3">用于全站展示，可随时修改。</span>
              )}
            </div>

            <div className="grid gap-1.5">
              <Label className="text-xs font-medium text-ink-3" htmlFor="team-slug">
                团队标识 slug
              </Label>
              <div className="flex items-center gap-2 rounded-[12px] border border-line bg-card/90 px-2.5 py-1">
                <span className="select-none font-mono text-xs text-ink-3">/teams/</span>
                <Input
                  className="h-8 border-0 bg-transparent px-0 font-mono text-sm shadow-none focus-visible:ring-0"
                  id="team-slug"
                  onChange={(event) => updateSlug(event.target.value)}
                  placeholder="team-slug"
                  value={draft.slug}
                />
              </div>
              {errors.slug ? (
                <span className="text-xs text-destructive">{errors.slug}</span>
              ) : (
                <span className="text-[11px] text-ink-3">默认按名称生成，可手动修改。</span>
              )}
            </div>

            <div className="grid gap-1.5">
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="team-description">团队说明</Label>
                <span className="text-[11px] tabular-nums text-ink-3">
                  {draft.description.length}/280
                </span>
              </div>
              <Textarea
                className="min-h-20 resize-y bg-card/90 text-sm"
                id="team-description"
                maxLength={280}
                onChange={(event) => updateDescription(event.target.value)}
                placeholder="简要说明团队负责的领域、服务对象或协作边界。"
                rows={3}
                value={draft.description}
              />
              {errors.description ? (
                <span className="text-xs text-destructive">{errors.description}</span>
              ) : (
                <span className="text-[11px] text-ink-3">用于团队目录展示，并可通过接口维护。</span>
              )}
            </div>

            <div className="grid gap-2">
              <Label>团队主题色</Label>
              <div className="flex flex-wrap items-center gap-2.5">
                {THEME_COLORS.map((color) => {
                  const isSelected = draft.display.color_tone === color.value;
                  return (
                    <button
                      aria-label={`主题色 ${color.label}`}
                      aria-pressed={isSelected}
                      className={cn(
                        "flex size-7 items-center justify-center rounded-[9px] transition",
                        color.class,
                        isSelected
                          ? "ring-2 ring-brand ring-offset-2 ring-offset-card"
                          : "opacity-75 hover:opacity-100",
                      )}
                      key={color.value}
                      onClick={() => updateColor(color.value)}
                      type="button"
                    >
                      {isSelected ? <span className="size-1.5 rounded-full bg-white" /> : null}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="grid gap-2">
              <Label>团队图标</Label>
              <TeamIconPicker
                colorTone={draft.display.color_tone}
                onSelect={updateIcon}
                value={draft.display.icon_key}
              />
            </div>
          </div>
        </GlassCard>

        <SoftCard className="p-4 sm:p-5">
          <div className="flex items-start gap-3">
            <IconTile tone="info" size="sm">
              <UserRoundCheck />
            </IconTile>
            <div className="min-w-0">
              <h2 className="text-sm font-semibold text-ink">人类负责人</h2>
              <p className="mt-0.5 text-xs text-ink-3">
                负责人负责团队配置、审批与业务判断。
              </p>
            </div>
          </div>

          <div className="mt-4 flex flex-col gap-2.5">
            <UserSearchSelect
              apiBaseUrl={apiBaseUrl}
              excludedUserIds={draft.owners.map((owner) => owner.id)}
              fetcher={fetcher}
              inputLabel="搜索平台注册用户"
              onSelect={addOwner}
              placeholder="搜索平台注册用户"
            />
            {errors.owner ? <span className="text-xs text-destructive">{errors.owner}</span> : null}
          </div>

          {draft.owners.length > 0 ? (
            <ul className="mt-3 flex flex-col gap-2 border-t border-line pt-3">
              {draft.owners.map((owner) => (
                <li
                  className="flex items-center justify-between gap-2 rounded-inner border border-brand/20 bg-brand-soft/40 px-3 py-2"
                  key={owner.id}
                >
                  <UserIdentity className="min-w-0" showSecondary size="sm" user={owner} />
                  <Button
                    aria-label={`移除负责人 ${owner.username}`}
                    className="size-8 text-ink-3 hover:text-destructive"
                    onClick={() => removeOwner(owner.id)}
                    size="icon"
                    type="button"
                    variant="ghost"
                  >
                    <X className="size-4" />
                  </Button>
                </li>
              ))}
            </ul>
          ) : null}
        </SoftCard>
      </div>

      <TeamCanvas
        draft={draft}
        onRemoveEmployee={removeEmployee}
        onRemoveOwner={removeOwner}
      />

      <EmployeeLibrary
        candidates={visibleEmployeeCandidates}
        employeeQuery={employeeQuery}
        isError={employeesQuery.isError}
        isLoading={employeesQuery.isLoading}
        onAdd={addEmployee}
        onPageChange={setEmployeePage}
        onQueryChange={updateEmployeeQuery}
        page={currentEmployeePage}
        pageCount={employeePageCount}
        total={candidateEmployees.length}
      />
    </div>
  );
}

function TeamCanvas({
  draft,
  onRemoveEmployee,
  onRemoveOwner
}: {
  draft: CreateTeamDraft;
  onRemoveEmployee: (employeeId: string) => void;
  onRemoveOwner: (ownerId: string) => void;
}) {
  const hasStructure = draft.owners.length > 0 && draft.initial_digital_employees.length > 0;

  return (
    <section
      aria-label="团队画布"
      className="relative min-h-[42rem] overflow-hidden rounded-card border border-line bg-card shadow-card"
    >
      <div className="relative z-10 flex items-start justify-between gap-3 border-b border-line px-5 py-4 sm:px-6">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="brand" size="sm">
            <Network />
          </IconTile>
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-ink">团队画布</h2>
            <p className="mt-0.5 text-xs text-ink-3">
              人类负责人在上方，数字员工在下方。
            </p>
          </div>
        </div>
        <span className="shrink-0 rounded-lg bg-card-soft px-2.5 py-1 text-xs font-medium text-ink-2">
          {draft.owners.length + draft.initial_digital_employees.length} 位成员
        </span>
      </div>

      <div className="relative min-h-[35rem] px-5 pb-5 pt-6 sm:px-6">
        {hasStructure ? <CanvasConnections /> : null}

        <div className="relative z-10 mx-auto flex max-w-sm flex-col gap-2">
          <p className="text-center text-[11px] font-semibold tracking-wide text-ink-3">人类负责人</p>
          {draft.owners.length === 0 ? (
            <CanvasEmptyCard label="从左侧选择负责人" />
          ) : (
            draft.owners.map((owner) => (
              <article
                className="group flex items-center justify-between gap-3 rounded-inner border border-brand/30 bg-card px-3 py-3 shadow-sm"
                key={owner.id}
              >
                <UserIdentity className="min-w-0" showSecondary user={owner} />
                <Button
                  aria-label={`从画布移除负责人 ${owner.username}`}
                  className="size-8 text-ink-3 opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100 focus-visible:opacity-100"
                  onClick={() => onRemoveOwner(owner.id)}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <X className="size-4" />
                </Button>
              </article>
            ))
          )}
        </div>

        <div className="relative z-10 mt-14">
          <div className="flex items-center justify-between gap-3">
            <p className="text-[11px] font-semibold tracking-wide text-ink-3">数字员工</p>
            <span className="text-xs text-ink-3">{draft.initial_digital_employees.length} 位已加入</span>
          </div>

          {draft.initial_digital_employees.length === 0 ? (
            <div className="mt-3">
              <CanvasEmptyCard label="从右侧员工库添加数字员工" />
            </div>
          ) : (
            <div className="mt-3 grid gap-3 sm:grid-cols-2 2xl:grid-cols-3">
              {draft.initial_digital_employees.map((employee) => (
                <article
                  className="group flex min-w-0 flex-col gap-3 rounded-inner border border-line bg-card p-3 shadow-sm transition-colors hover:border-line-strong hover:bg-card-soft"
                  key={employee.id}
                >
                  <div className="flex min-w-0 items-start justify-between gap-2">
                    <EmployeeAvatar
                      asset={employeeAvatarAsset(employee)}
                      name={employee.name}
                      size="sm"
                    />
                    <Button
                      aria-label={`移除数字员工 ${employee.name}`}
                      className="size-7 text-ink-3 opacity-0 transition-opacity hover:text-destructive group-hover:opacity-100 focus-visible:opacity-100"
                      onClick={() => onRemoveEmployee(employee.id)}
                      size="icon"
                      type="button"
                      variant="ghost"
                    >
                      <X className="size-3.5" />
                    </Button>
                  </div>
                  <div className="min-w-0">
                    <h3 className="truncate text-sm font-semibold text-ink">{employee.name}</h3>
                    <p className="mt-0.5 line-clamp-2 text-xs leading-5 text-ink-3">{employee.role}</p>
                  </div>
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-3">
                    <Bot className="size-3.5" />
                    <span>数字员工</span>
                  </div>
                </article>
              ))}
            </div>
          )}
        </div>
      </div>
    </section>
  );
}

function CanvasConnections() {
  return (
    <svg
      aria-hidden="true"
      className="pointer-events-none absolute inset-x-10 top-[8.5rem] z-0 h-36 w-[calc(100%-5rem)] text-brand/30"
      fill="none"
      preserveAspectRatio="none"
      viewBox="0 0 100 100"
    >
      <path d="M50 2V38M16 75V58H84V75M50 38V58" stroke="currentColor" strokeWidth="0.65" />
      <circle cx="50" cy="38" fill="currentColor" r="1.5" />
      <circle cx="16" cy="75" fill="currentColor" r="1.3" />
      <circle cx="50" cy="75" fill="currentColor" r="1.3" />
      <circle cx="84" cy="75" fill="currentColor" r="1.3" />
    </svg>
  );
}

function CanvasEmptyCard({ label }: { label: string }) {
  return (
    <div className="flex min-h-20 items-center justify-center rounded-inner border border-dashed border-line-strong bg-card-soft/60 px-4 text-center text-xs text-ink-3">
      {label}
    </div>
  );
}

function EmployeeLibrary({
  candidates,
  employeeQuery,
  isError,
  isLoading,
  onAdd,
  onPageChange,
  onQueryChange,
  page,
  pageCount,
  total
}: {
  candidates: DigitalEmployee[];
  employeeQuery: string;
  isError: boolean;
  isLoading: boolean;
  onAdd: (employee: DigitalEmployee) => void;
  onPageChange: (page: number) => void;
  onQueryChange: (query: string) => void;
  page: number;
  pageCount: number;
  total: number;
}) {
  return (
    <WorkSurface className="flex h-[42rem] min-w-0 flex-col">
      <div className="border-b border-line px-4 py-4 sm:px-5">
        <div className="flex items-start gap-3">
          <IconTile tone="artifact" size="sm">
            <Bot />
          </IconTile>
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-ink">数字员工库</h2>
            <p className="mt-0.5 text-xs text-ink-3">仅展示未归属团队的数字员工。</p>
          </div>
        </div>
        <div className="relative mt-4">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-3" />
          <Input
            aria-label="搜索候选数字员工"
            className="bg-card pl-9"
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="搜索名称或角色"
            type="search"
            value={employeeQuery}
          />
        </div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto p-3 sm:p-4">
        {isLoading ? (
          <p className="py-8 text-center text-sm text-ink-3">加载中...</p>
        ) : isError ? (
          <p className="py-8 text-center text-sm text-destructive">加载失败</p>
        ) : total === 0 ? (
          <p className="py-8 text-center text-sm text-ink-3">
            {employeeQuery.trim() === "" ? "当前暂无未归属团队的数字员工" : "没有匹配的数字员工"}
          </p>
        ) : (
          candidates.map((employee) => {
            const tone = EMPLOYEE_STATUS_TONE[employee.status] ?? "mute";
            return (
              <article
                className="flex min-w-0 items-center gap-3 rounded-inner border border-line bg-card px-3 py-3 transition-colors hover:bg-card-soft"
                key={employee.id}
              >
                <EmployeeAvatar
                  asset={employeeAvatarAsset(employee)}
                  name={employee.name}
                  size="sm"
                />
                <div className="min-w-0 flex-1">
                  <h3 className="truncate text-sm font-medium text-ink">{employee.name}</h3>
                  <p className="mt-0.5 truncate text-xs text-ink-3">{employee.role}</p>
                  <StatusPill className="mt-2 px-2 py-0.5 text-[10px]" tone={tone}>
                    {employeeStatusLabel(employee.status)}
                  </StatusPill>
                </div>
                <Button
                  aria-label={`加入 ${employee.name}`}
                  className="size-8 border border-line bg-card text-brand shadow-sm hover:border-brand hover:bg-brand-soft"
                  onClick={() => onAdd(employee)}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <Plus className="size-4" />
                </Button>
              </article>
            );
          })
        )}
      </div>
      {total > EMPLOYEE_LIBRARY_PAGE_SIZE ? (
        <Pagination
          className="shrink-0"
          onPageChange={onPageChange}
          page={page}
          pageCount={pageCount}
          pageSize={EMPLOYEE_LIBRARY_PAGE_SIZE}
          total={total}
        />
      ) : null}
    </WorkSurface>
  );
}
