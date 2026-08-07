import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, CheckCircle2, Loader2, Users } from "lucide-react";
import { IconTile, StatusPill, Button } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { getDigitalEmployeeOverview } from "@/lib/api/employees";
import {
  installSkill,
  type InstallSkillResult,
  type Skill,
  type SkillInstallTargetScope
} from "@/lib/api/skills";
import { listTeams } from "@/lib/api/teams";
import { cn } from "@/lib/utils";

type SkillInstallDialogProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  skill?: Skill | null;
};

type ApiOpts = { baseUrl: string; fetcher?: typeof fetch };

const targetScopeOptions: Array<{
  description: string;
  icon: typeof Users;
  label: string;
  value: SkillInstallTargetScope;
}> = [
  {
    description: "绑定到团队,团队内数字员工全部继承",
    icon: Users,
    label: "团队",
    value: "team"
},
  {
    description: "绑定到单个数字员工",
    icon: Bot,
    label: "数字员工",
    value: "employee"
},
];

export function SkillInstallDialog({
  apiBaseUrl,
  fetcher,
  onOpenChange,
  open,
  skill
}: SkillInstallDialogProps) {
  const queryClient = useQueryClient();
  const [targetScope, setTargetScope] = useState<SkillInstallTargetScope>("employee");
  const [selectedTeamId, setSelectedTeamId] = useState("");
  const [selectedEmployeeId, setSelectedEmployeeId] = useState("");
  const [installResult, setInstallResult] = useState<InstallSkillResult | null>(null);
  const apiOptions: ApiOpts = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);

  const teams = useQuery({
    enabled: open && targetScope === "team",
    queryKey: ["teams"],
    queryFn: () => listTeams(apiOptions)
});
  const employees = useQuery({
    enabled: open && targetScope === "employee",
    queryKey: ["digital-employees-overview", "skill-install-dialog"],
    queryFn: async () => {
      const overview = await getDigitalEmployeeOverview(apiOptions, { limit: 100 });
      return overview.items;
    }
});

  useEffect(() => {
    if (open) {
      setInstallResult(null);
      setSelectedTeamId("");
      setSelectedEmployeeId("");
      setTargetScope("employee");
    }
  }, [open, skill?.id]);

  const selectedTargetId = targetScope === "team" ? selectedTeamId : selectedEmployeeId;
  const targetLoadError =
    targetScope === "team" && teams.error instanceof Error
      ? teams.error.message
      : targetScope === "employee" && employees.error instanceof Error
        ? employees.error.message
        : undefined;
  const mutation = useMutation({
    mutationFn: () => {
      if (!skill) {
        throw new Error("缺少技能信息");
      }
      return installSkill(apiOptions, skill.id, {
        target_scope: targetScope,
        ...(targetScope === "team" ? { team_id: selectedTeamId } : { digital_employee_id: selectedEmployeeId })
});
    },
    onSuccess: async (result) => {
      setInstallResult(result);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["skills"] }),
        queryClient.invalidateQueries({ queryKey: ["skill", skill?.id] }),
      ]);
    }
});

  const mutationError = mutation.error instanceof Error ? mutation.error.message : undefined;
  const canSubmit = Boolean(skill && selectedTargetId) && !mutation.isPending;

  const resetDialogState = () => {
    setInstallResult(null);
    setSelectedTeamId("");
    setSelectedEmployeeId("");
    setTargetScope("employee");
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      resetDialogState();
    }
    onOpenChange(nextOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="gap-0 overflow-hidden rounded-[var(--radius-card)] border-line bg-card p-0 text-ink shadow-pop sm:max-w-xl">
        <DialogHeader className="border-b border-line px-5 py-4 text-left">
          <div className="flex min-w-0 items-start gap-3">
            <IconTile tone="artifact">
              <Bot />
            </IconTile>
            <div className="min-w-0">
              <DialogTitle className="text-xl font-bold text-ink">绑定技能</DialogTitle>
              <DialogDescription className="mt-1 text-sm leading-5 text-ink-2">
                {skill?.name ?? "选择技能"} · {skill?.version ?? "待选择"}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="space-y-5 px-5 py-5">
          <fieldset className="space-y-3">
            <legend className="text-sm font-bold text-ink">绑定范围</legend>
            <RadioGroup
              className="grid gap-3 sm:grid-cols-2"
              onValueChange={(value) => {
                setTargetScope(value as SkillInstallTargetScope);
                setInstallResult(null);
              }}
              value={targetScope}
            >
              {targetScopeOptions.map((option) => {
                const Icon = option.icon;
                return (
                  <label
                    className={cn(
                      "flex cursor-pointer items-start gap-3 rounded-2xl border border-line-strong bg-card-soft p-3 transition-colors",
                      targetScope === option.value && "border-brand bg-brand-soft/60",
                    )}
                    key={option.value}
                  >
                    <RadioGroupItem className="mt-1 border-line-strong text-brand" value={option.value} />
                    <span className="flex min-w-0 gap-2">
                      <Icon className="mt-0.5 size-4 shrink-0 text-ink-3" />
                      <span className="min-w-0">
                        <span className="block text-sm font-bold text-ink">{option.label}</span>
                        <span className="mt-0.5 block text-xs leading-5 text-ink-2">{option.description}</span>
                      </span>
                    </span>
                  </label>
                );
              })}
            </RadioGroup>
          </fieldset>

          <div className="space-y-2">
            <label className="text-sm font-bold text-ink" htmlFor="skill-install-target">
              绑定目标
            </label>
            {targetScope === "team" ? (
              <Select
                disabled={teams.isPending || teams.isError}
                onValueChange={(value) => {
                  setSelectedTeamId(value);
                  setInstallResult(null);
                }}
                value={selectedTeamId}
              >
                <SelectTrigger
                  aria-label="绑定到团队"
                  className="h-11 w-full rounded-xl border-line-strong bg-card text-ink shadow-none"
                  id="skill-install-target"
                >
                  <SelectValue placeholder={teams.isPending ? "加载团队…" : "选择团队"} />
                </SelectTrigger>
                <SelectContent>
                  {(teams.data ?? []).map((team) => (
                    <SelectItem key={team.id} value={team.id}>
                      {team.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <Select
                disabled={employees.isPending || employees.isError}
                onValueChange={(value) => {
                  setSelectedEmployeeId(value);
                  setInstallResult(null);
                }}
                value={selectedEmployeeId}
              >
                <SelectTrigger
                  aria-label="绑定到数字员工"
                  className="h-11 w-full rounded-xl border-line-strong bg-card text-ink shadow-none"
                  id="skill-install-target"
                >
                  <SelectValue placeholder={employees.isPending ? "加载数字员工…" : "选择数字员工"} />
                </SelectTrigger>
                <SelectContent>
                  {(employees.data ?? []).map((employee) => (
                    <SelectItem key={employee.identity_summary.id} value={employee.identity_summary.id}>
                      {employee.identity_summary.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <p className="text-xs leading-5 text-ink-3">
              绑定即时生效（逻辑授权），不依赖任何运行节点；技能文件会在目标员工下次任务派发时自动同步到运行环境。
            </p>
          </div>

          {targetLoadError ? (
            <div className="rounded-xl border border-danger/30 bg-danger-soft px-3 py-2 text-sm font-semibold text-danger">
              {targetLoadError}
            </div>
          ) : null}
          {mutationError ? (
            <div className="rounded-xl border border-danger/30 bg-danger-soft px-3 py-2 text-sm font-semibold text-danger">
              {mutationError}
            </div>
          ) : null}
          {installResult ? (
            <div className="flex items-center gap-2 rounded-xl border border-ok/30 bg-ok-soft px-3 py-2 text-sm font-semibold text-ok">
              <CheckCircle2 className="size-4" />
              <span>
                {installResult.already_bound
                  ? "该目标已拥有此技能（含团队继承），无需重复绑定"
                  : "已绑定，下次任务派发时同步到运行环境"}
              </span>
            </div>
          ) : null}
        </div>

        <DialogFooter className="border-t border-line bg-card-soft px-5 py-4">
          {installResult ? (
            <StatusPill tone="ok">{installResult.already_bound ? "已具备" : "已绑定"}</StatusPill>
          ) : null}
          <Button
            disabled={mutation.isPending}
            onClick={() => handleOpenChange(false)}
            type="button"
            variant="outline"
          >
            取消
          </Button>
          <Button disabled={!canSubmit} onClick={() => mutation.mutate()} type="button">
            {mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            确认绑定
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
