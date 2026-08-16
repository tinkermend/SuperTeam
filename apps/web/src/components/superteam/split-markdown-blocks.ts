/**
 * 流式 Markdown 块切分，对齐 desktop-cc-gui `splitMarkdownBlocks`：
 * 未闭合 fence 吞到文末、永不冻结；尾部 2 块每帧重排。
 */

export type MarkdownBlock = {
  key: string;
  text: string;
  frozen: boolean;
};

export const UNSTABLE_TAIL_BLOCKS = 2;

type FenceState = {
  marker: "`" | "~";
  length: number;
};

function matchFenceOpen(line: string): FenceState | null {
  const match = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line);
  if (!match) {
    return null;
  }
  const run = match[1] ?? "";
  const info = match[2] ?? "";
  if (run.startsWith("`") && info.includes("`")) {
    return null;
  }
  return { marker: run.startsWith("`") ? "`" : "~", length: run.length };
}

function matchFenceClose(line: string, fence: FenceState): boolean {
  const withoutIndent = line.replace(/^ {0,3}/, "");
  let runLength = 0;
  while (withoutIndent[runLength] === fence.marker) {
    runLength += 1;
  }
  if (runLength < fence.length) {
    return false;
  }
  return withoutIndent.slice(runLength).trim() === "";
}

export function splitMarkdownBlocks(source: string): MarkdownBlock[] {
  if (!source.trim()) {
    return [];
  }
  const starts: number[] = [];
  let offset = 0;
  let blockOpen = false;
  let fence: FenceState | null = null;

  for (const line of source.split("\n")) {
    if (fence) {
      if (matchFenceClose(line, fence)) {
        fence = null;
        blockOpen = false;
      }
    } else if (line.trim() === "") {
      blockOpen = false;
    } else {
      if (!blockOpen) {
        starts.push(offset);
        blockOpen = true;
      }
      const openedFence = matchFenceOpen(line);
      if (openedFence) {
        fence = openedFence;
      }
    }
    offset += line.length + 1;
  }

  const unstableFrom = Math.max(0, starts.length - UNSTABLE_TAIL_BLOCKS);
  return starts.map((start, index) => ({
    frozen: index < unstableFrom,
    key: String(start),
    text: source.slice(start, index + 1 < starts.length ? starts[index + 1] : source.length),
  }));
}
