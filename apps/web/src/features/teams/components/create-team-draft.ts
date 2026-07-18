import { DEFAULT_TEAM_ROLE_ICON_KEY } from "@/components/superteam/team-role-icon-catalog";
import type { UserSummary } from "@/lib/api/auth";
import type { DigitalEmployee } from "@/lib/api/employees";
export type TeamDisplayDraft = {
  color_tone: "blue" | "cyan" | "neutral" | "teal" | "violet";
  // 仅保存本地 2.5D 职能图标的受控键，避免持久化第三方资源 URL。
  icon_key: string;
};

export type CreateTeamDraft = {
  description: string;
  display: TeamDisplayDraft;
  displayTouched: boolean;
  initial_digital_employees: DigitalEmployee[];
  name: string;
  owners: UserSummary[];
  slug: string;
  slugTouched: boolean;
};

export const emptyCreateTeamDraft: CreateTeamDraft = {
  description: "",
  display: { color_tone: "blue", icon_key: DEFAULT_TEAM_ROLE_ICON_KEY },
  displayTouched: false,
  initial_digital_employees: [],
  name: "",
  owners: [],
  slug: "",
  slugTouched: false,
};

/** 把团队名称规整为 URL/接口安全的 slug；非 ASCII 字符会被去除。 */
export function slugify(value: string): string {
  return value
    .normalize("NFKD")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

export function inferTeamDisplay(value: string): TeamDisplayDraft {
  const normalized = value.toLowerCase();
  if (normalized.includes("ops") || normalized.includes("运维"))
    return { color_tone: "cyan", icon_key: "role-cloud-infrastructure" };
  if (normalized.includes("dev") || normalized.includes("研发"))
    return { color_tone: "blue", icon_key: "role-platform-engineering" };
  if (normalized.includes("qa") || normalized.includes("测试"))
    return { color_tone: "violet", icon_key: "role-quality-assurance" };
  if (normalized.includes("security") || normalized.includes("安全"))
    return { color_tone: "teal", icon_key: "role-application-security" };
  return { color_tone: "blue", icon_key: DEFAULT_TEAM_ROLE_ICON_KEY };
}

/**
 * 把创建草稿映射为后端 CreateTeamInput 负载。
 * 仅依赖既有 teams 接口；借调策略等生命周期项在团队创建后单独配置。
 */
export function toCreateTeamInput(draft: CreateTeamDraft) {
  const description = draft.description.trim();

  return {
    name: draft.name.trim(),
    slug: draft.slug.trim(),
    human_owner_user_ids: draft.owners.map((o) => o.id),
    initial_digital_employee_ids: draft.initial_digital_employees.map((e) => e.id),
    ...(description ? { description } : {}),
    metadata: {
      display: {
        color_tone: draft.display.color_tone,
        icon_key: draft.display.icon_key,
      },
    },
  };
}
