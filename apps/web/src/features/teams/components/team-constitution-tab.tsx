import { useEffect, useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Plus, Save, Trash2 } from "lucide-react";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { ApiClientOptions } from "@/lib/api/client";
import { ApiRequestError } from "@/lib/api/client";
import type {
  TeamConstitutionCategory,
  TeamConstitutionRule
} from "@/lib/api/teams";
import {
  listTeamConstitutionRevisions,
  rollbackTeamConstitution,
  saveTeamConstitution
} from "@/lib/api/teams";
import { constitutionCategoryLabel } from "@/lib/status-labels";
import { formatDateTime, formatRelativeTime } from "@/lib/format-time";

const CATEGORIES: TeamConstitutionCategory[] = ["forbid", "must", "require_approval"];

function categoryTone(category: TeamConstitutionCategory): Tone {
  if (category === "forbid") return "danger";
  if (category === "require_approval") return "warn";
  return "info";
}

function errorText(error: unknown, fallback: string) {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

type DraftRule = TeamConstitutionRule & { key: string };

function draftFromSnapshot(constitution?: Record<string, unknown>): DraftRule[] {
  const rules = Array.isArray(constitution?.rules) ? (constitution?.rules as unknown[]) : undefined;
  if (rules) {
    return rules
      .filter((item): item is Record<string, unknown> => typeof item === "object" && item !== null)
      .map((item, index) => ({
        key: String(item.id ?? index),
        id: typeof item.id === "string" ? item.id : undefined,
        text: typeof item.text === "string" ? item.text : "",
        category: (typeof item.category === "string"
          ? item.category
          : "must") as TeamConstitutionCategory
      }))
      .filter((rule) => rule.text.trim().length > 0);
  }
  // 旧快照只有 hard_rules（纯文本数组），按「必须」回退，保证老团队也能编辑。
  const legacy = Array.isArray(constitution?.hard_rules)
    ? (constitution?.hard_rules as unknown[])
    : [];
  return legacy
    .filter((item): item is string => typeof item === "string" && item.trim().length > 0)
    .map((text, index) => ({ key: `legacy-${index}`, text, category: "must" as const }));
}

type TeamConstitutionTabProps = {
  apiOptions: ApiClientOptions;
  canEdit: boolean;
  constitution?: Record<string, unknown>;
  onSaved?: () => void;
  teamId: string;
};

/**
 * 团队宪法编辑（spec §5.3，D1 接通 / D9 仅文本注入）。
 *
 * 规则条目取代此前的裸 textarea：分类让人一眼看出这条提醒的轻重，也决定注入 provider
 * 提示词时的分组。分类标签故意不用"禁止/必须/需审批"——宪法不触发任何门禁或审批，
 * 那样的措辞会让人误以为它有实际强制力。保存必须写变更说明并追加为新版本——宪法一次
 * 改动对全队所有派发生效。
 */
export function TeamConstitutionTab({
  apiOptions,
  canEdit,
  constitution,
  onSaved,
  teamId
}: TeamConstitutionTabProps) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<DraftRule[]>(() => draftFromSnapshot(constitution));
  const [changeNote, setChangeNote] = useState("");
  const [bulkText, setBulkText] = useState("");
  const [previewOpen, setPreviewOpen] = useState(false);
  const [hydrated, setHydrated] = useState(false);

  useEffect(() => {
    if (hydrated) return;
    setDraft(draftFromSnapshot(constitution));
    setHydrated(true);
  }, [constitution, hydrated]);

  const revisions = useQuery({
    queryKey: ["team-constitution-revisions", teamId],
    queryFn: () => listTeamConstitutionRevisions(apiOptions, teamId, { limit: 20 }),
    placeholderData: keepPreviousData
  });

  const saveMutation = useMutation({
    mutationFn: () =>
      saveTeamConstitution(apiOptions, teamId, {
        rules: draft
          .filter((rule) => rule.text.trim().length > 0)
          .map((rule) => ({ id: rule.id, text: rule.text.trim(), category: rule.category })),
        change_note: changeNote.trim()
      }),
    onSuccess: async () => {
      setPreviewOpen(false);
      setChangeNote("");
      await queryClient.invalidateQueries({ queryKey: ["team-constitution-revisions", teamId] });
      onSaved?.();
    }
  });

  const rollbackMutation = useMutation({
    mutationFn: (revisionNumber: number) =>
      rollbackTeamConstitution(apiOptions, teamId, revisionNumber),
    onSuccess: async (revision) => {
      setDraft(
        revision.rules.map((rule, index) => ({
          key: rule.id ?? `rollback-${index}`,
          id: rule.id,
          text: rule.text,
          category: rule.category
        })),
      );
      await queryClient.invalidateQueries({ queryKey: ["team-constitution-revisions", teamId] });
      onSaved?.();
    }
  });

  const original = useMemo(() => draftFromSnapshot(constitution), [constitution]);
  const diff = useMemo(() => {
    const before = new Map(original.map((rule) => [rule.text.trim(), rule.category]));
    const after = new Map(
      draft.filter((rule) => rule.text.trim()).map((rule) => [rule.text.trim(), rule.category]),
    );
    const added = [...after.keys()].filter((text) => !before.has(text));
    const removed = [...before.keys()].filter((text) => !after.has(text));
    const recategorized = [...after.entries()]
      .filter(([text, category]) => before.has(text) && before.get(text) !== category)
      .map(([text, category]) => ({ text, from: before.get(text)!, to: category }));
    return { added, removed, recategorized };
  }, [draft, original]);

  const dirty =
    diff.added.length > 0 || diff.removed.length > 0 || diff.recategorized.length > 0;
  const totalChars = draft.reduce((sum, rule) => sum + rule.text.trim().length, 0);

  const applyBulk = () => {
    const lines = bulkText
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean);
    if (lines.length === 0) return;
    setDraft((current) => {
      const existing = new Set(current.map((rule) => rule.text.trim()));
      const appended = lines
        .filter((line) => !existing.has(line))
        .map((line, index) => ({
          key: `bulk-${Date.now()}-${index}`,
          text: line,
          category: "must" as const
        }));
      return [...current, ...appended];
    });
    setBulkText("");
  };

  return (
    <div className="flex flex-col gap-4">
      <WorkSurface>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-base font-bold text-ink">团队宪法</h2>
              <StatusPill tone="mute">{draft.length} 条规则</StatusPill>
              <StatusPill tone="mute">{totalChars} 字</StatusPill>
            </div>
            <p className="mt-1 text-[13px] text-ink-2">
              规则会在每次派发时注入数字员工的提示词，团队约束在员工人格之前。这是提示词层面的提醒，不是强制门禁——数字员工仍可能不遵守。改动保存为新版本，需填写变更说明。
            </p>
          </div>
          <Button
            disabled={!canEdit || !dirty || saveMutation.isPending}
            onClick={() => setPreviewOpen(true)}
            size="sm"
          >
            <Save data-icon="inline-start" />
            预览并保存
          </Button>
        </div>
        <div className="flex flex-col gap-4 p-5">
          {draft.length === 0 ? (
            <p className="text-[13px] text-ink-2">尚未配置规则。</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {draft.map((rule, index) => (
                <li
                  key={rule.key}
                  className="flex flex-col gap-2 rounded-[14px] border border-line bg-card-soft/60 px-3 py-2.5 md:flex-row md:items-center"
                >
                  <Select
                    disabled={!canEdit}
                    onValueChange={(value) =>
                      setDraft((current) =>
                        current.map((item, itemIndex) =>
                          itemIndex === index
                            ? { ...item, category: value as TeamConstitutionCategory }
                            : item,
                        ),
                      )
                    }
                    value={rule.category}
                  >
                    <SelectTrigger
                      aria-label={`第 ${index + 1} 条规则分类`}
                      className="w-full md:w-32"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {CATEGORIES.map((category) => (
                          <SelectItem key={category} value={category}>
                            {constitutionCategoryLabel(category)}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <Input
                    aria-label={`第 ${index + 1} 条规则`}
                    disabled={!canEdit}
                    onChange={(event) =>
                      setDraft((current) =>
                        current.map((item, itemIndex) =>
                          itemIndex === index ? { ...item, text: event.target.value } : item,
                        ),
                      )
                    }
                    value={rule.text}
                  />
                  {canEdit ? (
                    <Button
                      aria-label={`删除第 ${index + 1} 条规则`}
                      onClick={() =>
                        setDraft((current) => current.filter((_, itemIndex) => itemIndex !== index))
                      }
                      size="icon"
                      type="button"
                      variant="ghost"
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  ) : null}
                </li>
              ))}
            </ul>
          )}

          {canEdit ? (
            <div className="flex flex-col gap-3 border-t border-line pt-4">
              <Button
                className="self-start"
                onClick={() =>
                  setDraft((current) => [
                    ...current,
                    { key: `new-${Date.now()}`, text: "", category: "must" }
                  ])
                }
                size="sm"
                type="button"
                variant="outline"
              >
                <Plus data-icon="inline-start" />
                添加规则
              </Button>
              <div className="flex flex-col gap-2">
                <Label htmlFor="constitution-bulk">批量粘贴（一行一条，按「必须」归类）</Label>
                <Textarea
                  id="constitution-bulk"
                  onChange={(event) => setBulkText(event.target.value)}
                  placeholder={"生产写操作必须审批\n变更窗口必须登记"}
                  rows={3}
                  value={bulkText}
                />
                <Button
                  className="self-start"
                  disabled={!bulkText.trim()}
                  onClick={applyBulk}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  追加到规则列表
                </Button>
              </div>
            </div>
          ) : null}

          {saveMutation.isError ? (
            <p className="text-[13px] text-danger">
              {errorText(saveMutation.error, "宪法保存失败，请重试")}
            </p>
          ) : null}
          {rollbackMutation.isError ? (
            <p className="text-[13px] text-danger">
              {errorText(rollbackMutation.error, "回滚失败，请重试")}
            </p>
          ) : null}
        </div>
      </WorkSurface>

      <WorkSurface>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
          <div className="flex items-center gap-2">
            <History className="size-4 text-ink-3" />
            <h2 className="text-base font-bold text-ink">版本历史</h2>
          </div>
          {revisions.isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
        </div>
        <div className="p-4">
          {revisions.isLoading ? <LoadingState label="加载版本历史" /> : null}
          {revisions.isError ? <ErrorState title="版本历史加载失败" /> : null}
          {!revisions.isLoading && !revisions.isError && (revisions.data?.length ?? 0) === 0 ? (
            <p className="py-2 text-[13px] text-ink-2">暂无版本记录</p>
          ) : null}
          {(revisions.data?.length ?? 0) > 0 ? (
            <DataTable aria-label="宪法版本历史">
              <thead>
                <tr>
                  <Th>版本</Th>
                  <Th>变更说明</Th>
                  <Th>规则数</Th>
                  <Th>时间</Th>
                  <Th className="text-right">操作</Th>
                </tr>
              </thead>
              <tbody>
                {(revisions.data ?? []).map((revision, index) => (
                  <Tr key={revision.id}>
                    <Td className="tabular-nums font-medium text-ink">v{revision.revision_number}</Td>
                    <Td>{revision.change_note || "-"}</Td>
                    <Td className="tabular-nums">{revision.rules?.length ?? 0}</Td>
                    <Td className="whitespace-nowrap tabular-nums">
                      <span title={revision.created_at ? formatDateTime(revision.created_at) : undefined}>
                        {revision.created_at ? formatRelativeTime(revision.created_at) : "-"}
                      </span>
                    </Td>
                    <Td className="text-right">
                      {canEdit && index > 0 ? (
                        <Button
                          disabled={rollbackMutation.isPending}
                          onClick={() => rollbackMutation.mutate(revision.revision_number)}
                          size="sm"
                          type="button"
                          variant="ghost"
                        >
                          回滚到此版本
                        </Button>
                      ) : index === 0 ? (
                        <StatusPill tone="ok">当前生效</StatusPill>
                      ) : null}
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </DataTable>
          ) : null}
        </div>
      </WorkSurface>

      <AlertDialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认宪法变更</AlertDialogTitle>
            <AlertDialogDescription>
              保存后立即对本团队所有数字员工的下次派发生效，并记为一个新版本。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5 rounded-[12px] border border-line bg-card-soft/60 px-3 py-2.5">
              {diff.added.map((text) => (
                <p key={`add-${text}`} className="text-xs text-ok-text">
                  + {text}
                </p>
              ))}
              {diff.removed.map((text) => (
                <p key={`del-${text}`} className="text-xs text-danger">
                  − {text}
                </p>
              ))}
              {diff.recategorized.map((item) => (
                <p key={`cat-${item.text}`} className="text-xs text-warn-text">
                  ~ {item.text}（{constitutionCategoryLabel(item.from)} →{" "}
                  {constitutionCategoryLabel(item.to)}）
                </p>
              ))}
              {!dirty ? <p className="text-xs text-ink-2">没有变更</p> : null}
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="constitution-change-note">变更说明</Label>
              <Textarea
                id="constitution-change-note"
                onChange={(event) => setChangeNote(event.target.value)}
                placeholder="说明为什么改这条规则，便于事后追溯"
                rows={2}
                value={changeNote}
              />
            </div>
            {saveMutation.isError ? (
              <p className="text-[13px] text-danger">
                {errorText(saveMutation.error, "宪法保存失败，请重试")}
              </p>
            ) : null}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={saveMutation.isPending}>取消</AlertDialogCancel>
            <AlertDialogAction
              disabled={!changeNote.trim() || saveMutation.isPending}
              onClick={(event) => {
                event.preventDefault();
                saveMutation.mutate();
              }}
            >
              保存为新版本
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

export function constitutionCategoryTone(category: TeamConstitutionCategory) {
  return categoryTone(category);
}
