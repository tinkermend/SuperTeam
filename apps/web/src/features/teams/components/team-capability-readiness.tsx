import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import {
  Button,
  DataTable,
  ErrorState,
  LoadingState,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import { Input } from "@/components/ui/input";
import type { ApiClientOptions } from "@/lib/api/client";
import { ApiRequestError } from "@/lib/api/client";
import {
  getTeamCapabilityReadiness,
  type TeamMcpReadinessEntry
} from "@/lib/api/capabilities";
import { upsertEmployeeEnvironmentVariable } from "@/lib/api/employees";

function errorText(error: unknown, fallback: string) {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

type ReadinessCell = {
  employeeId: string;
  employeeName: string;
  missing: string[];
};

type ReadinessRow = {
  serverId: string;
  serverKey: string;
  serverName: string;
  requiredEnvVars: string[];
  cells: ReadinessCell[];
};

function groupByServer(entries: TeamMcpReadinessEntry[]): ReadinessRow[] {
  const rows = new Map<string, ReadinessRow>();
  for (const entry of entries) {
    const row = rows.get(entry.mcp_server_id) ?? {
      serverId: entry.mcp_server_id,
      serverKey: entry.server_key,
      serverName: entry.server_name,
      requiredEnvVars: entry.required_env_vars ?? [],
      cells: []
    };
    row.cells.push({
      employeeId: entry.digital_employee_id,
      employeeName: entry.employee_name,
      missing: entry.missing_env_vars ?? []
    });
    rows.set(entry.mcp_server_id, row);
  }
  return [...rows.values()];
}

function readinessTone(ready: number, total: number): Tone {
  if (total === 0 || ready === total) return "ok";
  if (ready === 0) return "danger";
  return "warn";
}

function readinessLabel(ready: number, total: number) {
  if (total === 0) return "无成员";
  if (ready === total) return "全员就绪";
  if (ready === 0) return `未就绪 0/${total}`;
  return `部分就绪 ${ready}/${total}`;
}

/**
 * 成员就绪矩阵：团队 MCP × 成员，点名谁还缺哪个环境变量，并允许就地补值。
 *
 * 为什么需要它：变量名由注册表/团队绑定定义，**值只存在员工级**，所以团队绑一个需要
 * 凭据的 MCP 后，在每名成员各自配好 env 之前它对全队都是空壳。此前团队页完全看不到
 * 这件事（后端注释原话："Team-level env-var preflight is advisory"）。
 */
export function TeamCapabilityReadiness({
  apiOptions,
  teamId
}: {
  apiOptions: ApiClientOptions;
  teamId: string;
}) {
  const queryClient = useQueryClient();
  const [drafts, setDrafts] = useState<Record<string, string>>({});

  const readiness = useQuery({
    queryKey: ["team-capability-readiness", teamId],
    queryFn: () => getTeamCapabilityReadiness(apiOptions, teamId),
    placeholderData: keepPreviousData
  });

  const fillMutation = useMutation({
    mutationFn: (input: { employeeId: string; name: string; value: string }) =>
      upsertEmployeeEnvironmentVariable(apiOptions, input.employeeId, input.name, {
        value: input.value,
        sensitive: true
      }),
    onSuccess: async (_result, input) => {
      setDrafts((current) => ({ ...current, [`${input.employeeId}:${input.name}`]: "" }));
      await queryClient.invalidateQueries({ queryKey: ["team-capability-readiness", teamId] });
    }
  });

  const rows = useMemo(
    () => groupByServer(readiness.data?.mcp_readiness ?? []),
    [readiness.data],
  );

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
        <div className="min-w-0">
          <h2 className="text-base font-bold text-ink">成员就绪矩阵</h2>
          <p className="mt-1 text-[13px] text-ink-2">
            团队绑定的 MCP 需要的环境变量，值按成员各自配置；缺值的成员用不了该能力，可在此就地补齐。
          </p>
        </div>
        {readiness.isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
      </div>
      <div className="p-4">
        {readiness.isLoading ? <LoadingState label="加载就绪矩阵" /> : null}
        {readiness.isError ? <ErrorState title="就绪矩阵加载失败" /> : null}
        {fillMutation.isError ? (
          <p className="pb-2 text-[13px] text-danger">
            {errorText(fillMutation.error, "环境变量写入失败，请重试")}
          </p>
        ) : null}
        {!readiness.isLoading && !readiness.isError && rows.length === 0 ? (
          <p className="py-2 text-[13px] text-ink-2">团队暂无 MCP 绑定，或团队暂无数字员工。</p>
        ) : null}

        <div className="flex flex-col gap-4">
          {rows.map((row) => {
            const ready = row.cells.filter((cell) => cell.missing.length === 0).length;
            const blocked = row.cells.filter((cell) => cell.missing.length > 0);
            return (
              <div key={row.serverId} className="flex flex-col gap-2">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-medium text-ink">{row.serverName}</span>
                  <span className="font-mono text-xs text-ink-3">{row.serverKey}</span>
                  <StatusPill tone={readinessTone(ready, row.cells.length)}>
                    {readinessLabel(ready, row.cells.length)}
                  </StatusPill>
                  {row.requiredEnvVars.length === 0 ? (
                    <span className="text-xs text-ink-3">无需环境变量</span>
                  ) : (
                    <span className="flex flex-wrap items-center gap-1 text-xs text-ink-3">
                      <KeyRound className="size-3" />
                      需要 {row.requiredEnvVars.join("、")}
                    </span>
                  )}
                </div>
                {blocked.length > 0 ? (
                  <DataTable aria-label={`${row.serverName} 未就绪成员`}>
                    <thead>
                      <tr>
                        <Th>成员</Th>
                        <Th>缺少变量</Th>
                        <Th className="min-w-[280px]">就地补值</Th>
                      </tr>
                    </thead>
                    <tbody>
                      {blocked.map((cell) => {
                        const name = cell.missing[0];
                        const draftKey = `${cell.employeeId}:${name}`;
                        const draft = drafts[draftKey] ?? "";
                        return (
                          <Tr key={cell.employeeId}>
                            <Td className="font-medium text-ink">{cell.employeeName}</Td>
                            <Td>
                              <span className="flex flex-wrap gap-1">
                                {cell.missing.map((missing) => (
                                  <span
                                    key={missing}
                                    className="rounded border border-danger/40 bg-danger-soft px-1.5 py-0.5 font-mono text-[11px] text-danger-text"
                                  >
                                    {missing}
                                  </span>
                                ))}
                              </span>
                            </Td>
                            <Td>
                              <span className="flex items-center gap-2">
                                <Input
                                  aria-label={`为 ${cell.employeeName} 设置 ${name}`}
                                  className="h-8"
                                  onChange={(event) =>
                                    setDrafts((current) => ({
                                      ...current,
                                      [draftKey]: event.target.value
                                    }))
                                  }
                                  placeholder={name}
                                  type="password"
                                  value={draft}
                                />
                                <Button
                                  disabled={!draft || fillMutation.isPending}
                                  onClick={() =>
                                    fillMutation.mutate({
                                      employeeId: cell.employeeId,
                                      name,
                                      value: draft
                                    })
                                  }
                                  size="sm"
                                  type="button"
                                  variant="outline"
                                >
                                  保存
                                </Button>
                              </span>
                            </Td>
                          </Tr>
                        );
                      })}
                    </tbody>
                  </DataTable>
                ) : null}
              </div>
            );
          })}
        </div>
      </div>
    </WorkSurface>
  );
}
