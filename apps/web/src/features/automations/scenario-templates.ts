import type {
  AutomationCoordinationMode,
  CreateAutomationRuleInput,
} from "@/lib/api/automations";

export type AutomationRuleDraft = Partial<CreateAutomationRuleInput> & {
  name: string;
  coordination_mode: AutomationCoordinationMode;
  schedule_kind: "cron" | "interval";
};

export type AutomationScenarioTemplate = {
  id: string;
  title: string;
  description: string;
  draft: AutomationRuleDraft;
};

export const AUTOMATION_SCENARIO_TEMPLATES: AutomationScenarioTemplate[] = [
  {
    id: "weekday-loop",
    title: "工作日 Loop 巡检",
    description: "工作日 09:00 自动开跑，终态仍等人验收。",
    draft: {
      name: "工作日例行巡检",
      coordination_mode: "loop",
      schedule_kind: "cron",
      cron_expr: "0 9 * * 1-5",
      timezone: "Asia/Shanghai",
      demand_title_template: "{{date}} 例行巡检",
      demand_body_template:
        "请按项目当前目标执行例行巡检，汇总阻塞与风险。\n规则：{{rule_name}}\n项目：{{project_name}}",
      enabled: true,
    },
  },
  {
    id: "weekly-plan",
    title: "每周一 Plan",
    description: "每周一生成待确认计划，适合半自动立项。",
    draft: {
      name: "每周计划发起",
      coordination_mode: "plan",
      schedule_kind: "cron",
      cron_expr: "0 9 * * 1",
      timezone: "Asia/Shanghai",
      demand_title_template: "{{date}} 周计划",
      demand_body_template:
        "请为项目生成本周执行计划，等待人类确认后再派发。\n规则：{{rule_name}}",
      enabled: true,
    },
  },
  {
    id: "daily-chat",
    title: "每日对话巡检",
    description: "到点向指定员工发起对话；不进项目验收闭环。",
    draft: {
      name: "每日对话巡检",
      coordination_mode: "chat",
      schedule_kind: "cron",
      cron_expr: "0 9 * * *",
      timezone: "Asia/Shanghai",
      chat_objective_template:
        "请检查项目 {{project_name}} 今日状态，列出风险与待办。日期：{{date}}",
      enabled: true,
    },
  },
];
