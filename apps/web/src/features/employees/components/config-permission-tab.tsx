import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Send, ShieldCheck } from "lucide-react";
import { useState } from "react";
import {
  Button,
  Callout,
  LoadingState,
  notifyError,
  notifySuccess,
  RelativeTime,
  SectionHeader,
  SoftCard,
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  getEmployeePermissionChange,
  submitEmployeePermissionChange,
  type DigitalEmployee,
  type SubmitPermissionChangeInput,
} from "@/lib/api/employees";
import { ChipsEditor } from "./config-chips-editor";
import { PermissionChangeDiff } from "./permission-change-diff";
import { sameStringArray, stringArray, submitPermissionErrorMessage } from "../config-utils";
import { useDirtyReport, useTabRehydrate } from "./use-config-tab-state";

export function ConfigPermissionTab({
  apiOptions,
  employee,
  onDirtyChange,
}: {
  apiOptions: ApiClientOptions;
  employee: DigitalEmployee;
  onDirtyChange: (dirty: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const permissionPolicy = employee.permission_policy ?? {};
  const currentGrants = stringArray(permissionPolicy.grants);
  const currentAllowedActions = stringArray(
    (permissionPolicy as Record<string, unknown>).allowed_actions,
  );

  const [grants, setGrants] = useState<string[]>(currentGrants);
  const [allowedActions, setAllowedActions] = useState<string[]>(currentAllowedActions);

  const dirty =
    !sameStringArray(grants, stringArray((employee.permission_policy ?? {}).grants)) ||
    !sameStringArray(
      allowedActions,
      stringArray(((employee.permission_policy ?? {}) as Record<string, unknown>).allowed_actions),
    );

  useTabRehydrate(employee.id, dirty, [employee.permission_policy], () => {
    setGrants(stringArray((employee.permission_policy ?? {}).grants));
    setAllowedActions(
      stringArray(((employee.permission_policy ?? {}) as Record<string, unknown>).allowed_actions),
    );
  });
  useDirtyReport(dirty, onDirtyChange);

  const pending = useQuery({
    queryKey: ["employee-permission-change", employee.id],
    queryFn: () => getEmployeePermissionChange(apiOptions, employee.id),
  });

  const submitChange = useMutation({
    mutationFn: (input: SubmitPermissionChangeInput) =>
      submitEmployeePermissionChange(apiOptions, employee.id, input),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["digital-employee", employee.id] });
      queryClient.invalidateQueries({ queryKey: ["employee-permission-change", employee.id] });
      notifySuccess("已提交，待权限中心审批");
    },
    onError: (error) => {
      notifyError(submitPermissionErrorMessage(error));
    },
  });

  const handleSubmit = (event: React.FormEvent) => {
    event.preventDefault();
    const policyChanged =
      !sameStringArray(grants, currentGrants) ||
      !sameStringArray(allowedActions, currentAllowedActions);
    if (!policyChanged) return;
    const input: SubmitPermissionChangeInput = {
      permission_policy: {
        ...permissionPolicy,
        grants,
        allowed_actions: allowedActions,
      },
    };
    submitChange.mutate(input);
  };

  const hasPending = Boolean(pending.data);

  return (
    <form className="space-y-4" onSubmit={handleSubmit}>
      <SectionHeader title="权限" description="变更需权限中心审批，批准后生效" />
      {pending.isLoading ? <LoadingState /> : null}
      {pending.isError ? (
        <Callout tone="danger">
          无法加载待审批状态
          <button type="button" className="ml-2 underline" onClick={() => void pending.refetch()}>
            重试
          </button>
        </Callout>
      ) : null}
      {pending.data ? (
        <Callout
          tone="warn"
          title={`有 1 条待审批的权限变更 · 由 ${pending.data.approver_name} 审批`}
          description={
            <span>
              <RelativeTime value={pending.data.created_at} />
              <Link className="ml-2 underline" to="/permissions">
                去权限中心
              </Link>
            </span>
          }
        >
          <div className="mt-3">
            <PermissionChangeDiff
              current={pending.data.current_permission_policy}
              target={pending.data.target_permission_policy}
              currentRole={pending.data.current_role}
              targetRole={pending.data.target_role}
            />
          </div>
        </Callout>
      ) : null}

      <SoftCard className="space-y-4 p-5">
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-4 text-ink-2" />
          <span className="text-sm font-medium text-ink">资源与动作</span>
        </div>
        <ChipsEditor
          label="资源授权 · scope:resource 形式"
          placeholder="例如 database.read:dev_db，回车添加"
          values={grants}
          onChange={(next) => {
            setGrants(next);
          }}
        />
        <ChipsEditor
          label="动作白名单 · 员工可执行动作上限，留空不收敛"
          placeholder="例如 code.write，回车添加"
          values={allowedActions}
          onChange={(next) => {
            setAllowedActions(next);
          }}
        />
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
          {hasPending ? (
            <p className="text-xs text-ink-3">已有待审批变更，批准或驳回后再提交新的变更。</p>
          ) : (
            <p className="text-xs text-ink-3">
              提交后由团队审批人在权限中心批准后写回生效；员工有进行中工作时会被拒绝提交。
            </p>
          )}
          <Button
            className="shrink-0 self-end"
            type="submit"
            disabled={!dirty || submitChange.isPending || hasPending}
          >
            <Send />
            提交审批
          </Button>
        </div>
      </SoftCard>
    </form>
  );
}
