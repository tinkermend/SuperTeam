import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { V3Button, V3Segmented } from "@/components/superteam";
import {
  type AutomationCoordinationMode,
  type AutomationRule,
  type AutomationScheduleKind,
  type CreateAutomationRuleInput,
  type UpdateAutomationRuleInput,
} from "@/lib/api/automations";
import { listProjectMembers, listProjects } from "@/lib/api/projects";
import { HumanGateCallout } from "./human-gate-callout";

const MODE_OPTIONS: Array<{ label: string; value: AutomationCoordinationMode }> = [
  { label: "Plan", value: "plan" },
  { label: "Loop（推荐）", value: "loop" },
  { label: "对话", value: "chat" },
];

const SCHEDULE_OPTIONS: Array<{ label: string; value: AutomationScheduleKind }> = [
  { label: "Cron", value: "cron" },
  { label: "间隔", value: "interval" },
];

const CRON_PRESETS = [
  { label: "每天 09:00", value: "0 9 * * *" },
  { label: "每周一 09:00", value: "0 9 * * 1" },
  { label: "工作日 09:00", value: "0 9 * * 1-5" },
];

type AutomationRuleFormSheetProps = {
  apiBaseUrl: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editing?: AutomationRule | null;
  submitting?: boolean;
  error?: string | null;
  onCreate: (input: CreateAutomationRuleInput) => void;
  onUpdate: (ruleId: string, input: UpdateAutomationRuleInput) => void;
};

