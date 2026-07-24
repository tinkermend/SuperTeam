import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Eye, FileCheck2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import {
  SoftCard,
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  type Tone
} from "@/components/superteam";
import { Textarea } from "@/components/ui/textarea";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  getDemandAcceptanceCriteria,
  signDemandCriterionVerdict,
  type DemandAcceptanceCriterionDetail,
  type DemandCriterionDeliverable
} from "@/lib/api/projects";
import {
  ArtifactPreviewSheet,
  artifactContentHref,
  artifactPreviewKind,
  type PreviewableArtifact
} from "@/features/projects/components/artifact-preview-sheet";

const ACCEPTANCE_PENDING = "acceptance_pending";

function verdictPill(verdict: DemandAcceptanceCriterionDetail["verdict"]): {
  tone: Tone;
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

/** Unsigned blocking human_judgment criteria that still gate acceptance. */
export function unsignedBlockingHumanCriteria(
  demandStatus: string,
  criteria: DemandAcceptanceCriterionDetail[],
): DemandAcceptanceCriterionDetail[] {
  if (demandStatus !== ACCEPTANCE_PENDING) {
    return [];
  }
  return criteria.filter(
    (criterion) =>
      criterion.verification_method === "human_judgment" &&
      criterion.severity === "blocking" &&
      criterion.judge_type !== "human",
  );
}

type FinalAcceptanceHandler = (
  verdict: "satisfied" | "unsatisfied",
  reason: string,
  criterionIds: string[],
  options?: { alsoCloseProject?: boolean },
) => void;

function deliverableToPreviewable(
  deliverable: DemandCriterionDeliverable,
): PreviewableArtifact {
  return {
    id: deliverable.artifact_ref_id,
    title: deliverable.title,
    content_type: deliverable.content_type
};
}

function DeliverableChips({
  deliverables,
  onPreview
}: {
  deliverables: DemandCriterionDeliverable[];
  onPreview: (artifact: PreviewableArtifact) => void;
}) {
  if (deliverables.length === 0) {
    return null;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {deliverables.map((deliverable) => {
        const previewable = deliverableToPreviewable(deliverable);
        const canPreview = artifactPreviewKind(previewable) != null;
        return (
          <span
            className="inline-flex items-center gap-1.5 rounded-full border border-line bg-card px-2 py-0.5 text-[11px] text-ink-2"
            key={deliverable.artifact_ref_id}
          >
            <span className="max-w-[180px] truncate font-medium text-ink" title={deliverable.title}>
              {deliverable.title}
            </span>
            {canPreview ? (
              <button
                className="inline-flex items-center gap-0.5 text-brand hover:underline"
                onClick={() => onPreview(previewable)}
                type="button"
              >
                <Eye aria-hidden className="size-3" />
                预览
              </button>
            ) : null}
            <a
              className="inline-flex items-center gap-0.5 text-brand hover:underline"
              href={artifactContentHref(deliverable.artifact_ref_id)}
              rel="noreferrer"
              target="_blank"
            >
              <Download aria-hidden className="size-3" />
              下载
            </a>
          </span>
        );
      })}
    </div>
  );
}

function CriterionRow({
  criterion,
  onPreview
}: {
  criterion: DemandAcceptanceCriterionDetail;
  onPreview: (artifact: PreviewableArtifact) => void;
}) {
  const { tone, label } = verdictPill(criterion.verdict);
  const judge = judgeLabel(criterion.judge_type);

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

      <p className="text-sm leading-relaxed text-ink">{criterion.statement}</p>

      {criterion.evidence_refs.length > 0 ? (
        <div className="flex flex-wrap gap-1.5" data-testid={`criterion-evidence-${criterion.criterion_id}`}>
          {criterion.evidence_refs.map((ref, index) => (
            <span
              className="inline-flex items-center gap-1 rounded-full border border-line bg-card px-2 py-0.5 font-mono text-[11px] text-ink-2"
              key={`${ref}-${index}`}
              title={ref}
            >
              <span className="text-ink-3">
                {ref.startsWith("attestation:") ? "存证" : "证据"}
              </span>
              <span className="max-w-[220px] truncate">{ref}</span>
            </span>
          ))}
        </div>
      ) : null}

      {criterion.task_summaries.length > 0 ? (
        <details className="rounded-inner border border-line bg-card">
          <summary className="cursor-pointer px-3 py-2 text-xs font-medium text-ink-2">
            查看满足任务产出（{criterion.task_summaries.length}）
          </summary>
          <div className="divide-y divide-line border-t border-line">
            {criterion.task_summaries.map((summary, index) => (
              <div className="grid gap-1.5 px-3 py-2" key={`${summary.task_id}-${index}`}>
                <span className="font-mono text-[11px] text-ink-3">{summary.task_id}</span>
                <p className="text-xs leading-relaxed text-ink-2">
                  {summary.summary.trim() ? summary.summary : "该任务尚无执行结论"}
                </p>
                <DeliverableChips
                  deliverables={summary.deliverables ?? []}
                  onPreview={onPreview}
                />
              </div>
            ))}
          </div>
        </details>
      ) : null}
    </div>
  );
}

