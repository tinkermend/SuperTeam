import type { UserSummary } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";

export type TeamDisplayDraft = {
  color_tone: "blue" | "cyan" | "neutral" | "teal" | "violet";
  icon_key: "default" | "dev" | "ops" | "qa" | "security";
};

export type CreateTeamDraft = {
  display: TeamDisplayDraft;
  displayTouched: boolean;
  initial_members: InitialTeamMemberInput[];
  memberUsers: Record<string, UserSummary>;
  name: string;
  owner?: UserSummary;
  slug: string;
};

export const emptyCreateTeamDraft: CreateTeamDraft = {
  display: { color_tone: "neutral", icon_key: "default" },
  displayTouched: false,
  initial_members: [],
  memberUsers: {},
  name: "",
  slug: "",
};

export const teamIconOptions = [
  { color_tone: "cyan", icon_key: "ops", label: "选择运维团队图标" },
  { color_tone: "blue", icon_key: "dev", label: "选择研发团队图标" },
  { color_tone: "violet", icon_key: "qa", label: "选择测试团队图标" },
  { color_tone: "teal", icon_key: "security", label: "选择安全团队图标" },
  { color_tone: "neutral", icon_key: "default", label: "选择默认团队图标" },
] as const;

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
  return { color_tone: "neutral", icon_key: "default" };
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
