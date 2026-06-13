import Editor from "@monaco-editor/react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileText, Plus, Save } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { LiquidCard, SemanticIconTile, StatusBadge } from "@/components/superteam";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listWorkspaceFiles,
  upsertWorkspaceFile,
  type WorkspaceFile,
} from "@/lib/api/employees";

type InstructionFilesPanelProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
};

type InstructionDraft = {
  content: string;
  isDirty: boolean;
  path: string;
};

export function InstructionFilesPanel({ apiOptions, employeeId }: InstructionFilesPanelProps) {
  const queryClient = useQueryClient();
  const [selectedPath, setSelectedPath] = useState("");
  const [draftPath, setDraftPath] = useState("");
  const [draftContent, setDraftContent] = useState("");
  const [draftsByPath, setDraftsByPath] = useState<Record<string, InstructionDraft>>({});
  const [newFilePath, setNewFilePath] = useState("");
  const [isDirty, setIsDirty] = useState(false);

  const filesQuery = useQuery({
    queryKey: ["employee-workspace-files", employeeId],
    queryFn: () => listWorkspaceFiles(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });

  const files = useMemo(() => filesQuery.data ?? [], [filesQuery.data]);
  const selectedFile = files.find((file) => file.path === selectedPath);
  const localDrafts = Object.entries(draftsByPath)
    .filter(([path, draft]) => !files.some((file) => file.path === path) && draft.path.trim())
    .sort(([left], [right]) => left.localeCompare(right));

  useEffect(() => {
    if (selectedPath) return;

    const defaultFile = files.find((file) => file.path === "AGENTS.md") ?? files[0];
    const nextPath = defaultFile?.path ?? "AGENTS.md";
    const existingDraft = draftsByPath[nextPath];
    setSelectedPath(nextPath);
    setDraftPath(existingDraft?.path ?? nextPath);
    setDraftContent(existingDraft?.content ?? defaultFile?.content ?? "");
    setIsDirty(existingDraft?.isDirty ?? false);
    setDraftsByPath((currentDrafts) => {
      if (currentDrafts[nextPath]) return currentDrafts;

      return {
        ...currentDrafts,
        [nextPath]: {
          content: defaultFile?.content ?? "",
          isDirty: false,
          path: nextPath,
        },
      };
    });
  }, [draftsByPath, files, selectedPath]);

  useEffect(() => {
    if (files.length === 0) return;

    const serverFilesByPath = new Map(files.map((file) => [file.path, file]));
    setDraftsByPath((currentDrafts) => {
      let changed = false;
      const nextDrafts = { ...currentDrafts };

      for (const file of files) {
        const currentDraft = currentDrafts[file.path];
        if (!currentDraft) {
          nextDrafts[file.path] = {
            content: file.content,
            isDirty: false,
            path: file.path,
          };
          changed = true;
          continue;
        }

        if (!currentDraft.isDirty && (currentDraft.content !== file.content || currentDraft.path !== file.path)) {
          nextDrafts[file.path] = {
            content: file.content,
            isDirty: false,
            path: file.path,
          };
          changed = true;
        }
      }

      return changed ? nextDrafts : currentDrafts;
    });

    const selectedServerFile = selectedPath ? serverFilesByPath.get(selectedPath) : undefined;
    if (!selectedServerFile || isDirty) return;

    if (draftPath !== selectedServerFile.path) {
      setDraftPath(selectedServerFile.path);
    }
    if (draftContent !== selectedServerFile.content) {
      setDraftContent(selectedServerFile.content);
    }
  }, [draftContent, draftPath, files, isDirty, selectedPath]);

  const saveFile = useMutation({
    mutationFn: () =>
      upsertWorkspaceFile(apiOptions, employeeId, {
        path: draftPath.trim(),
        content: draftContent,
      }),
    onSuccess: (savedFile) => {
      queryClient.setQueryData<WorkspaceFile[]>(
        ["employee-workspace-files", employeeId],
        (currentFiles = []) => {
          const exists = currentFiles.some((file) => file.path === savedFile.path);
          if (!exists) return [...currentFiles, savedFile].sort((a, b) => a.path.localeCompare(b.path));

          return currentFiles.map((file) => (file.path === savedFile.path ? savedFile : file));
        },
      );
      setSelectedPath(savedFile.path);
      setDraftPath(savedFile.path);
      setDraftContent(savedFile.content);
      setIsDirty(false);
      setDraftsByPath((currentDrafts) => {
        const nextDrafts = { ...currentDrafts };
        if (selectedPath && selectedPath !== savedFile.path) {
          delete nextDrafts[selectedPath];
        }
        nextDrafts[savedFile.path] = {
          content: savedFile.content,
          isDirty: false,
          path: savedFile.path,
        };
        return nextDrafts;
      });
    },
  });

  const canSave = draftPath.trim().length > 0 && isDirty && !saveFile.isPending;

  const handleSelectFile = (file: WorkspaceFile) => {
    const existingDraft = draftsByPath[file.path];
    setSelectedPath(file.path);
    setDraftPath(existingDraft?.path ?? file.path);
    setDraftContent(existingDraft?.content ?? file.content);
    setIsDirty(existingDraft?.isDirty ?? false);
    setDraftsByPath((currentDrafts) => {
      if (currentDrafts[file.path]) return currentDrafts;

      return {
        ...currentDrafts,
        [file.path]: {
          content: file.content,
          isDirty: false,
          path: file.path,
        },
      };
    });
  };

  const handleCreateDraft = () => {
    const path = newFilePath.trim();
    if (!path) return;

    const existingFile = files.find((file) => file.path === path);
    const existingDraft = draftsByPath[path];
    setSelectedPath(path);
    setDraftPath(existingDraft?.path ?? path);
    setDraftContent(existingDraft?.content ?? existingFile?.content ?? "");
    setIsDirty(existingDraft?.isDirty ?? !existingFile);
    setDraftsByPath((currentDrafts) => ({
      ...currentDrafts,
      [path]: existingDraft ?? {
        content: existingFile?.content ?? "",
        isDirty: !existingFile,
        path,
      },
    }));
    setNewFilePath("");
  };

  const updateSelectedDraft = (patch: Partial<InstructionDraft>) => {
    const key = selectedPath || draftPath;
    if (!key) return;

    setDraftsByPath((currentDrafts) => ({
      ...currentDrafts,
      [key]: {
        ...(currentDrafts[key] ?? {
          content: draftContent,
          isDirty,
          path: draftPath,
        }),
        ...patch,
      },
    }));
  };

  return (
    <LiquidCard className="rounded-xl">
      <CardHeader className="gap-3 pb-3">
        <div className="flex min-w-0 items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <SemanticIconTile tone="decision" size="sm">
              <FileText />
            </SemanticIconTile>
            <div className="min-w-0">
              <CardTitle className="truncate text-base">宪法/人格文件</CardTitle>
              <p className="text-xs text-muted-foreground">维护员工个人指令、边界和输出约定。</p>
            </div>
          </div>
          {filesQuery.isFetching ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
        </div>
      </CardHeader>
      <CardContent className="grid gap-4 xl:grid-cols-[260px_minmax(0,1fr)]">
        <aside className="min-w-0 space-y-3">
          <div className="flex flex-col gap-2">
            {filesQuery.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {filesQuery.isError ? <p className="text-sm text-destructive">工作目录文件加载失败</p> : null}
            {!filesQuery.isLoading && !filesQuery.isError && files.length === 0 ? (
              <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无指令文件</p>
            ) : null}
            {files.map((file) => (
              <Button
                className="h-auto justify-start text-left"
                key={file.path}
                onClick={() => handleSelectFile(file)}
                type="button"
                variant={file.path === selectedPath ? "secondary" : "ghost"}
              >
                <FileText data-icon="inline-start" />
                <span className="truncate">{file.path}</span>
              </Button>
            ))}
            {localDrafts.map(([path, draft]) => (
              <Button
                className="h-auto justify-start text-left"
                key={path}
                onClick={() => {
                  setSelectedPath(path);
                  setDraftPath(draft.path);
                  setDraftContent(draft.content);
                  setIsDirty(draft.isDirty);
                }}
                type="button"
                variant={path === selectedPath ? "secondary" : "ghost"}
              >
                <FileText data-icon="inline-start" />
                <span className="truncate">{draft.path}</span>
              </Button>
            ))}
          </div>
          <div className="space-y-2 rounded-md border bg-background/70 p-3">
            <Label htmlFor="new-instruction-path">新文件路径</Label>
            <div className="flex gap-2">
              <Input
                id="new-instruction-path"
                onChange={(event) => setNewFilePath(event.target.value)}
                placeholder="AGENTS.local.md"
                value={newFilePath}
              />
              <Button
                aria-label="新建文件"
                disabled={!newFilePath.trim()}
                onClick={handleCreateDraft}
                size="icon"
                type="button"
                variant="outline"
              >
                <Plus />
              </Button>
            </div>
          </div>
        </aside>

        <section className="min-w-0 space-y-3">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-end">
            <div className="min-w-0 space-y-2">
              <Label htmlFor="instruction-path">文件路径</Label>
              <Input
                id="instruction-path"
                onChange={(event) => {
                  const nextPath = event.target.value;
                  setDraftPath(nextPath);
                  setIsDirty(true);
                  updateSelectedDraft({ isDirty: true, path: nextPath });
                }}
                value={draftPath}
              />
            </div>
            <Button disabled={!canSave} onClick={() => saveFile.mutate()} type="button">
              <Save data-icon="inline-start" />
              保存文件
            </Button>
          </div>

          <div className="overflow-hidden rounded-md border bg-background/80">
            <Editor
              height="420px"
              language={draftPath.endsWith(".md") ? "markdown" : "plaintext"}
              onChange={(value: string | undefined) => {
                const nextContent = value ?? "";
                setDraftContent(nextContent);
                setIsDirty(true);
                updateSelectedDraft({ content: nextContent, isDirty: true });
              }}
              options={{
                ariaLabel: "Workspace file editor",
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                wordWrap: "on",
              }}
              value={draftContent}
            />
          </div>

          <div className="flex min-h-5 flex-wrap items-center gap-2 text-sm">
            {selectedFile ? (
              <span className="text-xs text-muted-foreground">
                {selectedFile.size_bytes} bytes · revision {selectedFile.revision_number}
                {selectedFile.updated_at ? ` · ${selectedFile.updated_at}` : ""}
              </span>
            ) : (
              <span className="text-xs text-muted-foreground">本地新文件，保存后生效。</span>
            )}
            {isDirty ? <StatusBadge tone="warning">未保存</StatusBadge> : null}
            {!isDirty && saveFile.isSuccess ? <StatusBadge tone="success">已保存</StatusBadge> : null}
            {saveFile.isError ? <span className="text-sm text-destructive">保存失败</span> : null}
          </div>
        </section>
      </CardContent>
    </LiquidCard>
  );
}