function FinalAcceptanceGate({
  pendingCriteria,
  onAccept,
  isSigning
}: {
  pendingCriteria: DemandAcceptanceCriterionDetail[];
  onAccept?: FinalAcceptanceHandler;
  isSigning?: boolean;
}) {
  const [reason, setReason] = useState("");
  // §5.3「通过并结项」默认不勾选：勾选后签署请求带 also_close_project=true。
  const [alsoCloseProject, setAlsoCloseProject] = useState(false);
  if (!onAccept || pendingCriteria.length === 0) {
    return null;
  }
  const criterionIds = pendingCriteria.map((c) => c.criterion_id);

  return (
    <div
      className="grid gap-2.5 rounded-inner border border-warn/40 bg-warn-soft/40 p-3"
      data-testid="final-acceptance-gate"
    >
      <div className="grid gap-1">
        <p className="text-sm font-semibold text-ink">最终验收</p>
        <p className="text-xs leading-5 text-ink-2">
          确认交付是否符合需求意图。一次通过即完成本次人类守门；不通过将使本需求失败。
        </p>
      </div>
      <ul className="grid gap-1 text-xs text-ink-2">
        {pendingCriteria.map((criterion) => (
          <li key={criterion.criterion_id} data-testid={`final-acceptance-item-${criterion.criterion_id}`}>
            · {criterion.statement}
          </li>
        ))}
      </ul>
      <Textarea
        className="min-h-16 text-sm"
        data-testid="final-acceptance-reason"
        onChange={(event) => setReason(event.target.value)}
        placeholder="签署理由——不通过时必填；通过时可选"
        value={reason}
      />
      <label className="flex cursor-pointer items-start gap-2 text-xs leading-5 text-ink-2">
        <input
          checked={alsoCloseProject}
          className="mt-0.5 size-3.5 shrink-0 accent-[var(--brand)]"
          data-testid="final-acceptance-also-close"
          onChange={(event) => setAlsoCloseProject(event.target.checked)}
          type="checkbox"
        />
        <span>
          通过并结项
          <span className="mt-0.5 block text-[11px] text-ink-3">
            勾选后，若项目全部需求已终态，将直接归档，不再产生结项确认卡（默认不勾选）
          </span>
        </span>
      </label>
      <div className="flex flex-wrap gap-2">
        <Button
          data-testid="final-acceptance-pass"
          disabled={isSigning}
          onClick={() =>
            onAccept("satisfied", reason.trim(), criterionIds, {
              alsoCloseProject
})
          }
          size="sm"
          variant="primary"
        >
          {alsoCloseProject ? "通过并结项" : "通过"}
        </Button>
        <Button
          data-testid="final-acceptance-reject"
          disabled={isSigning || !reason.trim()}
          onClick={() => onAccept("unsatisfied", reason.trim(), criterionIds)}
          size="sm"
          variant="danger"
        >
          不通过
        </Button>
      </div>
    </div>
  );
}

export type CriteriaPanelViewProps = {
  criteria: DemandAcceptanceCriterionDetail[];
  demandStatus: string;
  isLoading?: boolean;
  isError?: boolean;
  onRetry?: () => void;
  onFinalAccept?: FinalAcceptanceHandler;
  isSigning?: boolean;
};

