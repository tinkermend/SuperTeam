import type {
  DigitalEmployeeCreateOptions,
  DigitalEmployeeTypeOption,
} from "@/lib/api/employees";
import type { EmployeeTemplate } from "@/lib/api/employee-templates";

export const preferredEmployeeTypeOrder = [
  "frontend_engineer",
  "backend_engineer",
  "database_admin",
  "devops_engineer",
  "fullstack_engineer",
  "implementation_engineer",
  "general_engineer",
];

export type TemplateCapabilitySummary = {
  skills: string[];
  mcpServers: string[];
  providerTypes: string[];
};

export type TemplateDefaultInjectionSummary = {
  skills: string[];
  mcpServers: string[];
  providerTypes: string[];
};

export function orderedEmployeeTypes(employeeTypes: DigitalEmployeeTypeOption[]) {
  return [...employeeTypes]
    .filter((item) => !item.metadata?.["system_type"])
    .sort((left, right) => {
      const leftIndex = preferredEmployeeTypeOrder.indexOf(left.type);
      const rightIndex = preferredEmployeeTypeOrder.indexOf(right.type);
      const normalizedLeft = leftIndex === -1 ? Number.MAX_SAFE_INTEGER : leftIndex;
      const normalizedRight = rightIndex === -1 ? Number.MAX_SAFE_INTEGER : rightIndex;

      if (normalizedLeft !== normalizedRight) {
        return normalizedLeft - normalizedRight;
      }
      return employeeTypes.indexOf(left) - employeeTypes.indexOf(right);
    });
}

export function firstPreferredEmployeeType(employeeTypes: DigitalEmployeeTypeOption[]) {
  return orderedEmployeeTypes(employeeTypes)[0];
}

export function templateRisk(_typeOption: DigitalEmployeeTypeOption) {
  return "medium";
}

export function riskSortValue(risk: string) {
  const order: Record<string, number> = { low: 1, medium: 2, high: 3, critical: 4 };
  return order[risk] ?? Number.MAX_SAFE_INTEGER;
}

export function templateSearchText(typeOption: DigitalEmployeeTypeOption) {
  const capability = templateCapabilitySummary(typeOption);
  return [
    typeOption.type,
    typeOption.label,
    typeOption.description,
    typeOption.default_role,
    ...capability.skills,
    ...capability.mcpServers,
    ...capability.providerTypes,
  ]
    .join(" ")
    .toLowerCase();
}

export function templateCapabilitySummary(typeOption: DigitalEmployeeTypeOption): TemplateCapabilitySummary {
  return {
    skills: typeOption.recommended_skills ?? [],
    mcpServers: typeOption.recommended_mcp_servers ?? [],
    providerTypes: typeOption.recommended_provider_types ?? [],
  };
}

export function templateDefaultInjectionSummary(
  typeOption: DigitalEmployeeTypeOption,
): TemplateDefaultInjectionSummary {
  const capabilityBindings = typeOption.capability_bindings ?? {};
  return {
    skills: stringList(capabilityBindings.skills),
    mcpServers: stringList(capabilityBindings.mcp_servers),
    providerTypes: stringList(capabilityBindings.provider_types),
  };
}

export function templateDefaultInjectionLine(typeOption: DigitalEmployeeTypeOption) {
  const summary = templateDefaultInjectionSummary(typeOption);
  const parts = [
    summary.skills.length > 0 ? `技能 ${summary.skills.length}` : "",
    summary.mcpServers.length > 0 ? `MCP ${summary.mcpServers.length}` : "",
    summary.providerTypes.length > 0 ? `Provider ${summary.providerTypes.length}` : "",
  ].filter(Boolean);

  return parts.join(" · ") || "无默认注入";
}

export function templateCapabilityPreview(typeOption: DigitalEmployeeTypeOption) {
  const summary = templateCapabilitySummary(typeOption);
  return [
    summary.skills[0],
    summary.mcpServers[0],
    summary.providerTypes[0],
  ].filter(Boolean).join(" · ") || "无模板能力";
}

export type TemplateUnlistedRecommendations = {
  skills: string[];
  mcpServers: string[];
  total: number;
};

// templateUnlistedRecommendations 汇总模板推荐(recommended_* ∪ capability_bindings)
// 中注册表里不存在的 key——它们在创建向导中会以"未上架"禁选展示,这里让模板
// 管理员在治理侧看到同一口径的缺口。注册表尚未加载(sets 为 undefined)时不告警。
export function templateUnlistedRecommendations(
  typeOption: DigitalEmployeeTypeOption,
  availableSkillKeys: ReadonlySet<string> | undefined,
  availableMcpKeys: ReadonlySet<string> | undefined,
): TemplateUnlistedRecommendations {
  const capability = templateCapabilitySummary(typeOption);
  const injection = templateDefaultInjectionSummary(typeOption);
  const skills = availableSkillKeys
    ? dedupe([...capability.skills, ...injection.skills]).filter((key) => !availableSkillKeys.has(key))
    : [];
  const mcpServers = availableMcpKeys
    ? dedupe([...capability.mcpServers, ...injection.mcpServers]).filter((key) => !availableMcpKeys.has(key))
    : [];
  return { skills, mcpServers, total: skills.length + mcpServers.length };
}

function dedupe(values: string[]) {
  return Array.from(new Set(values));
}

export function findTemplateByType(
  options: DigitalEmployeeCreateOptions | undefined,
  templateType: string | undefined,
) {
  const normalized = templateType?.trim();
  if (!normalized) return undefined;
  return options?.employee_types.find((item) => item.type === normalized);
}

export function stringList(value: unknown) {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string" && item.trim().length > 0);
}

export function stringValue(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

export function templateStatusLabel(template: EmployeeTemplate): "已启用" | "已禁用" {
  return template.status === "active" ? "已启用" : "已禁用";
}

export function templateStatusTone(template: EmployeeTemplate): "ok" | "mute" {
  return template.status === "active" ? "ok" : "mute";
}
