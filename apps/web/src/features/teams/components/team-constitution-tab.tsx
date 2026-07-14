import { Save, ShieldCheck } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { MasterDetailLayout, V3Button } from "@/components/superteam";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
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
    <MasterDetailLayout
      narrowDetail="stack"
      rail="md"
      master={
        <Card>
          <CardHeader>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <CardTitle className="text-base">团队宪法</CardTitle>
              <Badge variant="secondary">硬性规则</Badge>
            </div>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <ShieldCheck className="size-4" />
                <Label htmlFor="team-constitution-hard-rules">团队宪法</Label>
              </div>
              <Textarea
                disabled={!canEdit}
                id="team-constitution-hard-rules"
                onChange={(event) => setHardRulesText(event.target.value)}
                rows={8}
                value={hardRulesText}
              />
            </div>
          </CardContent>
        </Card>
      }
      detail={
        <Card>
          <CardHeader>
            <CardTitle className="text-base">保存</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 text-sm">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">当前内容</span>
              <Badge variant="outline">{hardRuleCount} 条硬性规则</Badge>
            </div>
            <V3Button disabled={!canEdit || saveMutation.isPending} onClick={() => saveMutation.mutate()}>
              <Save data-icon="inline-start" />
              保存宪法
            </V3Button>
            {saveMutation.isSuccess ? <p className="text-muted-foreground">团队宪法已保存。</p> : null}
            {saveMutation.isError ? <p className="text-destructive">团队宪法保存失败。</p> : null}
          </CardContent>
        </Card>
      }
    />
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
