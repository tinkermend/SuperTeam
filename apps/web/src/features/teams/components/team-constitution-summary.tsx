import { ShieldCheck } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { Button, StatusPill, WorkSurface } from "@/components/superteam";
import { constitutionCategoryLabel } from "@/lib/status-labels";

// 观察面：只列出当前生效的规则，编辑在团队配置页的「约束」分区。
// 措辞刻意用「规则/提醒」而非「硬性规则」——宪法是注入 provider 提示词的软提醒，
// 不是强制门禁（D9），"硬性"两字会让人误以为写了就一定会被遵守。
export function TeamConstitutionSummary({
  constitution,
  teamId
}: {
  constitution?: Record<string, unknown>;
  teamId: string;
}) {
  // 优先读结构化 rules（带分类）；旧快照只有 hard_rules 时按纯文本回退。
  const structured = Array.isArray(constitution?.rules)
    ? (constitution?.rules as unknown[])
        .filter((item): item is Record<string, unknown> => typeof item === "object" && item !== null)
        .map((item) => ({
          text: typeof item.text === "string" ? item.text : "",
          category: typeof item.category === "string" ? item.category : "must"
        }))
        .filter((rule) => rule.text.trim().length > 0)
    : undefined;
  const rules =
    structured ??
    (Array.isArray(constitution?.hard_rules)
      ? (constitution?.hard_rules as unknown[])
          .filter((rule): rule is string => typeof rule === "string" && rule.trim().length > 0)
          .map((text) => ({ text, category: "must" }))
      : []);

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-base font-bold text-ink">团队宪法</h2>
            <StatusPill tone="mute">{rules.length} 条规则</StatusPill>
          </div>
          <p className="mt-1 text-[13px] text-ink-2">注入执行提示词的团队级提醒，非强制门禁。</p>
        </div>
        <Button asChild size="sm" variant="ghost">
          <Link hash="constitution" params={{ teamId }} to="/teams/$teamId/config">
            去配置
          </Link>
        </Button>
      </div>
      <div className="p-5">
        {rules.length === 0 ? (
          <p className="text-[13px] text-ink-2">尚未配置规则。</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {rules.map((rule, index) => (
              <li
                key={`${index}-${rule.text}`}
                className="flex items-start gap-2 rounded-[12px] border border-line bg-card-soft/60 px-3 py-2.5"
              >
                <ShieldCheck className="mt-0.5 size-4 shrink-0 text-ink-3" />
                <StatusPill tone="mute">{constitutionCategoryLabel(rule.category)}</StatusPill>
                <span className="text-[13px] text-ink">{rule.text}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </WorkSurface>
  );
}
