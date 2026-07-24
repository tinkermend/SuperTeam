import { Save, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { StatusPill, V3Button, WorkSurface } from "@/components/superteam";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  updateTeamConstitution,
  type UpdateTeamConstitutionInput,
} from "@/lib/api/teams";

type TeamConstitutionTabProps = {
  apiOptions: ApiClientOptions;
  canEdit: boolean;
  constitution?: Record<string, unknown>;
  onSaved?: () => void;
  teamId: string;
};

export function TeamConstitutionTab({
  apiOptions,
  canEdit,
  constitution,
  onSaved,
  teamId,
}: TeamConstitutionTabProps) {
  const [hardRulesText, setHardRulesText] = useState(() => arrayText(constitution?.hard_rules));

  useEffect(() => {
    setHardRulesText(arrayText(constitution?.hard_rules));
  }, [constitution]);

  const constitutionInput = useMemo<UpdateTeamConstitutionInput>(
    () => ({
      ...(constitution ?? {}),
      hard_rules: lineList(hardRulesText),
    }),
    [constitution, hardRulesText],
  );

  const saveMutation = useMutation({
    mutationFn: () => updateTeamConstitution(apiOptions, teamId, constitutionInput),
    onSuccess: () => {
      onSaved?.();
    },
  });

  const hardRuleCount = lineList(hardRulesText).length;

  return (
    <WorkSurface>
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-v3-line px-5 py-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="text-base font-bold text-v3-ink">团队宪法</h2>
            <StatusPill tone="mute">{hardRuleCount} 条硬性规则</StatusPill>
          </div>
          <p className="mt-1 text-[13px] text-v3-ink-2">约束执行边界的硬性规则，一行一条。</p>
        </div>
        <V3Button
          disabled={!canEdit || saveMutation.isPending}
          onClick={() => saveMutation.mutate()}
          size="sm"
        >
          <Save data-icon="inline-start" />
          保存宪法
        </V3Button>
      </div>
      <div className="flex flex-col gap-3 p-5">
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-4 text-v3-ink-3" />
          <Label htmlFor="team-constitution-hard-rules">团队宪法</Label>
        </div>
        <Textarea
          disabled={!canEdit}
          id="team-constitution-hard-rules"
          onChange={(event) => setHardRulesText(event.target.value)}
          placeholder={"例如：\n生产写操作必须审批\n变更窗口必须登记"}
          rows={6}
          value={hardRulesText}
        />
        {saveMutation.isSuccess ? (
          <p className="text-[13px] text-v3-ink-2">团队宪法已保存。</p>
        ) : null}
        {saveMutation.isError ? (
          <p className="text-[13px] text-v3-danger">团队宪法保存失败。</p>
        ) : null}
      </div>
    </WorkSurface>
  );
}

function lineList(value: string): string[] {
  return value
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
}

function arrayText(value: unknown): string {
  if (!Array.isArray(value)) {
    return "";
  }
  return value.filter((item): item is string => typeof item === "string").join("\n");
}
