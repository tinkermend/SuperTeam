import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Search, SlidersHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusPill, WorkSurface } from "@/components/superteam";
import { listDigitalEmployees, type DigitalEmployee } from "@/lib/api/employees";
import { cn } from "@/lib/utils";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectDigitalEmployeesStepProps = {
  apiBaseUrl: string;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
};

type FilterMode = "all" | "schedulable" | "needsConfig";

export function ProjectDigitalEmployeesStep({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
}: ProjectDigitalEmployeesStepProps) {
  const [query, setQuery] = useState("");
  const [filterMode, setFilterMode] = useState<FilterMode>("all");
  const employeesQuery = useQuery({
    enabled: Boolean(draft.teamId),
    queryKey: ["project-create", "digital-employees", draft.teamId],
    queryFn: () => listDigitalEmployees({ baseUrl: apiBaseUrl, fetcher }, { team_id: draft.teamId }),
  });
  const selectedIds = useMemo(() => new Set(draft.selectedDigitalEmployees.map((employee) => employee.id)), [draft.selectedDigitalEmployees]);
  const employees = (employeesQuery.data ?? []).filter((employee) => {
    const textMatch = `${employee.name} ${employee.role}`.toLowerCase().includes(query.trim().toLowerCase());
    const modeMatch =
      filterMode === "all" ||
      (filterMode === "schedulable" && employee.status === "active") ||
      (filterMode === "needsConfig" && employee.status !== "active");
    return textMatch && modeMatch;
  });

  function toggleEmployee(employee: DigitalEmployee) {
    if (selectedIds.has(employee.id)) {
      onChange({
        ...draft,
        selectedDigitalEmployees: draft.selectedDigitalEmployees.filter((item) => item.id !== employee.id),
      });
      return;
    }
    onChange({ ...draft, selectedDigitalEmployees: [...draft.selectedDigitalEmployees, employee] });
  }

  return (
    <div className="grid gap-5">
      <div>
        <h3 className="text-xl font-semibold text-v3-ink">选择数字员工池</h3>
        <p className="mt-1 text-sm text-v3-ink-2">仅从项目数字员工池中选取执行员工；人类负责人不归入数字员工。</p>
      </div>

      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-[260px] flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-v3-ink-3" />
          <Input
            aria-label="搜索数字员工"
            className="pl-9"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索数字员工"
            type="search"
            value={query}
          />
        </div>
        <FilterButton active={filterMode === "all"} onClick={() => setFilterMode("all")}>全部</FilterButton>
        <FilterButton active={filterMode === "schedulable"} onClick={() => setFilterMode("schedulable")}>可调度</FilterButton>
        <FilterButton active={filterMode === "needsConfig"} onClick={() => setFilterMode("needsConfig")}>需配置</FilterButton>
        <Button aria-label="筛选设置" className="size-10 rounded-xl" size="icon" type="button" variant="outline">
          <SlidersHorizontal className="size-4" />
        </Button>
      </div>

      <WorkSurface>
        <div className="grid grid-cols-[3rem_minmax(12rem,1fr)_1fr_7rem] border-b border-v3-line bg-v3-card-soft px-4 py-3 text-xs font-semibold text-v3-ink-2">
          <span />
          <span>数字员工</span>
          <span>能力标签</span>
          <span>状态</span>
        </div>
        {employeesQuery.isLoading ? (
          <p className="px-4 py-6 text-sm text-v3-ink-2">正在加载数字员工...</p>
        ) : employeesQuery.isError ? (
          <p className="px-4 py-6 text-sm text-v3-danger">数字员工加载失败</p>
        ) : employees.length === 0 ? (
          <p className="px-4 py-6 text-sm text-v3-ink-2">{draft.teamId ? "暂无匹配数字员工" : "请先选择授权团队"}</p>
        ) : (
          employees.map((employee) => {
            const selected = selectedIds.has(employee.id);
            return (
              <div
                aria-pressed={selected}
                className="grid w-full cursor-pointer grid-cols-[3rem_minmax(12rem,1fr)_1fr_7rem] items-center border-b border-v3-line px-4 py-3 text-left last:border-b-0 hover:bg-v3-card-soft focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-v3-brand/10"
                key={employee.id}
                onClick={() => toggleEmployee(employee)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    toggleEmployee(employee);
                  }
                }}
                role="button"
                tabIndex={0}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    "grid size-5 place-items-center rounded-md border",
                    selected ? "border-v3-brand bg-v3-brand-soft text-v3-ok" : "border-v3-line-strong bg-v3-card text-v3-ink-3",
                  )}
                >
                  {selected ? <Check className="size-3.5" /> : null}
                </span>
                <div className="min-w-0">
                  <p className="truncate text-sm font-semibold text-v3-ink">{employee.name}</p>
                  <p className="truncate text-xs text-v3-ink-3">{employee.role}</p>
                </div>
                <div className="flex min-w-0 flex-wrap gap-1.5">
                  {employeeCapabilities(employee).map((capability) => (
                    <span className="rounded-lg bg-v3-mute-soft px-2 py-1 text-xs font-medium text-v3-mute" key={capability}>
                      {capability}
                    </span>
                  ))}
                </div>
                <StatusPill tone={employee.status === "active" ? "ok" : "warn"}>
                  {employee.status === "active" ? "可调度" : "待配置"}
                </StatusPill>
              </div>
            );
          })
        )}
      </WorkSurface>

      <div className="rounded-xl border border-v3-warn/20 bg-v3-warn-soft px-3 py-3 text-sm text-v3-warn">
        只选择可参与该项目的执行员工。创建项目不会立即发起任务，也不会自动调度数字员工。
      </div>
    </div>
  );
}

function FilterButton({
  active,
  children,
  onClick,
}: {
  active: boolean;
  children: ReactNode;
  onClick: () => void;
}) {
  return (
    <Button className={cn("rounded-xl", active && "border-v3-brand bg-v3-brand-soft text-v3-brand")} onClick={onClick} type="button" variant="outline">
      {children}
    </Button>
  );
}

function employeeCapabilities(employee: DigitalEmployee) {
  const metadata = employee.metadata ?? {};
  const label = typeof metadata.effective_config_label === "string" ? metadata.effective_config_label : undefined;
  return [employee.employee_type, label, employee.risk_level].filter(Boolean).slice(0, 3);
}
