import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check } from "lucide-react";
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
import { SoftCard, V3Button, V3Segmented } from "@/components/superteam";
import {
  type AutomationCoordinationMode,
  type AutomationRule,
  type AutomationScheduleKind,
  type CreateAutomationRuleInput,
  type UpdateAutomationRuleInput,
} from "@/lib/api/automations";
import { listProjectMembers, listProjects } from "@/lib/api/projects";
import { cn } from "@/lib/utils";
import type { AutomationRuleDraft } from "../scenario-templates";
import { HumanGateCallout } from "./human-gate-callout";

const MODE_CARDS: Array<{
  value: AutomationCoordinationMode;
  title: string;
  badge?: string;
  blurb: string;
}> = [
  {
    value: "loop",
    title: "循环",
    badge: "推荐",
    blurb: "到点开跑，中间尽量自治，终点仍等人验收。",
  },
  {
    value: "plan",
    title: "计划",
    blurb: "到点生成待确认计划，半自动立项。",
  },
  {
    value: "chat",
    title: "对话",
    blurb: "定时对员工发起对话，不进项目验收。",
  },
];

const SCHEDULE_OPTIONS: Array<{ label: string; value: AutomationScheduleKind }> = [
  { label: "按日历", value: "cron" },
  { label: "按间隔", value: "interval" },
];

const CRON_PRESETS = [
  { label: "每天 09:00", value: "0 9 * * *" },
  { label: "每周一 09:00", value: "0 9 * * 1" },
  { label: "工作日 09:00", value: "0 9 * * 1-5" },
];

const GATE_SUMMARY: Record<AutomationCoordinationMode, string[]> = {
  loop: ["到点自动发起需求", "执行中缺口仍可能升级等人", "终态验收需人类处理（Console / 飞书）"],
  plan: ["到点生成计划", "通常需计划确认后才派发", "终态验收需人类处理"],
  chat: ["到点发起员工对话", "不进入项目协调与验收", "若要闭环需另转任务或配置 Demand 规则"],
};

