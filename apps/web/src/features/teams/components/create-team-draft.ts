import type { UserSummary } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";

export type TeamDisplayDraft = {
  color_tone: "blue" | "cyan" | "neutral" | "teal" | "violet";
  // 旧版固定 5 键（default/dev/ops/qa/security）或任意 lucide 图标名（kebab-case）。
  icon_key: string;
};

export type CreateTeamDraft = {
  display: TeamDisplayDraft;
  displayTouched: boolean;
  initial_members: InitialTeamMemberInput[];
  memberUsers: Record<string, UserSummary>;
  name: string;
  owner?: UserSummary;
  slug: string;
  slugTouched: boolean;
};

export const emptyCreateTeamDraft: CreateTeamDraft = {
  display: { color_tone: "blue", icon_key: "default" },
  displayTouched: false,
  initial_members: [],
  memberUsers: {},
  name: "",
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
    return { color_tone: "cyan", icon_key: "ops" };
  if (normalized.includes("dev") || normalized.includes("研发"))
    return { color_tone: "blue", icon_key: "dev" };
  if (normalized.includes("qa") || normalized.includes("测试"))
    return { color_tone: "violet", icon_key: "qa" };
  if (normalized.includes("security") || normalized.includes("安全"))
    return { color_tone: "teal", icon_key: "security" };
  return { color_tone: "blue", icon_key: "default" };
}

/**
 * 把创建草稿映射为后端 CreateTeamInput 负载。
 * 仅依赖既有 teams 接口；借调策略等生命周期项在团队创建后单独配置。
 */
export function toCreateTeamInput(draft: CreateTeamDraft) {
  return {
    name: draft.name.trim(),
    slug: draft.slug.trim(),
    human_owner_user_ids: draft.owner?.id ? [draft.owner.id] : [],
    initial_members: draft.initial_members,
    metadata: {
      display: {
        color_tone: draft.display.color_tone,
        icon_key: draft.display.icon_key,
      },
    },
  };
}
