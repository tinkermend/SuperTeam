import {
  useLayoutEffect,
  useRef,
  useState,
  type ComponentProps,
  type ReactNode,
} from "react";
import { cn } from "@/lib/utils";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";

/**
 * SuperTeam v3 · 布局基元
 *
 * 布局宪法见 docs/design-system/layout-density.md（宽度档位 / 断点口径 /
 * 主从布局 / 指标带）。宽度取值全部来自 theme.css 的 `--v3-layout-*` /
 * `--v3-metric-*` token；内容区响应一律用容器断点，禁止视口断点。
 */

export type MasterDetailRail = "md" | "lg";

/** 双列展开阈值（rem），与下方容器断点变体（@4xl/@5xl）保持同源。 */
const railThresholdRem: Record<MasterDetailRail, number> = {
  md: 56,
  lg: 64,
};

/** 右栏列宽：上限取 rail token，空间紧张时可压缩，杜绝任何环境下的横向越界。 */
const railGridCols: Record<MasterDetailRail, string> = {
  md: "@4xl/master-detail:grid-cols-[minmax(0,1fr)_minmax(min(100%,16rem),var(--v3-layout-rail))]",
  lg: "@5xl/master-detail:grid-cols-[minmax(0,1fr)_minmax(min(100%,18rem),var(--v3-layout-rail-lg))]",
};

type MasterDetailLayoutProps = Omit<ComponentProps<"div">, "children"> & {
  /** 主列：队列 / 密集表格等数据本体；无 detail 时独占全宽。 */
  master: ReactNode;
  /**
   * 详情层（按需渲染）：选中对象时才传入。宽容器下作为 in-flow 右栏，
   * 窄容器下自动改为右侧 Sheet 抽屉；未选中传 undefined，不保留空态占位栏。
   */
  detail?: ReactNode;
  /**
   * 右栏档位：md=340px（容器 @4xl 展开双列）、lg=420px（@5xl 展开双列）。
   * 右栏内的 sticky 等响应样式用 `@4xl/master-detail:` / `@5xl/master-detail:`
   * 容器变体，与所选档位保持一致（Sheet 内这些变体不命中，自然失效）。
   */
  rail?: MasterDetailRail;
  /** Sheet 模式的无障碍标题（sr-only），默认「详情」。 */
  detailLabel?: string;
  /** Sheet 模式下用户关闭抽屉时回调（通常用于清除选中态）。 */
  onDetailDismiss?: () => void;
  /**
   * 窄容器下 detail 的去处：`sheet`（默认）适合选中对象的按需上下文；
   * `stack` 适合常驻面板（如驾驶舱右栏），窄容器时堆到主列下方而非弹抽屉。
   */
  narrowDetail?: "sheet" | "stack";
};

/**
 * 队列 + 按需详情层的主从布局。取代手写 `xl:grid-cols-[minmax(0,1fr)_NNNpx]`
 * 与常驻空态右栏：未选中时主列全宽；选中时宽容器右栏 in-flow、窄容器 Sheet。
 */
export function MasterDetailLayout({
  master,
  detail,
  rail = "md",
  detailLabel = "详情",
  onDetailDismiss,
  narrowDetail = "sheet",
  className,
  ...props
}: MasterDetailLayoutProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  // 初值取 true：宽屏首帧直接 in-flow，窄屏由首次测量立即修正，避免 Sheet 闪现。
  const [isWide, setIsWide] = useState(true);

  useLayoutEffect(() => {
    const root = rootRef.current;
    if (!root) {
      return;
    }
    const rootFont =
      parseFloat(getComputedStyle(document.documentElement).fontSize) || 16;
    const thresholdPx = railThresholdRem[rail] * rootFont;
    const measure = (width: number) => setIsWide(width >= thresholdPx);
    measure(root.getBoundingClientRect().width);
    const observer = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (entry) {
        measure(entry.contentRect.width);
      }
    });
    observer.observe(root);
    return () => observer.disconnect();
  }, [rail]);

  const detailInFlow =
    Boolean(detail) && (isWide || narrowDetail === "stack");

  return (
    <div
      className={cn("@container/master-detail min-w-0", className)}
      ref={rootRef}
      {...props}
    >
      <div
        className={cn(
          "grid min-w-0 items-start gap-5",
          detailInFlow && isWide && railGridCols[rail],
        )}
      >
        {master}
        {detailInFlow ? detail : null}
      </div>
      {detail && !isWide && narrowDetail === "sheet" ? (
        <Sheet
          open
          onOpenChange={(open) => {
            if (!open) {
              onDetailDismiss?.();
            }
          }}
        >
          <SheetContent
            aria-describedby={undefined}
            className="w-(--v3-layout-rail-lg) max-w-[calc(100vw-2rem)] sm:max-w-(--v3-layout-rail-lg) overflow-y-auto p-3"
            side="right"
          >
            <SheetTitle className="sr-only">{detailLabel}</SheetTitle>
            {detail}
          </SheetContent>
        </Sheet>
      ) : null}
    </div>
  );
}

/**
 * KPI 指标带。卡片宽度被限制在 `--v3-metric-min`～`--v3-metric-max`
 * （208–336px）区间：空间不足时先压缩到下限再换行，任何分辨率下卡片密度一致。
 * 取代手写 `sm:grid-cols-2 xl:grid-cols-4`。
 * 卡片到达宽度上限后，剩余空间摊进卡间距（≥3 张时 justify-between，gap 为
 * 最小间距），使首尾卡与下方内容块两端对齐；1–2 张时保持左对齐，避免两端
 * 分布留出中间大洞。用 flex-wrap 而非 grid auto-fit：auto-fit 的列数按 max
 * 轨道计算，会在还有尾部空间时提前把卡片挤到下一行。
 */
export function MetricGrid({ className, ...props }: ComponentProps<"section">) {
  return (
    <section
      className={cn(
        "flex min-w-0 flex-wrap gap-3 has-[>:nth-child(3)]:justify-between",
        "[&>*]:min-w-0 [&>*]:max-w-(--v3-metric-max) [&>*]:flex-[1_1_var(--v3-metric-min)]",
        className,
      )}
      {...props}
    />
  );
}
