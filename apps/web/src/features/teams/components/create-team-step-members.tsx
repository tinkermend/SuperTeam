import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Plus, X } from "lucide-react";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import { UserIdentity } from "@/components/superteam/user-identity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { listDigitalEmployees, type DigitalEmployee } from "@/lib/api/employees";
import type { UserSummary } from "@/lib/api/auth";
import { TeamIconTile } from "@/components/superteam/team-icon-tile";
import type { CreateTeamDraft } from "./create-team-draft";

export function CreateTeamStepMembers({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
  errors,
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
      listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }, { assignment: "unassigned" }),
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
      initial_digital_employees: [...draft.initial_digital_employees, employee],
    });
  }

  function removeEmployee(employeeId: string) {
    onChange({
      ...draft,
      initial_digital_employees: draft.initial_digital_employees.filter((emp) => emp.id !== employeeId),
    });
  }

  return (
    <div className="flex flex-col gap-6">
      {/* Top summary bar */}
      <div className="flex items-center gap-4 rounded-xl border bg-card p-4 shadow-sm">
        <TeamIconTile
          className="size-12 rounded-xl"
          metadata={{ display: draft.display }}
        />
        <div className="min-w-0">
          <div className="truncate text-base font-bold">
            {draft.name || "未命名团队"}
          </div>
          <div className="truncate font-mono text-xs text-muted-foreground">
            {draft.slug || "team-slug"}
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
        {/* Left Pane: Selection */}
        <div className="flex min-w-0 flex-col gap-6">
          <section className="flex flex-col gap-4 rounded-xl border bg-card p-5 shadow-sm">
            <div>
              <h2 className="text-sm font-semibold">团队负责人</h2>
              <p className="mt-1 text-xs text-muted-foreground">
                从平台注册用户中选择，负责团队配置与后续审批。
              </p>
            </div>
            
            <div className="flex flex-col gap-2">
              <UserSearchSelect
                apiBaseUrl={apiBaseUrl}
                excludedUserIds={draft.owners.map((o) => o.id)}
                fetcher={fetcher}
                onSelect={(user) => {
                  addOwner(user);
                }}
                placeholder="搜索平台注册用户"
              />
              {errors.owner ? (
                <span className="text-sm text-destructive">{errors.owner}</span>
              ) : null}
            </div>
          </section>

          <section className="flex flex-col gap-4 rounded-xl border bg-card p-5 shadow-sm">
            <div>
              <div className="flex items-center justify-between">
                <h2 className="text-sm font-semibold">可加入的数字员工</h2>
                <span className="text-xs text-muted-foreground">
                  已过滤 {selectedEmployeeIds.size} 位已加入员工
                </span>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                仅显示未归属团队的数字员工；已归属其他团队的员工不会出现在候选列表。
              </p>
            </div>

            <div className="flex flex-col gap-3">
              <Input
                aria-label="搜索候选数字员工"
                onChange={(e) => setEmployeeQuery(e.target.value)}
                placeholder="搜索数字员工名称、角色或技能"
                type="search"
                value={employeeQuery}
              />
              
              <div className="flex flex-col gap-2">
                {employeesQuery.isLoading ? (
                  <p className="py-4 text-center text-sm text-muted-foreground">加载中...</p>
                ) : employeesQuery.isError ? (
                  <p className="py-4 text-center text-sm text-destructive">加载失败</p>
                ) : candidateEmployees.length === 0 ? (
                  <p className="py-8 text-center text-sm text-muted-foreground">
                    {employeeQuery.trim() === ""
                      ? "当前系统暂无未归属团队的数字员工"
                      : "没有匹配的数字员工"}
                  </p>
                ) : (
                  candidateEmployees.map((employee) => (
                    <div
                      className="flex items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2 hover:bg-muted/30"
                      key={employee.id}
                    >
                      <div className="flex min-w-0 flex-col">
                        <div className="truncate text-sm font-medium">{employee.name}</div>
                        <div className="truncate text-xs text-muted-foreground">状态: {employee.status}</div>
                      </div>
                      <Button
                        onClick={() => addEmployee(employee)}
                        size="sm"
                        variant="ghost"
                      >
                        <Plus className="size-4 text-muted-foreground hover:text-primary" />
                      </Button>
                    </div>
                  ))
                )}
              </div>
            </div>
          </section>
        </div>

        {/* Right Pane: Selected List */}
        <div className="flex min-w-0 flex-col gap-4 rounded-xl border bg-card p-5 shadow-sm">
          <div className="flex items-center justify-between border-b pb-4">
            <h2 className="text-sm font-semibold">本团队人员预设</h2>
            <span className="text-xs text-muted-foreground">
              已选择 {draft.owners.length + draft.initial_digital_employees.length} 位
            </span>
          </div>

          <div className="flex flex-col gap-4">
            {/* Owners */}
            <div className="flex flex-col gap-2">
              <h3 className="text-xs font-semibold text-muted-foreground">负责人 ({draft.owners.length})</h3>
              {draft.owners.length === 0 ? (
                <p className="py-2 text-center text-xs text-muted-foreground">未选择负责人</p>
              ) : (
                <ul className="flex flex-col gap-2">
                  {draft.owners.map((owner) => (
                    <li
                      className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2 shadow-sm"
                      key={owner.id}
                    >
                      <UserIdentity showSecondary size="sm" user={owner} />
                      <Button
                        className="size-8 text-muted-foreground hover:text-destructive"
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

            {/* Digital Employees */}
            <div className="flex flex-col gap-2 border-t pt-4">
              <h3 className="text-xs font-semibold text-muted-foreground">
                数字员工 ({draft.initial_digital_employees.length})
              </h3>
              {draft.initial_digital_employees.length === 0 ? (
                <p className="py-2 text-center text-xs text-muted-foreground">暂未分配数字员工</p>
              ) : (
                <ul className="flex flex-col gap-2">
                  {draft.initial_digital_employees.map((employee) => (
                    <li
                      className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2 shadow-sm"
                      key={employee.id}
                    >
                      <div className="flex min-w-0 flex-col truncate">
                        <div className="truncate text-sm font-medium">{employee.name}</div>
                      </div>
                      <Button
                        className="size-8 text-muted-foreground hover:text-destructive"
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
          </div>
        </div>
      </div>
    </div>
  );
}
