import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import { UserIdentity } from "@/components/superteam/user-identity";
import { StatusPill, type Tone , Button} from "@/components/superteam";
import { Input } from "@/components/ui/input";
import {
  listDigitalEmployees,
  type DigitalEmployee,
  type DigitalEmployeeStatus
} from "@/lib/api/employees";
import type { UserSummary } from "@/lib/api/auth";
import { TeamIconTile } from "@/components/superteam/team-icon-tile";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";
import { employeeStatusLabel } from "@/lib/status-labels";
import type { CreateTeamDraft } from "./create-team-draft";

const EMPLOYEE_STATUS_TONE: Record<DigitalEmployeeStatus, Tone> = {
  draft: "mute",
  ready: "ok",
  active: "info",
  disabled: "mute",
  error: "danger"
};

export function CreateTeamStepMembers({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
  errors
}: {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
  errors: Record<string, string>;
}) {
  const [employeeQuery, setEmployeeQuery] = useState("");

  const employeesQuery = useQuery({
    queryKey: ["unassigned-digital-employees"],
    queryFn: () =>
      listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }, { assignment: "unassigned" })
});

  const selectedEmployeeIds = useMemo(
    () => new Set(draft.initial_digital_employees.map((emp) => emp.id)),
    [draft.initial_digital_employees],
  );

  const candidateEmployees = (employeesQuery.data ?? []).filter(
    (emp) =>
      !selectedEmployeeIds.has(emp.id) &&
      emp.name.toLowerCase().includes(employeeQuery.toLowerCase()),
  );

  function addOwner(owner: UserSummary) {
    if (draft.owners.some((o) => o.id === owner.id)) return;
    onChange({ ...draft, owners: [...draft.owners, owner] });
  }

  function removeOwner(ownerId: string) {
    onChange({ ...draft, owners: draft.owners.filter((o) => o.id !== ownerId) });
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
        (emp) => emp.id !== employeeId,
      )
});
  }

  return (
    <div className="grid grid-cols-1 items-start gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
      <div className="flex min-w-0 flex-col gap-4">
        <section className="flex flex-col gap-3 rounded-[22px] border border-line bg-card p-5 shadow-card">
          <div>
            <h2 className="text-sm font-semibold text-ink">团队负责人</h2>
            <p className="mt-1 text-xs text-ink-3">
              从平台注册用户中选择，负责团队配置与后续审批。
            </p>
          </div>
          <div className="flex flex-col gap-2">
            <UserSearchSelect
              apiBaseUrl={apiBaseUrl}
              excludedUserIds={draft.owners.map((o) => o.id)}
              fetcher={fetcher}
              onSelect={addOwner}
              placeholder="搜索平台注册用户"
            />
            {errors.owner ? (
              <span className="text-sm text-destructive">{errors.owner}</span>
            ) : null}
          </div>
        </section>

        <section className="flex flex-col gap-3 rounded-[22px] border border-line bg-card p-5 shadow-card">
          <div>
            <div className="flex items-center justify-between gap-3">
              <h2 className="text-sm font-semibold text-ink">可加入的数字员工</h2>
              <span className="text-xs text-ink-3">
                已选 {selectedEmployeeIds.size} 位
              </span>
            </div>
            <p className="mt-1 text-xs text-ink-3">
              仅显示未归属团队的数字员工。
            </p>
          </div>

          <Input
            aria-label="搜索候选数字员工"
            onChange={(e) => setEmployeeQuery(e.target.value)}
            placeholder="搜索数字员工名称、角色或技能"
            type="search"
            value={employeeQuery}
          />

          <div className="flex max-h-72 flex-col gap-2 overflow-y-auto pr-1">
            {employeesQuery.isLoading ? (
              <p className="py-4 text-center text-sm text-ink-3">加载中...</p>
            ) : employeesQuery.isError ? (
              <p className="py-4 text-center text-sm text-destructive">加载失败</p>
            ) : candidateEmployees.length === 0 ? (
              <p className="py-6 text-center text-sm text-ink-3">
                {employeeQuery.trim() === ""
                  ? "当前暂无未归属团队的数字员工"
                  : "没有匹配的数字员工"}
              </p>
            ) : (
              candidateEmployees.map((employee) => {
                const tone = EMPLOYEE_STATUS_TONE[employee.status] ?? "mute";
                return (
                  <div
                    className="flex items-center gap-3 rounded-[12px] border border-line bg-card px-3 py-2 hover:bg-card-soft"
                    key={employee.id}
                  >
                    <EmployeeAvatar
                      asset={employeeAvatarAsset(employee)}
                      name={employee.name}
                      size="sm"
                    />
                    <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <div className="truncate text-sm font-medium text-ink">
                        {employee.name}
                      </div>
                      <div className="truncate text-xs text-ink-3">{employee.role}</div>
                    </div>
                    <StatusPill className="shrink-0" tone={tone}>
                      {employeeStatusLabel(employee.status)}
                    </StatusPill>
                    <Button
                      aria-label={`加入 ${employee.name}`}
                      className="shrink-0"
                      onClick={() => addEmployee(employee)}
                      size="sm"
                      variant="ghost"
                    >
                      <Plus className="size-4 text-ink-3 hover:text-brand" />
                    </Button>
                  </div>
                );
              })
            )}
          </div>
        </section>
      </div>

      <aside className="flex min-w-0 flex-col gap-4 rounded-[22px] border border-line bg-card p-5 shadow-card lg:sticky lg:top-4">
        <div className="flex items-center gap-3 border-b border-line pb-4">
          <TeamIconTile
            className="size-11 shrink-0 rounded-[12px]"
            metadata={{ display: draft.display }}
          />
          <div className="min-w-0">
            <div className="truncate text-sm font-bold text-ink">
              {draft.name.trim() || "未命名团队"}
            </div>
            <div className="truncate font-mono text-[11px] text-ink-3">
              /teams/{draft.slug.trim() || "team-slug"}
            </div>
          </div>
        </div>

        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold text-ink">本团队人员预设</h2>
          <span className="text-xs text-ink-3">
            {draft.owners.length + draft.initial_digital_employees.length} 位
          </span>
        </div>

        <div className="flex flex-col gap-2">
          <h3 className="text-xs font-semibold text-ink-3">
            负责人 ({draft.owners.length})
          </h3>
          {draft.owners.length === 0 ? (
            <p className="py-2 text-center text-xs text-ink-3">未选择负责人</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {draft.owners.map((owner) => (
                <li
                  className="flex items-center justify-between gap-3 rounded-[12px] border border-line px-3 py-2"
                  key={owner.id}
                >
                  <UserIdentity showSecondary size="sm" user={owner} />
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
          )}
        </div>

        <div className="flex flex-col gap-2 border-t border-line pt-4">
          <h3 className="text-xs font-semibold text-ink-3">
            数字员工 ({draft.initial_digital_employees.length})
          </h3>
          {draft.initial_digital_employees.length === 0 ? (
            <p className="py-2 text-center text-xs text-ink-3">暂未分配数字员工</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {draft.initial_digital_employees.map((employee) => (
                <li
                  className="flex items-center justify-between gap-3 rounded-[12px] border border-line px-3 py-2"
                  key={employee.id}
                >
                  <div className="flex min-w-0 items-center gap-3">
                    <EmployeeAvatar
                      asset={employeeAvatarAsset(employee)}
                      name={employee.name}
                      size="sm"
                    />
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium text-ink">
                        {employee.name}
                      </div>
                      <div className="truncate text-xs text-ink-3">{employee.role}</div>
                    </div>
                  </div>
                  <Button
                    aria-label={`移除数字员工 ${employee.name}`}
                    className="size-8 shrink-0 text-ink-3 hover:text-destructive"
                    onClick={() => removeEmployee(employee.id)}
                    size="icon"
                    type="button"
                    variant="ghost"
                  >
                    <X className="size-4" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </aside>
    </div>
  );
}
