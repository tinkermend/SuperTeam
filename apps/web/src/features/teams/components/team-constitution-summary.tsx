import { ShieldCheck } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { Button, StatusPill, WorkSurface } from "@/components/superteam";

// 观察面：只列出当前生效的硬性规则，编辑在团队配置页的「约束」分区。
export function TeamConstitutionSummary({
  constitution,
  teamId
}: {
  constitution?: Record<string, unknown>;
  teamId: string;
}) {
  const rules = Array.isArray(constitution?.hard_rules)
    ? (constitution?.hard_rules as unknown[]).filter(
        (rule): rule is string => typeof rule === "string" && rule.trim().length > 0,
      )
    : [];

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-base font-bold text-ink">团队宪法</h2>
            <StatusPill tone="mute">{rules.length} 条硬性规则</StatusPill>
          </div>
          <p className="mt-1 text-[13px] text-ink-2">约束执行边界的硬性规则。</p>
        </div>
        <Button asChild size="sm" variant="ghost">
          <Link hash="constitution" params={{ teamId }} to="/teams/$teamId/config">
            去配置
          </Link>
        </Button>
      </div>
      <div className="p-5">
        {rules.length === 0 ? (
          <p className="text-[13px] text-ink-2">尚未配置硬性规则。</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {rules.map((rule, index) => (
              <li
                key={`${index}-${rule}`}
                className="flex items-start gap-2 rounded-[12px] border border-line bg-card-soft/60 px-3 py-2.5"
              >
                <ShieldCheck className="mt-0.5 size-4 shrink-0 text-ink-3" />
                <span className="text-[13px] text-ink">{rule}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </WorkSurface>
  );
}