export function CriteriaPanelView({
  criteria,
  demandStatus,
  isLoading,
  isError,
  onRetry,
  onFinalAccept,
  isSigning
}: CriteriaPanelViewProps) {
  const [previewArtifact, setPreviewArtifact] =
    useState<PreviewableArtifact | null>(null);
  const pendingHuman = unsignedBlockingHumanCriteria(demandStatus, criteria);

  return (
    <SoftCard className="grid gap-3 p-4">
      <div className="flex items-center gap-2">
        <FileCheck2 className="size-4 text-ink-2" />
        <h3 className="text-sm font-semibold text-ink">验收判据血缘</h3>
        {demandStatus === ACCEPTANCE_PENDING ? (
          <StatusPill tone="warn">待验收</StatusPill>
        ) : null}
      </div>

      {isError ? (
        <ErrorState description="无法加载验收判据" onRetry={onRetry} />
      ) : isLoading ? (
        <LoadingState />
      ) : criteria.length === 0 ? (
        <EmptyState title="本需求未声明验收判据" />
      ) : (
        <>
          <FinalAcceptanceGate
            isSigning={isSigning}
            onAccept={onFinalAccept}
            pendingCriteria={pendingHuman}
          />
          <div className="divide-y divide-line rounded-inner border border-line">
            {criteria.map((criterion) => (
              <CriterionRow
                criterion={criterion}
                key={criterion.criterion_id}
                onPreview={setPreviewArtifact}
              />
            ))}
          </div>
        </>
      )}

      <ArtifactPreviewSheet
        artifact={previewArtifact}
        onClose={() => setPreviewArtifact(null)}
      />
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
    refetchInterval: 5000
});

  const signMutation = useMutation({
    mutationFn: async (input: {
      verdict: "satisfied" | "unsatisfied";
      reason: string;
      criterionIds: string[];
      alsoCloseProject?: boolean;
    }) => {
      if (input.criterionIds.length === 0) {
        return;
      }
      if (input.verdict === "unsatisfied") {
        // 一条 unsatisfied 即失败整 demand；其余未签不再要求。
        await signDemandCriterionVerdict(apiOptions, demandId, {
          criterion_id: input.criterionIds[0]!,
          reason: input.reason || undefined,
          verdict: "unsatisfied"
});
        return;
      }
      const lastIndex = input.criterionIds.length - 1;
      for (let i = 0; i < input.criterionIds.length; i += 1) {
        const criterionId = input.criterionIds[i]!;
        // §5.3：also_close_project 只挂在最后一条签署上，避免中途半签就尝试结项。
        await signDemandCriterionVerdict(apiOptions, demandId, {
          also_close_project:
            Boolean(input.alsoCloseProject) && i === lastIndex ? true : undefined,
          criterion_id: criterionId,
          reason: input.reason || undefined,
          verdict: "satisfied"
});
      }
    },
    onError: (error) => {
      const message =
        error instanceof ApiRequestError && error.status === 409
          ? "该判据已被签署或需求已收敛，请刷新后重试"
          : "签署失败，请重试";
      toast.error(message);
    },
    onSuccess: () => {
      toast.success("已记录最终验收");
      void queryClient.invalidateQueries({
        queryKey: ["demand-acceptance-criteria", apiBaseUrl, demandId]
});
      // 需求可能因本次签署收敛为已完成/失败，刷新详情头卡与河道列表的状态徽标。
      void queryClient.invalidateQueries({ queryKey: ["workflow-detail", apiBaseUrl, demandId] });
      void queryClient.invalidateQueries({ queryKey: ["workflow-instances", apiBaseUrl] });
      void queryClient.invalidateQueries({ queryKey: ["inbox"] });
      void queryClient.invalidateQueries({ queryKey: ["projects"] });
    }
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
      onFinalAccept={(verdict, reason, criterionIds, options) =>
        signMutation.mutate({
          alsoCloseProject: options?.alsoCloseProject,
          criterionIds,
          reason,
          verdict
})
      }
    />
  );
}
