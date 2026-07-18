export type TeamRoleIcon = {
  key: string;
  keywords: readonly string[];
  label: string;
  src: string;
};

export const DEFAULT_TEAM_ROLE_ICON_KEY = "role-general-team";

/**
 * Curated, locally hosted 2.5D team-role illustrations. Keeping the asset key
 * in team metadata rather than an arbitrary URL makes the visual system stable,
 * cacheable, and independent of third-party asset hosts.
 */
export const teamRoleIcons = [
  { key: DEFAULT_TEAM_ROLE_ICON_KEY, label: "通用团队", keywords: ["团队", "通用", "协作"], src: "/images/team-role-icons/general-team.webp" },
  { key: "role-frontend-web", label: "Web 前端", keywords: ["前端", "web", "浏览器"], src: "/images/team-role-icons/frontend-web.webp" },
  { key: "role-frontend-mobile", label: "移动端前端", keywords: ["前端", "移动端", "手机"], src: "/images/team-role-icons/frontend-mobile.webp" },
  { key: "role-backend-api", label: "后端 API", keywords: ["后端", "api", "服务端"], src: "/images/team-role-icons/backend-api.webp" },
  { key: "role-backend-data-services", label: "数据服务", keywords: ["后端", "数据服务", "服务"], src: "/images/team-role-icons/backend-data-services.webp" },
  { key: "role-platform-engineering", label: "平台工程", keywords: ["平台", "工程", "基础设施"], src: "/images/team-role-icons/platform-engineering.webp" },
  { key: "role-cloud-infrastructure", label: "云基础设施", keywords: ["云", "基础设施", "服务器"], src: "/images/team-role-icons/cloud-infrastructure.webp" },
  { key: "role-sre-observability", label: "SRE 与可观测性", keywords: ["sre", "可观测性", "监控"], src: "/images/team-role-icons/sre-observability.webp" },
  { key: "role-devops-cicd", label: "DevOps 与持续交付", keywords: ["devops", "cicd", "持续交付"], src: "/images/team-role-icons/devops-cicd.webp" },
  { key: "role-quality-assurance", label: "质量保障", keywords: ["测试", "qa", "质量"], src: "/images/team-role-icons/quality-assurance.webp" },
  { key: "role-quality-automation", label: "自动化测试", keywords: ["测试", "自动化", "qa"], src: "/images/team-role-icons/quality-automation.webp" },
  { key: "role-performance-engineering", label: "性能工程", keywords: ["性能", "压测", "测试"], src: "/images/team-role-icons/performance-engineering.webp" },
  { key: "role-application-security", label: "应用安全", keywords: ["安全", "应用安全", "appsec"], src: "/images/team-role-icons/application-security.webp" },
  { key: "role-cloud-security", label: "云安全", keywords: ["安全", "云安全", "cloud"], src: "/images/team-role-icons/cloud-security.webp" },
  { key: "role-data-security", label: "数据安全与隐私", keywords: ["安全", "数据安全", "隐私"], src: "/images/team-role-icons/data-security.webp" },
  { key: "role-product-discovery", label: "产品探索", keywords: ["产品", "探索", "调研"], src: "/images/team-role-icons/product-discovery.webp" },
  { key: "role-product-delivery", label: "产品交付", keywords: ["产品", "交付", "上线"], src: "/images/team-role-icons/product-delivery.webp" },
  { key: "role-project-program-management", label: "项目与项目群管理", keywords: ["项目", "项目群", "管理"], src: "/images/team-role-icons/project-program-management.webp" },
  { key: "role-ux-ui-design", label: "UX/UI 设计", keywords: ["设计", "ux", "ui"], src: "/images/team-role-icons/ux-ui-design.webp" },
  { key: "role-implementation-consulting", label: "实施咨询", keywords: ["实施", "咨询", "交付"], src: "/images/team-role-icons/implementation-consulting.webp" },
  { key: "role-solution-architecture", label: "解决方案架构", keywords: ["架构", "解决方案", "方案"], src: "/images/team-role-icons/solution-architecture.webp" },
  { key: "role-customer-support", label: "客户成功与技术支持", keywords: ["客户成功", "支持", "客服"], src: "/images/team-role-icons/customer-support.webp" },
  { key: "role-data-engineering", label: "数据工程", keywords: ["数据", "数据工程", "管道"], src: "/images/team-role-icons/data-engineering.webp" },
  { key: "role-data-analytics", label: "数据分析与 BI", keywords: ["数据", "分析", "bi"], src: "/images/team-role-icons/data-analytics.webp" },
  { key: "role-machine-learning-engineering", label: "机器学习工程", keywords: ["机器学习", "ml", "模型"], src: "/images/team-role-icons/machine-learning-engineering.webp" },
  { key: "role-llmops", label: "LLMOps", keywords: ["llmops", "ai", "模型运营"], src: "/images/team-role-icons/llmops.webp" },
  { key: "role-database-administration", label: "数据库管理", keywords: ["数据库", "dba", "数据"], src: "/images/team-role-icons/database-administration.webp" },
  { key: "role-release-management", label: "发布管理", keywords: ["发布", "上线", "版本"], src: "/images/team-role-icons/release-management.webp" },
  { key: "role-technical-writing", label: "技术写作与赋能", keywords: ["文档", "技术写作", "赋能"], src: "/images/team-role-icons/technical-writing.webp" },
  { key: "role-integration-engineering", label: "集成工程", keywords: ["集成", "连接器", "api"], src: "/images/team-role-icons/integration-engineering.webp" },
  { key: "role-finops", label: "FinOps 与云成本", keywords: ["finops", "成本", "云成本"], src: "/images/team-role-icons/finops.webp" },
] as const satisfies readonly TeamRoleIcon[];

export function getTeamRoleIcon(iconKey?: string) {
  return teamRoleIcons.find((icon) => icon.key === iconKey);
}
