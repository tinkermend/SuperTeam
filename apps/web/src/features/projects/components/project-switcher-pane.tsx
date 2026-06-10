import { Archive, CircleDot, FolderKanban, Plus, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LiquidCard, StatusBadge } from "@/components/superteam";
import type { Project, ProjectStatus } from "@/lib/api/projects";
import { cn } from "@/lib/utils";

export type ProjectListFilters = {
  q: string;
  status: "all" | ProjectStatus;
};

type ProjectSwitcherPaneProps = {
  filters: ProjectListFilters;
  isFetching?: boolean;
  onCreateProject: () => void;
  onFiltersChange: (filters: ProjectListFilters) => void;
  onSelectProject: (projectId: string) => void;
  projects: Project[];
  selectedProjectId?: string;
};

const statusOptions: Array<{ label: string; value: "all" | ProjectStatus }> = [
  { label: "全部状态", value: "all" },
  { label: "运行中", value: "running" },
  { label: "配置中", value: "configuring" },
  { label: "验收中", value: "acceptance" },
  { label: "已暂停", value: "paused" },
  { label: "已归档", value: "archived" },
];

export function ProjectSwitcherPane({
  filters,
  isFetching,
  onCreateProject,
  onFiltersChange,
  onSelectProject,
  projects,
  selectedProjectId,
}: ProjectSwitcherPaneProps) {
  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div>
          <h2 className="text-base font-semibold">项目列表</h2>
          <p className="text-xs text-muted-foreground">
            {isFetching ? "正在刷新" : `${projects.length} 个项目`}
          </p>
        </div>
        <Button size="sm" type="button" onClick={onCreateProject}>
          <Plus data-icon="inline-start" />
          新建
        </Button>
      </div>

      <div className="grid gap-3 border-b p-4">
        <label className="relative text-sm">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="搜索项目"
            className="pl-9"
            placeholder="搜索项目、目标"
            value={filters.q}
            onChange={(event) =>
              onFiltersChange({ ...filters, q: event.target.value })
            }
          />
        </label>
        <Select
          value={filters.status}
          onValueChange={(value) =>
            onFiltersChange({
              ...filters,
              status: value as ProjectListFilters["status"],
            })
          }
        >
          <SelectTrigger aria-label="项目状态筛选" className="w-full">
            <SelectValue placeholder="项目状态" />
          </SelectTrigger>
          <SelectContent>
            {statusOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="max-h-[calc(100vh-320px)] min-h-[260px] overflow-y-auto p-2">
        {projects.length === 0 ? (
          <div className="flex h-44 flex-col items-center justify-center gap-2 px-4 text-center text-sm text-muted-foreground">
            <FolderKanban className="size-5" />
            没有符合筛选条件的项目
          </div>
        ) : (
          <div className="grid gap-2">
            {projects.map((project) => {
              const selected = project.id === selectedProjectId;
              return (
                <button
                  className={cn(
                    "w-full rounded-lg border bg-white/55 p-3 text-left transition hover:border-primary/40 hover:bg-primary/5",
                    selected && "border-primary/60 bg-primary/10",
                  )}
                  key={project.id}
                  type="button"
                  onClick={() => onSelectProject(project.id)}
                >
                  <div className="flex min-w-0 items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold">
                        {project.name}
                      </p>
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {project.goal}
                      </p>
                    </div>
                    <StatusBadge
                      tone={statusTone(project.status)}
                      className="shrink-0"
                    >
                      {statusLabel(project.status)}
                    </StatusBadge>
                  </div>
                  <div className="mt-3 flex items-center gap-3 text-xs text-muted-foreground">
                    {project.status === "archived" ? (
                      <Archive className="size-3.5" />
                    ) : (
                      <CircleDot className="size-3.5 text-[color:var(--superteam-success)]" />
                    )}
                    <span className="truncate">
                      {project.coordination_status || "registered"}
                    </span>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>
    </LiquidCard>
  );
}

export function statusLabel(status: ProjectStatus | string) {
  const labels: Record<string, string> = {
    acceptance: "验收中",
    archived: "已归档",
    configuring: "配置中",
    draft: "草稿",
    paused: "已暂停",
    running: "运行中",
  };
  return labels[status] ?? status;
}

export function statusTone(status: ProjectStatus | string) {
  if (status === "running") return "success";
  if (status === "archived") return "neutral";
  if (status === "paused" || status === "acceptance") return "warning";
  if (status === "configuring" || status === "draft") return "info";
  return "neutral";
}
