import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Save } from "lucide-react";
import { useMemo, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button, notifyError, notifySuccess, SectionHeader, SoftCard } from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  replaceDigitalEmployeeRoles,
  updateDigitalEmployeeProfile,
  type DigitalEmployee,
  type DigitalEmployeeRoleImpact,
  type DigitalEmployeeRoleImpactCasting,
} from "@/lib/api/employees";
import { listRoleVocabulary } from "@/lib/api/casting";
import { firstRoleTitle, RoleKeysPicker } from "../role-keys-picker";
import { sameStringSet, stringArray } from "../config-utils";
import { useDirtyReport, useTabRehydrate } from "./use-config-tab-state";

export function ConfigIdentityTab({
  apiOptions,
  employee,
  onDirtyChange,
}: {
  apiOptions: ApiClientOptions;
  employee: DigitalEmployee;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const currentKeys = employee.role_keys ?? [];
  const [description, setDescription] = useState(employee.description ?? "");
  const [role, setRole] = useState(employee.role ?? "");
  const [selected, setSelected] = useState<string[]>(currentKeys);
  const [pendingImpact, setPendingImpact] = useState<DigitalEmployeeRoleImpact | null>(null);
  const [saving, setSaving] = useState(false);

  const profileDirty =
    description !== (employee.description ?? "") || role !== (employee.role ?? "");
  const rolesDirty = !sameStringSet(selected, employee.role_keys ?? []);
  const dirty = profileDirty || rolesDirty;

  useTabRehydrate(
    employee.id,
    dirty,
    [employee.description, employee.role, employee.role_keys],
    () => {
      setDescription(employee.description ?? "");
      setRole(employee.role ?? "");
      setSelected(employee.role_keys ?? []);
    },
  );
  useDirtyReport(dirty, onDirtyChange);

  const vocabulary = useQuery({
    queryKey: ["role-vocabulary"],
    queryFn: () => listRoleVocabulary(apiOptions),
  });
  const derivedRole = firstRoleTitle(selected, vocabulary.data ?? []);

  const saveRoles = useMutation({
    mutationFn: ({ confirmImpact }: { confirmImpact: boolean }) =>
      replaceDigitalEmployeeRoles(apiOptions, employee.id, selected, confirmImpact),
    onSuccess: (result) => {
      setSelected(result.role_keys ?? []);
      setPendingImpact(null);
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employee.id] });
      queryClient.invalidateQueries({ queryKey: ["digital-employees"] });
    },
    onError: (error) => {
      if (!(error instanceof ApiRequestError) || error.status !== 400) return;
      const payload = error.payload as
        | {
            code?: string;
            affected_castings?: DigitalEmployeeRoleImpactCasting[];
            affected_count?: number;
          }
        | undefined;
      if (payload?.code !== "casting_impact_requires_confirm") return;
      setPendingImpact({
        affected_castings: payload.affected_castings ?? [],
        affected_count: payload.affected_count ?? payload.affected_castings?.length ?? 0,
      });
    },
  });

  const impactDesc = useMemo(() => {
    if (!pendingImpact) return "";
    const lines = pendingImpact.affected_castings.map(
      (row) => `· ${row.project_name} / ${row.template_name} · 角色 ${row.role_key}`,
    );
    return [
      `移除角色将解除以下 ${pendingImpact.affected_count} 条编制，并通知项目负责人：`,
      ...lines,
    ].join("\n");
  }, [pendingImpact]);

  const handleSave = async (confirmImpact = false) => {
    setSaving(true);
    try {
      if (profileDirty && !confirmImpact) {
        try {
          const updated = await updateDigitalEmployeeProfile(apiOptions, employee.id, {
            description: description.trim(),
            role: role.trim(),
          });
          setDescription(updated.description ?? "");
          setRole(updated.role ?? "");
          queryClient.setQueryData(["digital-employee", employee.id], updated);
          queryClient.invalidateQueries({ queryKey: ["digital-employees"] });
          queryClient.invalidateQueries({ queryKey: ["digital-employee-overview"] });
        } catch (error) {
          notifyError(error instanceof Error ? error.message : "保存员工说明 / 职责描述失败");
          return;
        }
      }
      if (rolesDirty || confirmImpact) {
        try {
          await saveRoles.mutateAsync({ confirmImpact });
        } catch (error) {
          if (
            error instanceof ApiRequestError &&
            (error.code === "casting_impact_requires_confirm" ||
              (error.payload as { code?: string } | undefined)?.code ===
                "casting_impact_requires_confirm")
          ) {
            return;
          }
          notifyError(error instanceof Error ? error.message : "保存剧本角色失败");
          return;
        }
      }
      notifySuccess("身份已保存");
    } finally {
      setSaving(false);
    }
  };

  const externalCaps = stringArray(employee.capability_bindings?.external_capabilities);

  return (
    <div className="space-y-4">
      <SectionHeader title="身份" description="保存后即时生效，无需审批" />
      <SoftCard className="space-y-4 p-5">
        <div className="space-y-2">
          <Label htmlFor="employee-profile-description" className="text-sm font-medium text-ink">
            员工说明
          </Label>
          <Textarea
            id="employee-profile-description"
            aria-label="员工说明"
            placeholder="简述这位数字员工负责什么、边界与协作方式，便于列表扫读识别。"
            rows={3}
            value={description}
            onChange={(event) => {
              setDescription(event.target.value);
            }}
          />
          <p className="text-xs text-ink-3">可选。会出现在数字员工卡片上，超出两行以省略号截断。</p>
        </div>
        <div className="space-y-2">
          <Label className="text-sm font-medium text-ink">剧本角色</Label>
          <p className="text-xs text-ink-3">
            一人可兼多角色。保存后即时生效，决定该员工在项目编制 / 扩编候选中出现在哪些角色下。
          </p>
          <RoleKeysPicker
            options={vocabulary.data ?? []}
            selected={selected}
            loading={vocabulary.isPending}
            error={vocabulary.isError}
            onChange={(next) => {
              setSelected(next);
            }}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="employee-profile-role" className="text-sm font-medium text-ink">
            职责描述
          </Label>
          <Input
            id="employee-profile-role"
            aria-label="职责描述"
            value={role}
            placeholder={derivedRole || "未填时用所选剧本角色名称"}
            onChange={(event) => {
              setRole(event.target.value);
            }}
          />
          <p className="text-xs text-ink-3">仅用于列表展示，不参与编制。未填时用所选剧本角色名称。</p>
        </div>
        <div className="rounded-inner border border-line bg-card-soft p-3">
          <p className="text-xs font-semibold text-ink-2">已声明能力（参考）</p>
          {externalCaps.length ? (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {externalCaps.map((cap) => (
                <span
                  key={cap}
                  className="rounded-inner border border-line bg-card px-2 py-0.5 font-mono text-xs text-ink-2"
                >
                  {cap}
                </span>
              ))}
            </div>
          ) : (
            <p className="mt-1 text-xs text-ink-3">未声明外部能力</p>
          )}
          <p className="mt-2 text-xs text-ink-3">
            实际绑定请到「能力」Tab；声明用于选角佐证，请到「执行配置」编辑。
          </p>
        </div>
        <div className="flex justify-end">
          <Button
            type="button"
            disabled={!dirty || saving || saveRoles.isPending}
            onClick={() => void handleSave(false)}
          >
            <Save />
            保存
          </Button>
        </div>
      </SoftCard>
      <ConfirmDialog
        open={pendingImpact !== null}
        onOpenChange={(open) => {
          if (!open) setPendingImpact(null);
        }}
        title="确认解除受影响编制"
        desc={impactDesc}
        confirmText="确认并保存"
        destructive
        isLoading={saveRoles.isPending || saving}
        handleConfirm={() => void handleSave(true)}
      />
    </div>
  );
}
