import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import {
  Button,
  SoftDialog,
  SoftDialogBody,
  SoftDialogContent,
  SoftDialogDescription,
  SoftDialogFooter,
  SoftDialogHeader,
  SoftDialogTitle,
  notifySuccess,
} from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { replaceSkillArchive, type Skill } from "@/lib/api/skills";
import { skillArchiveErrorMessage } from "./archive-errors";

type SkillReplaceDialogProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  skill?: Skill;
};

export function SkillReplaceDialog({
  apiBaseUrl,
  fetcher,
  onOpenChange,
  open,
  skill,
}: SkillReplaceDialogProps) {
  const queryClient = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const replace = useMutation({
    mutationFn: () => {
      if (!skill || !file) {
        throw new Error("请选择技能 zip 包");
      }
      return replaceSkillArchive({ baseUrl: apiBaseUrl, fetcher }, skill.id, { file });
    },
    onSuccess: (updated) => {
      const shortChecksum = updated.archive_checksum_sha256.slice(0, 8);
      notifySuccess(
        updated.version !== skill?.version
          ? `已更新到 ${updated.version}`
          : `已替换归档，checksum ${shortChecksum}`,
      );
      void queryClient.invalidateQueries({ queryKey: ["skills"] });
      void queryClient.invalidateQueries({ queryKey: ["skill", updated.id] });
      void queryClient.invalidateQueries({ queryKey: ["skill-archive", updated.id] });
      setFile(null);
      onOpenChange(false);
    },
  });

  const bindingCount =
    (skill?.team_bindings.length ?? 0) +
    (skill?.agent_bindings.length ?? 0) +
    (skill?.project_bindings?.length ?? 0);

  return (
    <SoftDialog
      onOpenChange={(next) => {
        if (!next) {
          setFile(null);
          replace.reset();
        }
        onOpenChange(next);
      }}
      open={open}
    >
      <SoftDialogContent size="md">
        <SoftDialogHeader>
          <SoftDialogTitle>更新技能包</SoftDialogTitle>
          <SoftDialogDescription>
            绑定会保留，slug 不会改变。下次任务才会按新 checksum 物化到 Runtime。
          </SoftDialogDescription>
        </SoftDialogHeader>
        <SoftDialogBody className="space-y-3 text-sm text-ink-2">
          {skill ? (
            <div className="rounded-inner bg-card-inner p-3 text-[13px]">
              <p className="font-semibold text-ink">{skill.name}</p>
              <p className="mt-1 font-mono text-[12px] text-ink-3">{skill.slug}</p>
              <p className="mt-2">
                当前版本 {skill.version} · checksum {skill.archive_checksum_sha256.slice(0, 12)}
              </p>
              <p className="mt-1">当前绑定 {bindingCount} 项（团队 / 员工 / 项目）</p>
            </div>
          ) : null}
          <Input
            accept=".zip,application/zip"
            aria-label="技能 zip 包"
            onChange={(event) => {
              setFile(event.target.files?.[0] ?? null);
            }}
            type="file"
          />
          {replace.isError ? (
            <p className="font-semibold text-danger">{skillArchiveErrorMessage(replace.error)}</p>
          ) : null}
        </SoftDialogBody>
        <SoftDialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button" variant="ghost">
            关闭
          </Button>
          <Button
            disabled={!file || replace.isPending || !skill}
            onClick={() => replace.mutate()}
            type="button"
          >
            <RefreshCw data-icon="inline-start" />
            {replace.isPending ? "更新中…" : "上传并替换"}
          </Button>
        </SoftDialogFooter>
      </SoftDialogContent>
    </SoftDialog>
  );
}
