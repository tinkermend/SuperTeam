import type {
  DigitalEmployeeCreateOptions,
  DigitalEmployeeTypeOption,
} from "@/lib/api/employees";

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

export type TemplateAvailabilityStatus = {
  label: "平台默认" | "继承团队基线";
  notes: string[];
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

export function templateRisk(typeOption: DigitalEmployeeTypeOption) {
  return stringValue(typeOption.default_approval_policy?.min_risk_for_human) || "medium";
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
  const defaultCapabilitySelection = typeOption.default_capability_selection ?? {};
  return {
    skills: stringList(defaultCapabilitySelection.enabled_skills),
    mcpServers: stringList(defaultCapabilitySelection.enabled_mcp_servers),
    providerTypes: stringList(defaultCapabilitySelection.enabled_provider_types),
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

export function templateAvailabilityStatus(
  options: DigitalEmployeeCreateOptions | undefined,
): TemplateAvailabilityStatus {
  if (!options) {
    return { label: "平台默认", notes: [] };
  }

  const teamConfig = options.team_config;
  const notes: string[] = [];
  if ((teamConfig.skills ?? []).length > 0) {
    notes.push(`团队继承技能 ${teamConfig.skills.length}`);
  }
  if ((teamConfig.mcp_servers ?? []).length > 0) {
    notes.push(`团队继承 MCP ${teamConfig.mcp_servers.length}`);
  }
  if (Object.keys(teamConfig.constitution ?? {}).length > 0) {
    notes.push("创建时会继承团队宪法基线");
  }
  return {
    label: notes.length > 0 ? "继承团队基线" : "平台默认",
    notes,
  };
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
