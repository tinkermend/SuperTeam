import type { ApiClientOptions } from "./client";

export type EmployeeCostItem = {
  employee_id: string;
  employee_name: string;
  provider_type: string;
  run_count: number;
  total_tokens: number;
};

export type DailyTrendPoint = {
  day: string;
  provider_type: string;
  run_count: number;
  total_tokens: number;
};

export type CostSummary = {
  period_start: string;
  period_end: string;
  total_tokens: number;
  total_runs: number;
  by_employee: EmployeeCostItem[];
  by_provider: Record<string, number>;
  daily_trend: DailyTrendPoint[];
};

export type EmployeeCostList = {
  period_start: string;
  period_end: string;
  items: EmployeeCostItem[];
};

export type CostPeriod = "7d" | "30d" | "90d";

export async function getCostSummary(
  opts: ApiClientOptions,
  period: CostPeriod = "30d",
): Promise<CostSummary> {
  const fetcher = opts.fetcher ?? fetch;
  const url = `${opts.baseUrl}/api/v1/costs/summary?period=${period}`;
  const res = await fetcher(url, { credentials: "include" });
  if (!res.ok) throw new Error(`getCostSummary: ${res.status}`);
  return res.json() as Promise<CostSummary>;
}

export async function getEmployeeCosts(
  opts: ApiClientOptions,
  period: CostPeriod = "30d",
): Promise<EmployeeCostList> {
  const fetcher = opts.fetcher ?? fetch;
  const url = `${opts.baseUrl}/api/v1/costs/employees?period=${period}`;
  const res = await fetcher(url, { credentials: "include" });
  if (!res.ok) throw new Error(`getEmployeeCosts: ${res.status}`);
  return res.json() as Promise<EmployeeCostList>;
}
