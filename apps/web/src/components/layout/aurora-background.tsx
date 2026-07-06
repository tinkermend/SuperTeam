/**
 * 全局沉浸极光背景底。
 *
 * 挂在 authenticated shell 内一次，全站页面共享同一层极光氛围背景（token 驱动，浅/深色自动切换）。
 * 只铺"背景底"：数据卡片、表格、审计/日志面等内容表面仍保持不透明高对比，可读性不受影响。
 * 沉浸玻璃卡/hero 仅限 DESIGN.md 定义的 Tier A 入口/创建页叠加使用。
 *
 * 视觉全部来自 apps/web/src/styles/theme.css 的 `--v3-aurora-*` token；本组件不承载任何色值。
 */
export function AuroraBackground() {
  return (
    <div
      aria-hidden
      className="v3-aurora-background"
      data-slot="v3-aurora-background"
    />
  );
}
