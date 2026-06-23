import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, CheckCircle2, Loader2, Users } from "lucide-react";
import { IconTile, StatusPill, V3Button } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { listDigitalEmployees } from "@/lib/api/employees";
import {
  InstallSkillError,
  installSkill,
  type InstallSkillResult,
  type Skill,
  type SkillInstallBlockedTarget,
  type SkillInstallTargetScope,
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
    description: "安装到团队内可执行的数字员工",
    icon: Users,
    label: "团队",
    value: "team",
  },
  {
    description: "安装到单个数字员工运行环境",
    icon: Bot,
    label: "数字员工",
    value: "employee",
  },
];

export function SkillInstallDialog({
  apiBaseUrl,
  fetcher,
  onOpenChange,
  open,
  skill,
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
    queryFn: () => listTeams(apiOptions),
  });
  const employees = useQuery({
    enabled: open && targetScope === "employee",
    queryKey: ["digital-employees"],
    queryFn: () => listDigitalEmployees(apiOptions),
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
        ...(targetScope === "team" ? { team_id: selectedTeamId } : { digital_employee_id: selectedEmployeeId }),
        timeout_sec: 15,
      });
    },
    onSuccess: async (result) => {
      setInstallResult(result);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["skills"] }),
        queryClient.invalidateQueries({ queryKey: ["skill", skill?.id] }),
      ]);
    },
  });

  const installError = mutation.error instanceof InstallSkillError ? mutation.error : undefined;
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
      <DialogContent className="gap-0 overflow-hidden rounded-[var(--v3-r-card)] border-v3-line bg-v3-card p-0 text-v3-ink shadow-v3-pop sm:max-w-xl">
        <DialogHeader className="border-b border-v3-line px-5 py-4 text-left">
          <div className="flex min-w-0 items-start gap-3">
            <IconTile tone="artifact">
              <Bot />
            </IconTile>
            <div className="min-w-0">
              <DialogTitle className="text-xl font-bold text-v3-ink">安装技能</DialogTitle>
              <DialogDescription className="mt-1 text-sm leading-5 text-v3-ink-2">
                {skill?.name ?? "选择技能"} · {skill?.version ?? "待选择"}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <div className="space-y-5 px-5 py-5">
          <fieldset className="space-y-3">
            <legend className="text-sm font-bold text-v3-ink">安装范围</legend>
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
                      "flex cursor-pointer items-start gap-3 rounded-2xl border border-v3-line-strong bg-v3-card-soft p-3 transition-colors",
                      targetScope === option.value && "border-v3-brand bg-v3-brand-soft/60",
                    )}
                    key={option.value}
                  >
                    <RadioGroupItem className="mt-1 border-v3-line-strong text-v3-brand" value={option.value} />
                    <span className="flex min-w-0 gap-2">
                      <Icon className="mt-0.5 size-4 shrink-0 text-v3-ink-3" />
                      <span className="min-w-0">
                        <span className="block text-sm font-bold text-v3-ink">{option.label}</span>
                        <span className="mt-0.5 block text-xs leading-5 text-v3-ink-2">{option.description}</span>
                      </span>
                    </span>
                  </label>
                );
              })}
            </RadioGroup>
          </fieldset>

          <div className="space-y-2">
            <label className="text-sm font-bold text-v3-ink" htmlFor="skill-install-target">
              安装目标
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
                  aria-label="安装团队"
                  className="h-11 w-full rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none"
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
                  aria-label="安装数字员工"
                  className="h-11 w-full rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none"
                  id="skill-install-target"
                >
                  <SelectValue placeholder={employees.isPending ? "加载数字员工…" : "选择数字员工"} />
                </SelectTrigger>
                <SelectContent>
                  {(employees.data ?? []).map((employee) => (
                    <SelectItem key={employee.id} value={employee.id}>
                      {employee.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
            <p className="text-xs leading-5 text-v3-ink-3">
              安装请求会等待运行节点确认，超时时间 15 秒。
            </p>
          </div>

          {targetLoadError ? (
            <div className="rounded-xl border border-v3-danger/30 bg-v3-danger-soft px-3 py-2 text-sm font-semibold text-v3-danger">
              {targetLoadError}
            </div>
          ) : null}
          {mutationError ? (
            <div className="space-y-3 rounded-xl border border-v3-danger/30 bg-v3-danger-soft px-3 py-2 text-sm text-v3-danger">
              <div className="font-semibold">{mutationError}</div>
              {installError?.phase ? (
                <div className="font-mono text-[11px] uppercase tracking-normal text-v3-danger/80">
                  {installError.phase}
                </div>
              ) : null}
              {installError?.blockedTargets.length ? (
                <BlockedTargetList blockedTargets={installError.blockedTargets} />
              ) : null}
            </div>
          ) : null}
          {installResult ? (
            <div className="flex items-center gap-2 rounded-xl border border-v3-ok/30 bg-v3-ok-soft px-3 py-2 text-sm font-semibold text-v3-ok">
              <CheckCircle2 className="size-4" />
              <span>已安装到 {installResult.installed_count} 个目标</span>
            </div>
          ) : null}
        </div>

        <DialogFooter className="border-t border-v3-line bg-v3-card-soft px-5 py-4">
          {installResult ? (
            <StatusPill tone="ok">安装完成</StatusPill>
          ) : null}
          <V3Button
            disabled={mutation.isPending}
            onClick={() => handleOpenChange(false)}
            type="button"
            variant="outline"
          >
            取消
          </V3Button>
          <V3Button disabled={!canSubmit} onClick={() => mutation.mutate()} type="button">
            {mutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            确认安装
          </V3Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function BlockedTargetList({ blockedTargets }: { blockedTargets: SkillInstallBlockedTarget[] }) {
  return (
    <div className="space-y-2">
      {blockedTargets.map((target, index) => (
        <div
          className="rounded-lg border border-v3-danger/20 bg-v3-card px-3 py-2 text-v3-ink"
          data-testid="skill-install-blocked-target"
          key={`${target.digital_employee_id ?? target.node_id ?? target.reason_code}-${index}`}
        >
          <div className="flex min-w-0 items-start justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate text-sm font-bold text-v3-ink">
                {target.employee_name || target.digital_employee_id || "未知目标"}
              </div>
              <div className="mt-1 text-xs leading-5 text-v3-ink-2">{target.message}</div>
            </div>
            <span className="shrink-0 rounded-lg bg-v3-danger-soft px-2 py-1 font-mono text-[11px] font-bold text-v3-danger">
              {target.reason_code}
            </span>
          </div>
          {providerNodeLabel(target) ? (
            <div className="mt-2 font-mono text-[11px] text-v3-ink-3">{providerNodeLabel(target)}</div>
          ) : null}
        </div>
      ))}
    </div>
  );
}

function providerNodeLabel(target: SkillInstallBlockedTarget) {
  const parts = [target.provider_type, target.node_id ?? target.runtime_node_id].filter(Boolean);
  return parts.length ? parts.join(" · ") : undefined;
}
