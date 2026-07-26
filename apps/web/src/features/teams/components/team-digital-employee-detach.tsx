import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowLeftRight, LogOut } from "lucide-react";
import { Button, LoadingState } from "@/components/superteam";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import type { ApiClientOptions } from "@/lib/api/client";
import { ApiRequestError } from "@/lib/api/client";
import type { DigitalEmployee } from "@/lib/api/employees";
import { reassignDigitalEmployeeTeam } from "@/lib/api/employees";
import { listTeamSummaries, unbindTeamDigitalEmployee } from "@/lib/api/teams";

// 归属可逆：收编有了逆操作（移出回候岗大厅）和横向操作（换队）。两者的服务端守卫
// 一致（在役执行 / 仍被非归档项目引用即 409），错误 message 由后端给出，此处直接
// 展示，不在前端另拼一套文案。
const DETACH_CONSEQUENCE =
  "移出后该员工进入候岗大厅，失去本团队的技能与 MCP 继承；无团队归属的员工不能参与项目。下次派发的工作目录会按新归属重算。";

const TRANSFER_CONSEQUENCE =
  "换队后该员工的技能与 MCP 继承切换到目标团队，下次派发的工作目录按新归属重算（provider 会话连续性重置）。";

function errorText(error: unknown, fallback: string) {
  if (error instanceof ApiRequestError && error.detail) {
    return error.detail;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

type DetachActionsProps = {
  apiOptions: ApiClientOptions;
  employee: DigitalEmployee;
  onChanged: () => void;
  teamId: string;
};

export function TeamDigitalEmployeeDetachActions({
  apiOptions,
  employee,
  onChanged,
  teamId
}: DetachActionsProps) {
  const [unbindOpen, setUnbindOpen] = useState(false);
  const [transferOpen, setTransferOpen] = useState(false);

  return (
    <>
      <Button
        aria-label={`移出 ${employee.name}`}
        onClick={() => setUnbindOpen(true)}
        size="sm"
        type="button"
        variant="ghost"
      >
        <LogOut data-icon="inline-start" className="size-3.5" />
        移出
      </Button>
      <Button
        aria-label={`换队 ${employee.name}`}
        onClick={() => setTransferOpen(true)}
        size="sm"
        type="button"
        variant="ghost"
      >
        <ArrowLeftRight data-icon="inline-start" className="size-3.5" />
        换队
      </Button>

      <UnbindDialog
        apiOptions={apiOptions}
        employee={employee}
        onChanged={onChanged}
        onOpenChange={setUnbindOpen}
        open={unbindOpen}
        teamId={teamId}
      />
      <TransferDialog
        apiOptions={apiOptions}
        employee={employee}
        onChanged={onChanged}
        onOpenChange={setTransferOpen}
        open={transferOpen}
        teamId={teamId}
      />
    </>
  );
}

function UnbindDialog({
  apiOptions,
  employee,
  onChanged,
  onOpenChange,
  open,
  teamId
}: DetachActionsProps & { onOpenChange: (open: boolean) => void; open: boolean }) {
  const unbindMutation = useMutation({
    mutationFn: () => unbindTeamDigitalEmployee(apiOptions, teamId, employee.id),
    onSuccess: () => {
      onOpenChange(false);
      onChanged();
    }
  });

  useEffect(() => {
    if (open) {
      unbindMutation.reset();
    }
    // 只在开合时重置错误，不随 mutation 身份变化重跑。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>确认移出数字员工</AlertDialogTitle>
          <AlertDialogDescription>
            将「{employee.name}」移出本团队。{DETACH_CONSEQUENCE}
          </AlertDialogDescription>
        </AlertDialogHeader>
        {unbindMutation.isError ? (
          <p className="text-[13px] text-danger">
            {errorText(unbindMutation.error, "移出失败，请重试")}
          </p>
        ) : null}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={unbindMutation.isPending}>取消</AlertDialogCancel>
          <AlertDialogAction
            disabled={unbindMutation.isPending}
            onClick={(event) => {
              // 守卫命中要留在弹窗里显示原因，不能让 Radix 自动关闭。
              event.preventDefault();
              unbindMutation.mutate();
            }}
          >
            确认移出
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function TransferDialog({
  apiOptions,
  employee,
  onChanged,
  onOpenChange,
  open,
  teamId
}: DetachActionsProps & { onOpenChange: (open: boolean) => void; open: boolean }) {
  const [targetTeamId, setTargetTeamId] = useState("");

  const teams = useQuery({
    enabled: open,
    queryKey: ["team-summaries", "transfer-targets"],
    queryFn: () => listTeamSummaries(apiOptions)
  });

  const transferMutation = useMutation({
    mutationFn: (nextTeamId: string) =>
      reassignDigitalEmployeeTeam(apiOptions, employee.id, nextTeamId),
    onSuccess: () => {
      onOpenChange(false);
      onChanged();
    }
  });

  useEffect(() => {
    if (open) {
      setTargetTeamId("");
      transferMutation.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const candidates = (teams.data ?? []).filter(
    (team) => team.id !== teamId && team.status === "active",
  );

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>把数字员工换到其他团队</AlertDialogTitle>
          <AlertDialogDescription>
            将「{employee.name}」从本团队转到目标团队。{TRANSFER_CONSEQUENCE}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="flex flex-col gap-2">
          <Label htmlFor="transfer-target-team">目标团队</Label>
          {teams.isLoading ? (
            <LoadingState label="加载可选团队" />
          ) : (
            <Select
              disabled={transferMutation.isPending || candidates.length === 0}
              onValueChange={setTargetTeamId}
              value={targetTeamId}
            >
              <SelectTrigger aria-label="目标团队" className="w-full" id="transfer-target-team">
                <SelectValue placeholder="选择目标团队" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {candidates.map((team) => (
                    <SelectItem key={team.id} value={team.id}>
                      {team.name}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          )}
          {!teams.isLoading && candidates.length === 0 ? (
            <p className="text-[13px] text-ink-2">没有其他可选团队。</p>
          ) : null}
          {teams.isError ? (
            <p className="text-[13px] text-danger">团队列表加载失败。</p>
          ) : null}
          {transferMutation.isError ? (
            <p className="text-[13px] text-danger">
              {errorText(transferMutation.error, "换队失败，请重试")}
            </p>
          ) : null}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={transferMutation.isPending}>取消</AlertDialogCancel>
          <AlertDialogAction
            disabled={!targetTeamId || transferMutation.isPending}
            onClick={(event) => {
              event.preventDefault();
              if (targetTeamId) {
                transferMutation.mutate(targetTeamId);
              }
            }}
          >
            确认换队
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
