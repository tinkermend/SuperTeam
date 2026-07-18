import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileCheck2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import {
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  type V3Tone,
} from "@/components/superteam";
import { Textarea } from "@/components/ui/textarea";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  getDemandAcceptanceCriteria,
  signDemandCriterionVerdict,
  type DemandAcceptanceCriterionDetail,
} from "@/lib/api/projects";

const ACCEPTANCE_PENDING = "acceptance_pending";

function verdictPill(verdict: DemandAcceptanceCriterionDetail["verdict"]): {
  tone: V3Tone;
  label: string;
} {
  switch (verdict) {
    case "satisfied":
      return { label: "已满足", tone: "ok" };
    case "unsatisfied":
      return { label: "未满足", tone: "danger" };
    case "not_applicable":
      return { label: "不适用", tone: "mute" };
    case "pending":
      // review_gate 完成时占位:检测器出结论前保持 HOLD。
      return { label: "检测中", tone: "info" };
    case "escalate_human":
      // 对抗评审判官预算耗尽:显式升级到人类决断,不是"还没判"。
      return { label: "升级人类", tone: "warn" };
    default:
      return { label: "待判定", tone: "mute" };
  }
}

function judgeLabel(judgeType: DemandAcceptanceCriterionDetail["judge_type"]): string | null {
  switch (judgeType) {
    case "human":
      return "负责人判定";
    case "executor":
      return "员工判定";
    case "adversarial":
      return "对抗评审判定";
    case "review_gate":
      return "检测门判定";
    default:
      return null;
  }
}

function canSignCriterion(
  demandStatus: string,
  criterion: DemandAcceptanceCriterionDetail,
): boolean {
  return (
    demandStatus === ACCEPTANCE_PENDING &&
    criterion.verification_method === "human_judgment" &&
    criterion.severity === "blocking" &&
    // 已有人类判定即视为已签署（人类判定优先于员工判定）。
    criterion.judge_type !== "human"
  );
}

type SignHandler = (
  criterionId: string,
  verdict: "satisfied" | "unsatisfied",
  reason: string,
) => void;

