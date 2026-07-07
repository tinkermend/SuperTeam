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
  externalCapabilities: string[];
  providerTypes: string[];
};

export type TemplateGovernanceStatus = {
  label: "可用" | "被治理过滤";
  blockedReasons: string[];
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
    externalCapabilities: stringList(defaultCapabilitySelection.enabled_external_capabilities),
    providerTypes: stringList(defaultCapabilitySelection.enabled_provider_types),
  };
}

export function templateDefaultInjectionLine(typeOption: DigitalEmployeeTypeOption) {
  const summary = templateDefaultInjectionSummary(typeOption);
  const parts = [
    summary.skills.length > 0 ? `技能 ${summary.skills.length}` : "",
    summary.mcpServers.length > 0 ? `MCP ${summary.mcpServers.length}` : "",
    summary.externalCapabilities.length > 0 ? `外部能力 ${summary.externalCapabilities.length}` : "",
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

export function templateGovernanceStatus(
  options: DigitalEmployeeCreateOptions | undefined,
  typeOption: DigitalEmployeeTypeOption,
): TemplateGovernanceStatus {
  if (!options) {
    return { label: "可用", blockedReasons: [] };
  }

  const blockedReasons: string[] = [];
  const teamConfig = options.team_config;
  const defaultInjection = templateDefaultInjectionSummary(typeOption);
  const allowedSkills = new Set(teamConfig.allowed_skills ?? []);
  const allowedMcpServers = new Set(teamConfig.allowed_mcp_servers ?? []);
  const allowedExternalCapabilities = new Set(teamConfig.allowed_external_capabilities ?? []);
  const allowedProviders = new Set(teamConfig.allowed_provider_types ?? []);

  const blockedSkills = blockedByAllowList(defaultInjection.skills, allowedSkills);
  const blockedMcp = blockedByAllowList(defaultInjection.mcpServers, allowedMcpServers);
  const blockedExternalCapabilities = blockedByAllowList(
    defaultInjection.externalCapabilities,
    allowedExternalCapabilities,
  );
  const blockedProviders = blockedByAllowList(defaultInjection.providerTypes, allowedProviders);

  if (blockedSkills.length > 0) blockedReasons.push(`技能受限 ${blockedSkills.join(", ")}`);
  if (blockedMcp.length > 0) blockedReasons.push(`MCP 受限 ${blockedMcp.join(", ")}`);
  if (blockedExternalCapabilities.length > 0) {
    blockedReasons.push(`外部能力受限 ${blockedExternalCapabilities.join(", ")}`);
  }
  if (blockedProviders.length > 0) blockedReasons.push(`Provider 受限 ${blockedProviders.join(", ")}`);

  return {
    label: blockedReasons.length > 0 ? "被治理过滤" : "可用",
    blockedReasons,
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

function blockedByAllowList(values: string[], allowedValues: Set<string>) {
  if (allowedValues.size === 0) return [];
  return values.filter((value) => !allowedValues.has(value));
}
