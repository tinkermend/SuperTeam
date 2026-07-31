import { useState, type ReactNode } from "react";
import { ClipboardCheck, Package } from "lucide-react";

import { EmptyState, SoftCard, StatusPill } from "@/components/superteam";
import { dossierRailItemStateLabel } from "@/lib/status-labels";
import type {
  DemandDossierRailItemState,
  DemandDossierRailSlot,
  ProjectDemandDossier,
} from "@/lib/api/projects";

const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function railItemTone(state: DemandDossierRailItemState) {
  switch (state) {
    case "delivered":
      return "ok" as const;
    case "missing":
      return "warn" as const;
    case "info":
      return "info" as const;
    default:
      return "mute" as const;
  }
}

/**
 * 一单卷宗右轨：交付判定汇总 + 按剧本 kind 分槽的交付事实。
 *
 * 诚实边界：`unknown` 表示"暂无声明，无法判定"，**不得**渲染成失败——把没有
 * 结构化声明的任务显示成红色未交付，会让人去追一个并不存在的问题。
 */
export function DemandDossierRail({
  acceptance,
  acceptanceDetail,
  acceptanceOpen = false,
  handoffSummary,
  onAcceptanceToggle,
  slots,
}: {
  acceptance: ProjectDemandDossier["acceptance"];
  /** 验收明细（既有判据血缘面板）；折叠态不渲染，避免为看一行摘要先拉一次明细。 */
  acceptanceDetail?: ReactNode;
  acceptanceOpen?: boolean;
  handoffSummary: ProjectDemandDossier["handoff_summary"];
  onAcceptanceToggle?: () => void;
  slots: DemandDossierRailSlot[];
}) {
  const [showAssessments, setShowAssessments] = useState(false);
  const total =
    handoffSummary.fulfilled +
    handoffSummary.partial +
    handoffSummary.unfulfilled +
    handoffSummary.unknown;

  return (
    <div className="grid gap-3" data-testid="demand-dossier-rail">
      <SoftCard className="p-4" data-testid="demand-dossier-handoff-summary">
        <div className="flex items-center gap-2">
          <ClipboardCheck className="size-4 text-ink-2" />
          <h3 className="text-sm font-semibold text-ink">交付判定</h3>
        </div>
        {total === 0 ? (
          <p className="mt-2 text-[12.5px] text-ink-3">本单尚无可判定的任务交接。</p>
        ) : (
          <>
            <div className="mt-3 flex flex-wrap gap-1.5">
              <StatusPill tone="ok">已交付 {handoffSummary.fulfilled}</StatusPill>
              <StatusPill tone="warn">部分交付 {handoffSummary.partial}</StatusPill>
              <StatusPill tone="danger">未交付 {handoffSummary.unfulfilled}</StatusPill>
              {/* unknown 用 mute：它是"没声明"，不是"没做到"。 */}
              <StatusPill tone="mute">暂无声明 {handoffSummary.unknown}</StatusPill>
            </div>
            {handoffSummary.unknown > 0 ? (
              <p className="mt-2 text-[11.5px] leading-5 text-ink-3">
                「暂无声明」表示该任务没有结构化交付声明，无法判定，并不代表未完成。
              </p>
            ) : null}
            <button
              className="mt-2 text-[12px] font-semibold text-brand hover:underline"
              onClick={() => setShowAssessments((value) => !value)}
              type="button"
            >
              {showAssessments ? "收起明细" : "按任务查看明细"}
            </button>
            {showAssessments ? (
              <ul className="mt-2 grid gap-1.5">
                {handoffSummary.assessments.map((assessment) => (
                  <li
                    className="flex items-start justify-between gap-2 rounded-inner bg-card-soft px-2.5 py-2"
                    key={assessment.project_task_id}
                  >
                    <span className="min-w-0 truncate text-[12px] text-ink-2">
                      {assessment.project_task_name || "未命名任务"}
                    </span>
                    <StatusPill
                      tone={
                        assessment.status === "fulfilled"
                          ? "ok"
                          : assessment.status === "partial"
                            ? "warn"
                            : assessment.status === "unfulfilled"
                              ? "danger"
                              : "mute"
                      }
                    >
                      {assessment.status === "fulfilled"
                        ? "已交付"
                        : assessment.status === "partial"
                          ? "部分交付"
                          : assessment.status === "unfulfilled"
                            ? "未交付"
                            : "暂无声明"}
                    </StatusPill>
                  </li>
                ))}
              </ul>
            ) : null}
          </>
        )}
      </SoftCard>

      {slots.length === 0 ? (
        <SoftCard className="p-6">
          <EmptyState
            description="等这一单产生结论、证据、工件或分支提交后，交付事实会按剧本分槽出现在这里。"
            icon={<Package />}
            title="本单尚未形成可展示的交付事实"
          />
        </SoftCard>
      ) : (
        slots.map((slot) => (
          <SoftCard className="p-4" data-testid={`demand-dossier-slot-${slot.kind}`} key={slot.kind}>
            <div className="flex items-center justify-between gap-2">
              {/* 槽标题用服务端已中文化的 title；kind 只作技术键不外显。 */}
              <h3 className="text-sm font-semibold text-ink">{slot.title}</h3>
              <span className="text-[11px] tabular-nums text-ink-3">{slot.items.length}</span>
            </div>
            {slot.items.length === 0 ? (
              <p className="mt-2 text-[12px] text-ink-3">本单尚无该类交付事实。</p>
            ) : (
              <ul className="mt-2.5 grid gap-2">
                {slot.items.map((item) => (
                  <li className="rounded-inner bg-card-soft px-2.5 py-2" key={item.id}>
                    <div className="flex items-start justify-between gap-2">
                      <p className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-ink">
                        {item.title}
                      </p>
                      <StatusPill tone={railItemTone(item.state)}>
                        {dossierRailItemStateLabel(item.state)}
                      </StatusPill>
                    </div>
                    {item.summary ? (
                      <p className="mt-1 line-clamp-2 text-[12px] leading-5 text-ink-2">
                        {item.summary}
                      </p>
                    ) : null}
                    {item.ref ? (
                      <p className="mt-1 truncate font-mono text-[11px] text-ink-3" title={item.ref}>
                        {/* 纯 UUID 的引用加「引用 ·」前缀：路径/URI 自带语义可直接显示，
                            裸标识符单独一行会被读成"这个对象叫这个名字"。 */}
                        {UUID_PATTERN.test(item.ref) ? `引用 · ${item.ref}` : item.ref}
                      </p>
                    ) : null}
                    {item.project_task_name ? (
                      <p className="mt-1 truncate text-[11px] text-ink-3">
                        来自 · {item.project_task_name}
                      </p>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
          </SoftCard>
        ))
      )}

      {acceptance && (acceptance.criteria_total ?? 0) > 0 ? (
        <SoftCard className="p-4" data-testid="demand-dossier-acceptance-summary">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <h3 className="text-sm font-semibold text-ink">验收</h3>
            {onAcceptanceToggle ? (
              <button
                className="text-[12px] font-semibold text-brand hover:underline"
                data-testid="demand-dossier-acceptance-toggle"
                onClick={onAcceptanceToggle}
                type="button"
              >
                {acceptanceOpen ? "收起判据" : "展开判据"}
              </button>
            ) : null}
          </div>
          <p className="mt-1.5 text-[12.5px] text-ink-2">
            {acceptance.criteria_total} 条判据
            {(acceptance.pending_human_judgment ?? 0) > 0
              ? ` · 待签 ${acceptance.pending_human_judgment}`
              : " · 无待签"}
          </p>
          {acceptanceOpen && acceptanceDetail ? (
            <div className="mt-3">{acceptanceDetail}</div>
          ) : null}
        </SoftCard>
      ) : null}
    </div>
  );
}
