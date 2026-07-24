import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { X } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { listDigitalEmployees, type DigitalEmployee } from "@/lib/api/employees";
import { employeeStatusLabel } from "@/lib/status-labels";
import type { CreateTeamDraft } from "./create-team-draft";
import { Button } from "@/components/superteam";

type CreateTeamDigitalEmployeesStepProps = {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
};

export function CreateTeamDigitalEmployeesStep({
  apiBaseUrl,
  draft,
  fetcher,
  onChange
}: CreateTeamDigitalEmployeesStepProps) {
  const [query, setQuery] = useState("");
  const employeesQuery = useQuery({
    queryKey: ["unassigned-digital-employees"],
    queryFn: () =>
      listDigitalEmployees(
        {
          baseUrl: apiBaseUrl,
          fetcher
},
        {
          assignment: "unassigned"
},
      )
});

  const selectedIds = useMemo(
    () => new Set(draft.initial_digital_employees.map((emp) => emp.id)),
    [draft.initial_digital_employees],
  );

  const candidates = (employeesQuery.data ?? []).filter(
    (emp) => !selectedIds.has(emp.id) && emp.name.toLowerCase().includes(query.toLowerCase()),
  );

  function addEmployee(employee: DigitalEmployee) {
    if (selectedIds.has(employee.id)) return;
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
    <div className="flex flex-col gap-4">
      <div className="rounded-md border bg-muted/20 px-3 py-2 text-xs text-muted-foreground">
        创建时仅可分配尚未归属团队的独立数字员工。
      </div>

      <div className="grid gap-2">
        <Label htmlFor="team-digital-employee-search">搜索候选数字员工</Label>
        <Input
          aria-label="搜索候选数字员工"
          id="team-digital-employee-search"
          onChange={(event) => setQuery(event.target.value)}
          placeholder="按名称搜索后点击分配"
          type="search"
          value={query}
        />
        <div className="flex min-w-0 flex-col gap-1">
          {employeesQuery.isLoading ? (
            <p className="px-2 py-1.5 text-sm text-muted-foreground">加载中...</p>
          ) : employeesQuery.isError ? (
            <p className="px-2 py-1.5 text-sm text-destructive">
              加载失败
            </p>
          ) : candidates.length === 0 ? (
            <p className="px-2 py-1.5 text-sm text-muted-foreground">
              {query.trim() === ""
                ? "暂无可用未分配数字员工"
                : "没有匹配的候选数字员工"}
            </p>
          ) : (
            candidates.slice(0, 5).map((employee) => (
              <Button
                className="h-auto justify-start truncate px-3 py-2 text-left"
                key={employee.id}
                onClick={() => addEmployee(employee)}
                type="button"
                variant="ghost"
              >
                <div className="flex flex-col gap-0.5 truncate">
                  <div className="truncate text-sm font-medium">
                    {employee.name}
                  </div>
                  <div className="truncate text-xs text-muted-foreground">
                    状态: {employeeStatusLabel(employee.status)}
                  </div>
                </div>
              </Button>
            ))
          )}
        </div>
      </div>

      {draft.initial_digital_employees.length > 0 ? (
        <div className="grid gap-2 border-t pt-4">
          <Label>已选数字员工 ({draft.initial_digital_employees.length})</Label>
          <ul className="flex flex-col gap-1">
            {draft.initial_digital_employees.map((employee) => (
              <li
                className="flex items-center justify-between gap-3 rounded-lg border bg-card px-3 py-2 shadow-sm"
                key={employee.id}
              >
                <div className="flex min-w-0 flex-col truncate">
                  <div className="truncate text-sm font-medium">
                    {employee.name}
                  </div>
                </div>
                <Button
                  className="size-8 text-muted-foreground hover:text-destructive"
                  onClick={() => removeEmployee(employee.id)}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <span className="sr-only">移除</span>
                  <X className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}
