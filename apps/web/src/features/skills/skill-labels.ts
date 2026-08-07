/** 技能来源等面向用户的中文标签（禁止工作面直渲 API 枚举）。 */

const SKILL_SOURCE_LABELS: Record<string, string> = {
  upload: "上传",
  system: "系统内置",
  marketplace: "市场",
  none: "未记录",
};

export function skillSourceLabel(source: string | undefined | null): string {
  const key = (source ?? "").trim();
  if (!key) return "未记录";
  return SKILL_SOURCE_LABELS[key] ?? key;
}