type AutomationRuleFormSheetProps = {
  apiBaseUrl: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  editing?: AutomationRule | null;
  draft?: AutomationRuleDraft | null;
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
  draft,
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
    setName(draft?.name ?? "");
    setProjectId(draft?.project_id ?? "");
    setMode(draft?.coordination_mode ?? "loop");
    setTitleTemplate(draft?.demand_title_template ?? "");
    setBodyTemplate(draft?.demand_body_template ?? "");
    setEmployeeId(draft?.digital_employee_id ?? "");
    setChatObjective(draft?.chat_objective_template ?? "");
    setScheduleKind(draft?.schedule_kind ?? "cron");
    setCronExpr(draft?.cron_expr ?? "0 9 * * *");
    setIntervalHours(
      draft?.interval_seconds
        ? String(Math.max(1, Math.round(draft.interval_seconds / 3600)))
        : "24",
    );
  }, [draft, editing, open]);

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
      <SheetContent className="flex w-full flex-col gap-0 overflow-y-auto sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{isEdit ? "编辑定时规则" : "新建定时规则"}</SheetTitle>
          <SheetDescription>
            到点走任务中枢同一发起链路。发起人固定为当前账号；离开项目后规则将自动停用。
          </SheetDescription>
        </SheetHeader>

        <div className="flex flex-1 flex-col gap-6 px-4 py-4">
          <section className="space-y-3">
            <h3 className="text-sm font-extrabold text-v3-ink">1. 锚点</h3>
            <div className="space-y-2">
              <Label htmlFor="automation-name">规则名称</Label>
              <Input
                id="automation-name"
                placeholder="例如：每周回归巡检"
                value={name}
                onChange={(e) => setName(e.target.value)}
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
                    没有可选项目。请先{" "}
                    <Link className="text-v3-brand underline-offset-2 hover:underline" to="/projects">
                      加入或创建项目
                    </Link>
                    。
                  </p>
                ) : null}
              </div>
            ) : (
              <p className="rounded-[10px] border border-v3-line bg-v3-soft px-3 py-2 text-sm text-v3-ink-2">
                项目与模式创建后不可更改。如需更换，请停用本规则并新建。
              </p>
            )}
          </section>

          {!isEdit ? (
            <section className="space-y-3">
              <h3 className="text-sm font-extrabold text-v3-ink">2. 模式</h3>
              <div className="grid gap-2 sm:grid-cols-3">
                {MODE_CARDS.map((card) => {
                  const selected = mode === card.value;
                  return (
                    <button
                      key={card.value}
                      aria-pressed={selected}
                      className={cn(
                        "rounded-[14px] border px-3 py-3 text-left transition-colors",
                        selected
                          ? "border-v3-brand bg-v3-brand-soft shadow-sm"
                          : "border-v3-line bg-v3-card hover:border-v3-brand/40",
                      )}
                      type="button"
                      onClick={() => setMode(card.value)}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-sm font-semibold text-v3-ink">{card.title}</span>
                        {card.badge ? (
                          <span className="rounded-md bg-v3-brand px-1.5 py-0.5 text-[10px] font-bold text-white">
                            {card.badge}
                          </span>
                        ) : null}
                      </div>
                      <p className="mt-1.5 text-[11.5px] leading-4 text-v3-ink-3">{card.blurb}</p>
                    </button>
                  );
                })}
              </div>
              <HumanGateCallout mode={mode} />
            </section>
          ) : (
            <HumanGateCallout mode={mode} />
          )}

          <section className="space-y-3">
            <h3 className="text-sm font-extrabold text-v3-ink">
              {isEdit ? "2. 内容模板" : "3. 内容模板"}
            </h3>
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
                    placeholder="描述到点后要对员工说什么"
                    rows={4}
                    value={chatObjective}
                    onChange={(e) => setChatObjective(e.target.value)}
                  />
                  <p className="text-[11px] text-v3-ink-3">
                    可用变量：{"{{date}}"} {"{{datetime}}"} {"{{rule_name}}"} {"{{project_name}}"}
                  </p>
                </div>
              </>
            ) : (
              <>
                <div className="space-y-2">
                  <Label htmlFor="demand-title">需求标题模板</Label>
                  <Input
                    id="demand-title"
                    placeholder="例如：{{date}} 例行巡检"
                    value={titleTemplate}
                    onChange={(e) => setTitleTemplate(e.target.value)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="demand-body">需求正文模板</Label>
                  <Textarea
                    id="demand-body"
                    placeholder="写清到点后要完成的目标与约束"
                    rows={5}
                    value={bodyTemplate}
                    onChange={(e) => setBodyTemplate(e.target.value)}
                  />
                  <p className="text-[11px] text-v3-ink-3">
                    可用变量：{"{{date}}"} {"{{datetime}}"} {"{{rule_name}}"} {"{{project_name}}"}
                  </p>
                </div>
              </>
            )}
          </section>

          <section className="space-y-3">
            <h3 className="text-sm font-extrabold text-v3-ink">
              {isEdit ? "3. 日程" : "4. 日程"}
            </h3>
            <V3Segmented
              aria-label="日程种类"
              options={SCHEDULE_OPTIONS}
              value={scheduleKind}
              onChange={setScheduleKind}
            />
            {scheduleKind === "cron" ? (
              <div className="space-y-3">
                <div className="grid gap-2 sm:grid-cols-3">
                  {CRON_PRESETS.map((preset) => {
                    const selected = cronExpr === preset.value;
                    return (
                      <button
                        key={preset.value}
                        aria-pressed={selected}
                        className={cn(
                          "rounded-[12px] border px-3 py-3 text-left text-sm font-medium transition-colors",
                          selected
                            ? "border-v3-brand bg-v3-brand-soft text-v3-brand-deep"
                            : "border-v3-line bg-v3-card text-v3-ink hover:border-v3-brand/40",
                        )}
                        type="button"
                        onClick={() => setCronExpr(preset.value)}
                      >
                        {preset.label}
                      </button>
                    );
                  })}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="cron-expr">高级 cron（可选）</Label>
                  <Input
                    id="cron-expr"
                    aria-label="cron 表达式"
                    className="font-mono text-sm"
                    placeholder="分 时 日 月 周"
                    value={cronExpr}
                    onChange={(e) => setCronExpr(e.target.value)}
                  />
                  <p className="text-[11px] text-v3-ink-3">时区 Asia/Shanghai</p>
                </div>
              </div>
            ) : (
              <div className="space-y-2">
                <Label htmlFor="interval-hours">间隔（小时）</Label>
                <Input
                  id="interval-hours"
                  min={1}
                  type="number"
                  value={intervalHours}
                  onChange={(e) => setIntervalHours(e.target.value)}
                />
              </div>
            )}
            <p className="text-[11px] text-v3-ink-3">
              重叠策略：上一次触发对应的需求/对话尚未结束时，跳过本次。
            </p>
          </section>

          <SoftCard className="space-y-2 border-v3-line p-4 shadow-none">
            <h3 className="text-sm font-extrabold text-v3-ink">到点后你还可能需要</h3>
            <ul className="space-y-1.5">
              {GATE_SUMMARY[mode].map((line) => (
                <li key={line} className="flex items-start gap-2 text-sm text-v3-ink-2">
                  <Check aria-hidden className="mt-0.5 size-3.5 shrink-0 text-v3-brand" />
                  <span>{line}</span>
                </li>
              ))}
            </ul>
            <p className="pt-1 text-[11px] text-v3-ink-3">
              发起人 = 当前账号。被移出项目或账号停用后，规则将自动停用。
            </p>
          </SoftCard>

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
