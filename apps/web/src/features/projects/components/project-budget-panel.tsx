import { BadgeDollarSign } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
} from "@/components/superteam";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type {
  ProjectBudgetLedgerEntry,
  ProjectBudgetSummary,
} from "@/lib/api/projects";

type ProjectBudgetPanelProps = {
  budgetLedger?: ProjectBudgetLedgerEntry[];
  budgetSummary?: ProjectBudgetSummary;
};

export function ProjectBudgetPanel({
  budgetLedger = [],
  budgetSummary,
}: ProjectBudgetPanelProps) {
  const summary = budgetSummary ?? {
    actual_cost: "0",
    actual_tokens: 0,
    estimated_cost: "0",
    estimated_tokens: 0,
    ledger_count: budgetLedger.length,
  };

  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center justify-between gap-3 border-b p-4">
        <div className="flex min-w-0 items-center gap-3">
          <SemanticIconTile tone="warning" size="sm">
            <BadgeDollarSign />
          </SemanticIconTile>
          <div className="min-w-0">
            <h3 className="font-semibold">预算流水</h3>
            <p className="truncate text-xs text-muted-foreground">
              Token、成本估算与实际消耗
            </p>
          </div>
        </div>
        <StatusBadge tone="warning">{summary.ledger_count} 条</StatusBadge>
      </div>

      <div className="grid gap-4 p-4">
        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <MetricBlock label="预估 Token" value={formatNumber(summary.estimated_tokens)} />
          <MetricBlock label="实际 Token" value={formatNumber(summary.actual_tokens)} />
          <MetricBlock label="预估成本" value={formatCost(summary.estimated_cost)} />
          <MetricBlock label="实际成本" value={formatCost(summary.actual_cost)} />
          <MetricBlock label="流水数" value={formatNumber(summary.ledger_count)} />
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>类型</TableHead>
              <TableHead>来源</TableHead>
              <TableHead>Token</TableHead>
              <TableHead>成本</TableHead>
              <TableHead className="min-w-[180px]">原因</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {budgetLedger.length === 0 ? (
              <TableRow>
                <TableCell
                  className="h-24 text-center text-sm text-muted-foreground"
                  colSpan={5}
                >
                  暂无预算流水
                </TableCell>
              </TableRow>
            ) : (
              budgetLedger.map((entry) => (
                <TableRow key={entry.id}>
                  <TableCell>
                    <StatusBadge tone="neutral">{entry.cost_type}</StatusBadge>
                  </TableCell>
                  <TableCell>{entry.source}</TableCell>
                  <TableCell>
                    <span className="font-mono text-xs">
                      {formatOptionalNumber(entry.estimated_tokens)} /{" "}
                      {formatOptionalNumber(entry.actual_tokens)}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="font-mono text-xs">
                      {formatCost(entry.estimated_cost)} / {formatCost(entry.actual_cost)}
                    </span>
                  </TableCell>
                  <TableCell className="max-w-[260px] whitespace-normal">
                    <span className="line-clamp-2 text-sm">
                      {entry.reason || "未记录原因"}
                    </span>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </LiquidCard>
  );
}

function MetricBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-lg border bg-white/55 p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-2 truncate font-mono text-sm font-semibold">{value}</p>
    </div>
  );
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatOptionalNumber(value?: number) {
  return typeof value === "number" ? formatNumber(value) : "-";
}

function formatCost(value: string) {
  return value ? `¥${value}` : "¥0";
}
