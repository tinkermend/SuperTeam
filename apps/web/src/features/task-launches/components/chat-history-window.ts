/** 表现层窗口。desktop-cc-gui 默认 150 条时间线行；这里一轮是完整问答，取 30。 */
export const CHAT_HISTORY_WINDOW_SIZE = 30;

export function sliceChatHistoryWindow<T>(
  entries: T[],
  extraRevealed: number,
): { hidden: number; visible: T[] } {
  const windowSize = CHAT_HISTORY_WINDOW_SIZE + Math.max(0, extraRevealed);
  if (entries.length <= windowSize) {
    return { hidden: 0, visible: entries };
  }
  const hidden = entries.length - windowSize;
  return { hidden, visible: entries.slice(hidden) };
}
