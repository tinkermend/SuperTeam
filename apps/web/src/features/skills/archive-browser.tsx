import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FileText, Folder } from "lucide-react";
import { EmptyState, ErrorState } from "@/components/superteam";
import {
  getSkillArchiveContent,
  listSkillArchiveEntries,
  type SkillArchiveEntry,
} from "@/lib/api/skills";
import { skillArchiveErrorMessage, skillArchivePreviewTitle } from "./archive-errors";

export function SkillArchiveBrowser({
  apiBaseUrl,
  fetcher,
  skillId,
}: {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  skillId: string;
}) {
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const entries = useQuery({
    queryKey: ["skill-archive", skillId, "entries"],
    queryFn: () => listSkillArchiveEntries(apiOptions, skillId),
  });
  const files = (entries.data ?? []).filter((item) => item.kind === "file");
  const defaultPath = files.find((item) => item.path === "SKILL.md")?.path ?? files[0]?.path;
  const [selectedPath, setSelectedPath] = useState<string>();
  useEffect(() => {
    setSelectedPath(defaultPath);
  }, [defaultPath, skillId]);
  const activePath = selectedPath ?? defaultPath;
  const selected = files.find((item) => item.path === activePath);
  const content = useQuery({
    queryKey: ["skill-archive", skillId, "content", activePath],
    queryFn: () => getSkillArchiveContent(apiOptions, skillId, activePath ?? ""),
    enabled: Boolean(activePath) && Boolean(selected?.previewable),
  });

  if (entries.isError) {
    return (
      <ErrorState
        title={skillArchivePreviewTitle(entries.error)}
        description={skillArchiveErrorMessage(entries.error)}
      />
    );
  }
  if (!entries.isPending && files.length === 0) {
    return <EmptyState title="包内没有可展示的文件" />;
  }

  return (
    <div className="grid min-w-0 gap-3 lg:grid-cols-[minmax(12rem,16rem)_minmax(0,1fr)]">
      <ul className="max-h-80 space-y-1 overflow-auto rounded-inner bg-card-inner p-2 text-[13px]">
        {(entries.data ?? []).map((item) => (
          <ArchiveTreeItem
            item={item}
            key={item.path}
            selected={item.path === activePath}
            onSelect={() => setSelectedPath(item.path)}
          />
        ))}
      </ul>
      <div className="min-w-0 rounded-inner bg-card-inner p-3">
        {!selected ? (
          <p className="text-sm text-ink-3">选择左侧文件以预览</p>
        ) : !selected.previewable ? (
          <p className="text-sm text-ink-2">
            {selected.path} · {selected.content_type} · {selected.size_bytes} 字节，二进制不可预览。
          </p>
        ) : content.isError ? (
          <ErrorState title="无法读取文件" description={skillArchiveErrorMessage(content.error)} />
        ) : (
          <>
            {content.data?.truncated ? (
              <p className="mb-2 text-[12px] font-semibold text-warn">已截断，完整内容请在本地查看</p>
            ) : null}
            <pre className="max-h-80 overflow-auto whitespace-pre-wrap break-all text-[12px] text-ink">
              {content.data?.content ?? (content.isPending ? "加载中…" : "")}
            </pre>
          </>
        )}
      </div>
    </div>
  );
}

function ArchiveTreeItem({
  item,
  onSelect,
  selected,
}: {
  item: SkillArchiveEntry;
  onSelect: () => void;
  selected: boolean;
}) {
  if (item.kind === "directory") {
    return (
      <li className="flex items-center gap-1.5 px-2 py-1 text-ink-3">
        <Folder className="size-3.5" />
        {item.path}
      </li>
    );
  }
  return (
    <li>
      <button
        className={`flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-left ${selected ? "bg-brand-soft font-semibold text-ink" : "text-ink-2 hover:bg-card"}`}
        onClick={onSelect}
        type="button"
      >
        <FileText className="size-3.5" />
        {item.path}
      </button>
    </li>
  );
}
