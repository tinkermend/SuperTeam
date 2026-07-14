import { type ComponentProps, type ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * SuperTeam v3 · 布局基元
 *
 * 布局宪法见 docs/design-system/layout-density.md（宽度档位 / 断点口径 /
 * 主从布局 / 指标带）。宽度取值全部来自 theme.css 的 `--v3-layout-*` /
 * `--v3-metric-*` token；内容区响应一律用容器断点，禁止视口断点。
 */

export type MasterDetailRail = "md" | "lg";

const railGridCols: Record<MasterDetailRail, string> = {
  md: "@4xl/master-detail:grid-cols-[minmax(0,1fr)_var(--v3-layout-rail)]",
  lg: "@5xl/master-detail:grid-cols-[minmax(0,1fr)_var(--v3-layout-rail-lg)]",
};

type MasterDetailLayoutProps = Omit<ComponentProps<"div">, "children"> & {
  /** 主列：队列 / 密集表格等数据本体。 */
  master: ReactNode;
  /** 右栏：选中对象上下文 / triage 面板；窄容器下落到主列下方。 */
  detail: ReactNode;
  /**
   * 右栏档位：md=340px（@4xl 展开双列）、lg=420px（@5xl 展开双列）。
   * 右栏内的 sticky 等响应样式用 `@4xl/master-detail:` / `@5xl/master-detail:`
   * 容器变体，与所选档位保持一致。
   */
  rail?: MasterDetailRail;
};

/** 队列 + 右栏主从布局。取代手写 `xl:grid-cols-[minmax(0,1fr)_NNNpx]`。 */
export function MasterDetailLayout({
  master,
  detail,
  rail = "md",
  className,
  ...props
}: MasterDetailLayoutProps) {
  return (
    <div className={cn("@container/master-detail min-w-0", className)} {...props}>
      <div className={cn("grid min-w-0 items-start gap-5", railGridCols[rail])}>
        {master}
        {detail}
      </div>
    </div>
  );
}

/**
 * KPI 指标带。卡片宽度被限制在 `--v3-metric-min`～`--v3-metric-max`
 * （208–336px）区间：空间不足时先压缩到下限再换行，空间富余时到上限即止、
 * 尾部留白；任何分辨率下卡片密度一致。取代手写 `sm:grid-cols-2 xl:grid-cols-4`。
 * 用 flex-wrap 而非 grid auto-fit：auto-fit 的列数按 max 轨道计算，会在还有
 * 尾部空间时提前把卡片挤到下一行。
 */
export function MetricGrid({ className, ...props }: ComponentProps<"section">) {
  return (
    <section
      className={cn(
        "flex min-w-0 flex-wrap gap-3",
        "[&>*]:min-w-0 [&>*]:max-w-(--v3-metric-max) [&>*]:flex-[1_1_var(--v3-metric-min)]",
        className,
      )}
      {...props}
    />
  );
}
