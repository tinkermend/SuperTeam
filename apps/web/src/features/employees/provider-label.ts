const PROVIDER_LABELS: Record<string, string> = {
  claude: "Claude Code",
  claude_code: "Claude Code",
  codex: "Codex",
  open_code: "OpenCode",
  opencode: "OpenCode",
  unknown: "未知",
};

/** 统一 provider 展示名：大小写、`-`/`_` 分隔符变体均归一；未知值原样返回。 */
export function providerDisplayName(value: string): string {
  const normalized = value.trim().toLowerCase().replace(/-/g, "_");
  return PROVIDER_LABELS[normalized] ?? value;
}
