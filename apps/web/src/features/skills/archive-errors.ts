import { ApiRequestError } from "@/lib/api/client";
import type { SkillSlugConflict } from "@/lib/api/skills";

export function skillArchivePreviewTitle(error: unknown): string {
  if (error instanceof ApiRequestError && error.status === 413) {
    return "技能包过大";
  }
  return "无法预览技能包";
}

export function skillArchiveErrorMessage(error: unknown): string {
  if (error instanceof ApiRequestError) {
    if (error.status === 400 && error.detail?.includes("zip archive must include SKILL.md")) {
      return "上传失败：技能压缩包必须包含 SKILL.md 文件";
    }
    if (error.detail?.trim()) {
      return error.detail.trim();
    }
    if (error.status === 409) {
      return "技能已存在，请使用「更新技能包」";
    }
    if (error.status === 413) {
      return "技能包过大，平台不提供在线预览，请在本地解压查看";
    }
    if (error.status === 415) {
      return "该文件不是文本，无法在线预览";
    }
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "操作失败，请稍后重试。";
}

export function skillSlugConflict(error: unknown): SkillSlugConflict | undefined {
  if (!(error instanceof ApiRequestError) || error.status !== 409) {
    return undefined;
  }
  const payload = error.payload as Partial<SkillSlugConflict> | undefined;
  if (!payload?.skill_id || !payload.slug) {
    return undefined;
  }
  return {
    code: payload.code ?? "skill_slug_conflict",
    message: payload.message ?? skillArchiveErrorMessage(error),
    skill_id: payload.skill_id,
    slug: payload.slug,
    name: payload.name ?? payload.slug,
  };
}