function CriterionRow({
  criterion,
  demandStatus,
  onSign,
  isSigning,
}: {
  criterion: DemandAcceptanceCriterionDetail;
  demandStatus: string;
  onSign?: SignHandler;
  isSigning?: boolean;
}) {
  const [reason, setReason] = useState("");
  const { tone, label } = verdictPill(criterion.verdict);
  const judge = judgeLabel(criterion.judge_type);
  const showControls = Boolean(onSign) && canSignCriterion(demandStatus, criterion);

  return (
    <div className="grid gap-2.5 p-3" data-testid={`criterion-row-${criterion.criterion_id}`}>
      <div className="flex flex-wrap items-center gap-1.5">
        <StatusPill
          showDot={false}
          tone={criterion.verification_method === "human_judgment" ? "info" : "mute"}
        >
          {criterion.verification_method === "human_judgment" ? "人类判定" : "自动验证"}
        </StatusPill>
        {criterion.severity === "non_blocking" ? (
          <StatusPill showDot={false} tone="mute">
            非阻断
          </StatusPill>
        ) : null}
        <StatusPill tone={tone}>{label}</StatusPill>
        {judge ? (
          <StatusPill showDot={false} tone="mute">
            {judge}
          </StatusPill>
        ) : null}
      </div>

      <p className="text-sm leading-relaxed text-v3-ink">{criterion.statement}</p>

      {criterion.evidence_refs.length > 0 ? (
        <div className="flex flex-wrap gap-1.5" data-testid={`criterion-evidence-${criterion.criterion_id}`}>
          {criterion.evidence_refs.map((ref, index) => (
            <span
              className="inline-flex items-center gap-1 rounded-v3-pill border border-v3-line bg-v3-surface px-2 py-0.5 font-mono text-[11px] text-v3-ink-2"
              key={`${ref}-${index}`}
              title={ref}
            >
              <span className="text-v3-ink-3">
                {ref.startsWith("attestation:") ? "存证" : "证据"}
              </span>
              <span className="max-w-[220px] truncate">{ref}</span>
            </span>
          ))}
        </div>
      ) : null}

      {criterion.task_summaries.length > 0 ? (
        <details className="rounded-v3-inner border border-v3-line bg-v3-surface">
          <summary className="cursor-pointer px-3 py-2 text-xs font-medium text-v3-ink-2">
            查看满足任务产出（{criterion.task_summaries.length}）
          </summary>
          <div className="divide-y divide-v3-line border-t border-v3-line">
            {criterion.task_summaries.map((summary, index) => (
              <div className="grid gap-1 px-3 py-2" key={`${summary.task_id}-${index}`}>
                <span className="font-mono text-[11px] text-v3-ink-3">{summary.task_id}</span>
                <p className="text-xs leading-relaxed text-v3-ink-2">
                  {summary.summary.trim() ? summary.summary : "该任务尚无执行结论"}
                </p>
              </div>
            ))}
          </div>
        </details>
      ) : null}

      {showControls ? (
        <div className="grid gap-2 rounded-v3-inner border border-v3-line bg-v3-surface p-2.5">
          <Textarea
            className="min-h-16 text-sm"
            data-testid={`criterion-reason-${criterion.criterion_id}`}
            onChange={(event) => setReason(event.target.value)}
            placeholder="签署理由（可选）——请先核对上方产出再判定"
            value={reason}
          />
          <div className="flex flex-wrap gap-2">
            <V3Button
              data-testid={`criterion-sign-satisfied-${criterion.criterion_id}`}
              disabled={isSigning}
              onClick={() => onSign?.(criterion.criterion_id, "satisfied", reason.trim())}
              size="sm"
              variant="primary"
            >
              满足
            </V3Button>
            <V3Button
              data-testid={`criterion-sign-unsatisfied-${criterion.criterion_id}`}
              disabled={isSigning}
              onClick={() => onSign?.(criterion.criterion_id, "unsatisfied", reason.trim())}
              size="sm"
              variant="danger"
            >
              不满足
            </V3Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}

export type CriteriaPanelViewProps = {
  criteria: DemandAcceptanceCriterionDetail[];
  demandStatus: string;
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
  onSign?: SignHandler;
  isSigning?: boolean;
};

export function CriteriaPanelView({
  criteria,
  demandStatus,
  isLoading,
  isError,
  onRetry,
  onSign,
  isSigning,
}: CriteriaPanelViewProps) {
  return (
    <SoftCard className="grid gap-3 p-4">
      <div className="flex items-center gap-2">
        <FileCheck2 className="size-4 text-v3-ink-2" />
        <h3 className="text-sm font-semibold text-v3-ink">验收判据血缘</h3>
        {demandStatus === ACCEPTANCE_PENDING ? (
          <StatusPill tone="warn">待验收</StatusPill>
        ) : null}
      </div>

      {isError ? (
        <V3ErrorState description="无法加载验收判据" onRetry={onRetry} />
      ) : isLoading ? (
        <V3LoadingState />
      ) : criteria.length === 0 ? (
        <V3EmptyState title="本需求未声明验收判据" />
      ) : (
        <div className="divide-y divide-v3-line rounded-v3-inner border border-v3-line">
          {criteria.map((criterion) => (
            <CriterionRow
              criterion={criterion}
              demandStatus={demandStatus}
              isSigning={isSigning}
              key={criterion.criterion_id}
              onSign={onSign}
            />
          ))}
        </div>
      )}
    </SoftCard>
  );
}

export type CriteriaPanelProps = {
  apiOptions: ApiClientOptions;
  apiBaseUrl: string;
  demandId: string;
};

export function CriteriaPanel({ apiOptions, apiBaseUrl, demandId }: CriteriaPanelProps) {
  const queryClient = useQueryClient();
  const criteriaQuery = useQuery({
    enabled: Boolean(demandId),
    queryFn: () => getDemandAcceptanceCriteria(apiOptions, demandId),
    queryKey: ["demand-acceptance-criteria", apiBaseUrl, demandId],
    refetchInterval: 5000,
  });

  const signMutation = useMutation({
    mutationFn: (input: {
      criterionId: string;
      verdict: "satisfied" | "unsatisfied";
      reason: string;
    }) =>
      signDemandCriterionVerdict(apiOptions, demandId, {
        criterion_id: input.criterionId,
        reason: input.reason || undefined,
        verdict: input.verdict,
      }),
    onError: (error) => {
      const message =
        error instanceof ApiRequestError && error.status === 409
          ? "该判据已被签署或需求已收敛，请刷新后重试"
          : "签署失败，请重试";
      toast.error(message);
    },
    onSuccess: () => {
      toast.success("已记录判定");
      void queryClient.invalidateQueries({
        queryKey: ["demand-acceptance-criteria", apiBaseUrl, demandId],
      });
      // 需求可能因本次签署收敛为已完成/失败，刷新详情头卡与河道列表的状态徽标。
      void queryClient.invalidateQueries({ queryKey: ["workflow-detail", apiBaseUrl, demandId] });
      void queryClient.invalidateQueries({ queryKey: ["workflow-instances", apiBaseUrl] });
    },
  });

  const data = criteriaQuery.data;

  return (
    <CriteriaPanelView
      criteria={data?.criteria ?? []}
      demandStatus={data?.demand_status ?? ""}
      isError={criteriaQuery.isError}
      isLoading={criteriaQuery.isLoading}
      isSigning={signMutation.isPending}
      onRetry={() => void criteriaQuery.refetch()}
      onSign={(criterionId, verdict, reason) =>
        signMutation.mutate({ criterionId, reason, verdict })
      }
    />
  );
}