export function AutomationRuleFormSheet({
  apiBaseUrl,
  open,
  onOpenChange,
  editing,
  submitting,
  error,
  onCreate,
  onUpdate,
}: AutomationRuleFormSheetProps) {
  const isEdit = Boolean(editing);
  const [name, setName] = useState("");
  const [projectId, setProjectId] = useState("");
  const [mode, setMode] = useState<AutomationCoordinationMode>("loop");
  const [titleTemplate, setTitleTemplate] = useState("");
  const [bodyTemplate, setBodyTemplate] = useState("");
  const [employeeId, setEmployeeId] = useState("");
  const [chatObjective, setChatObjective] = useState("");
  const [scheduleKind, setScheduleKind] = useState<AutomationScheduleKind>("cron");
  const [cronExpr, setCronExpr] = useState("0 9 * * *");
  const [intervalHours, setIntervalHours] = useState("24");

  useEffect(() => {
    if (!open) return;
    if (editing) {
      setName(editing.name);
      setProjectId(editing.project_id);
      setMode(editing.coordination_mode);
      setTitleTemplate(editing.demand_title_template ?? "");
      setBodyTemplate(editing.demand_body_template ?? "");
      setEmployeeId(editing.digital_employee_id ?? "");
      setChatObjective(editing.chat_objective_template ?? "");
      setScheduleKind(editing.schedule_kind);
      setCronExpr(editing.cron_expr ?? "0 9 * * *");
      setIntervalHours(
        editing.interval_seconds
          ? String(Math.max(1, Math.round(editing.interval_seconds / 3600)))
          : "24",
      );
      return;
    }
    setName("");
    setProjectId("");
    setMode("loop");
    setTitleTemplate("");
    setBodyTemplate("");
    setEmployeeId("");
    setChatObjective("");
    setScheduleKind("cron");
    setCronExpr("0 9 * * *");
    setIntervalHours("24");
  }, [editing, open]);

  const projectsQuery = useQuery({
    queryKey: ["automation-form-projects", apiBaseUrl],
    queryFn: () => listProjects({ baseUrl: apiBaseUrl }),
    enabled: open && !isEdit,
  });

  const anchorProjectId = isEdit ? (editing?.project_id ?? "") : projectId;

  const employeesQuery = useQuery({
    queryKey: ["automation-form-employees", apiBaseUrl, anchorProjectId],
    queryFn: () => listProjectMembers({ baseUrl: apiBaseUrl }, anchorProjectId),
    enabled: open && mode === "chat" && Boolean(anchorProjectId),
  });

  const projectList = projectsQuery.data ?? [];

  const employees = useMemo(() => {
    const members = employeesQuery.data ?? [];
    return members
      .filter((member) => member.principal_type === "digital_employee")
      .map((member) => ({
        id: member.principal_id,
        name: member.display_name_snapshot?.trim() || member.principal_id,
      }));
  }, [employeesQuery.data]);

  function handleSubmit() {
    const intervalSeconds = Math.max(60, Number(intervalHours || "0") * 3600);
    if (isEdit && editing) {
      onUpdate(editing.id, {
        name: name.trim(),
        demand_title_template: mode === "chat" ? null : titleTemplate,
        demand_body_template: mode === "chat" ? null : bodyTemplate,
        digital_employee_id: mode === "chat" ? employeeId || null : null,
        chat_objective_template: mode === "chat" ? chatObjective : null,
        schedule_kind: scheduleKind,
        cron_expr: scheduleKind === "cron" ? cronExpr : null,
        interval_seconds: scheduleKind === "interval" ? intervalSeconds : null,
        timezone: "Asia/Shanghai",
      });
      return;
    }
    onCreate({
      name: name.trim(),
      project_id: projectId,
      coordination_mode: mode,
      demand_title_template: mode === "chat" ? undefined : titleTemplate,
      demand_body_template: mode === "chat" ? undefined : bodyTemplate,
      digital_employee_id: mode === "chat" ? employeeId : undefined,
      chat_objective_template: mode === "chat" ? chatObjective : undefined,
      schedule_kind: scheduleKind,
      cron_expr: scheduleKind === "cron" ? cronExpr : undefined,
      interval_seconds: scheduleKind === "interval" ? intervalSeconds : undefined,
      timezone: "Asia/Shanghai",
      enabled: true,
    });
  }

  const canSubmit =
    name.trim().length > 0 &&
    (isEdit || projectId) &&
    (mode === "chat"
      ? Boolean(employeeId && chatObjective.trim())
      : Boolean(titleTemplate.trim() && bodyTemplate.trim())) &&
    (scheduleKind === "cron" ? Boolean(cronExpr.trim()) : Number(intervalHours) > 0);

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle>{isEdit ? "编辑定时规则" : "新建定时规则"}</SheetTitle>
          <SheetDescription>
            到点后走任务中枢同一条发起链路。发起人固定为当前账号；离开项目后规则将自动停用。
          </SheetDescription>
        </SheetHeader>

        <div className="flex flex-1 flex-col gap-4 px-4 py-4">
          <div className="space-y-2">
            <Label htmlFor="automation-name">规则名称</Label>
            <Input
              id="automation-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：每周回归巡检"
            />
          </div>

          {!isEdit ? (
            <div className="space-y-2">
              <Label>项目</Label>
              <Select value={projectId || undefined} onValueChange={setProjectId}>
                <SelectTrigger>
                  <SelectValue placeholder="选择有发起权的项目" />
                </SelectTrigger>
                <SelectContent>
                  {projectList.map((project) => (
                    <SelectItem key={project.id} value={project.id}>
                      {project.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {projectList.length === 0 && !projectsQuery.isLoading ? (
                <p className="text-sm text-muted-foreground">
                  没有可选项目。请先加入或创建项目后再配置定时规则。
                </p>
              ) : null}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">
              项目与模式创建后不可更改。如需更换，请停用本规则并新建。
            </p>
          )}

          {!isEdit ? (
            <div className="space-y-2">
              <Label>模式</Label>
              <V3Segmented
                aria-label="协调模式"
                options={MODE_OPTIONS}
                value={mode}
                onChange={setMode}
              />
            </div>
          ) : null}

          <HumanGateCallout mode={mode} />

          {mode === "chat" ? (
            <>
              <div className="space-y-2">
                <Label>数字员工</Label>
                <Select value={employeeId || undefined} onValueChange={setEmployeeId}>
                  <SelectTrigger>
                    <SelectValue placeholder="选择对话员工" />
                  </SelectTrigger>
                  <SelectContent>
                    {employees.map((employee) => (
                      <SelectItem key={employee.id} value={employee.id}>
                        {employee.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-2">
                <Label htmlFor="chat-objective">对话目标模板</Label>
                <Textarea
                  id="chat-objective"
                  value={chatObjective}
                  onChange={(e) => setChatObjective(e.target.value)}
                  rows={4}
                  placeholder="可用变量 {{date}} {{datetime}} {{rule_name}} {{project_name}}"
                />
              </div>
            </>
          ) : (
            <>
              <div className="space-y-2">
                <Label htmlFor="demand-title">需求标题模板</Label>
                <Input
                  id="demand-title"
                  value={titleTemplate}
                  onChange={(e) => setTitleTemplate(e.target.value)}
                  placeholder="例如：{{date}} 例行巡检"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="demand-body">需求正文模板</Label>
                <Textarea
                  id="demand-body"
                  value={bodyTemplate}
                  onChange={(e) => setBodyTemplate(e.target.value)}
                  rows={5}
                  placeholder="可用变量 {{date}} {{datetime}} {{rule_name}} {{project_name}}"
                />
              </div>
            </>
          )}

          <div className="space-y-2">
            <Label>日程</Label>
            <V3Segmented
              aria-label="日程种类"
              options={SCHEDULE_OPTIONS}
              value={scheduleKind}
              onChange={setScheduleKind}
            />
          </div>

          {scheduleKind === "cron" ? (
            <div className="space-y-2">
              <div className="flex flex-wrap gap-2">
                {CRON_PRESETS.map((preset) => (
                  <V3Button
                    key={preset.value}
                    size="sm"
                    type="button"
                    variant={cronExpr === preset.value ? "primary" : "secondary"}
                    onClick={() => setCronExpr(preset.value)}
                  >
                    {preset.label}
                  </V3Button>
                ))}
              </div>
              <Input
                value={cronExpr}
                onChange={(e) => setCronExpr(e.target.value)}
                placeholder="cron 表达式"
                aria-label="cron 表达式"
              />
              <p className="text-xs text-muted-foreground">时区 Asia/Shanghai</p>
            </div>
          ) : (
            <div className="space-y-2">
              <Label htmlFor="interval-hours">间隔（小时）</Label>
              <Input
                id="interval-hours"
                type="number"
                min={1}
                value={intervalHours}
                onChange={(e) => setIntervalHours(e.target.value)}
              />
            </div>
          )}

          <p className="text-xs text-muted-foreground">
            重叠策略：上一次触发对应的需求/对话尚未结束时，跳过本次。
          </p>

          {error ? (
            <p className="text-sm text-destructive" role="alert">
              {error}
            </p>
          ) : null}
        </div>

        <SheetFooter className="gap-2 border-t p-4">
          <V3Button variant="secondary" onClick={() => onOpenChange(false)}>
            取消
          </V3Button>
          <V3Button disabled={!canSubmit || submitting} onClick={handleSubmit}>
            {isEdit ? "保存" : "创建并启用"}
          </V3Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
