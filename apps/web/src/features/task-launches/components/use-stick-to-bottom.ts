import { useCallback, useLayoutEffect, useRef, useState, type RefObject } from "react";

const BOTTOM_THRESHOLD_PX = 100;

function gapFromBottom(node: HTMLElement): number {
  return node.scrollHeight - node.scrollTop - node.clientHeight;
}

/** Stick a scroll container to its end unless the user scrolls away. */
export function useStickToBottom(
  containerRef: RefObject<HTMLElement | null>,
  contentKey: unknown,
  pinToken: unknown,
  innerRef?: RefObject<HTMLElement | null>,
): { awayFromBottom: boolean; jumpToBottom: () => void } {
  const pinnedRef = useRef(true);
  const [awayFromBottom, setAwayFromBottom] = useState(false);

  const jumpToBottom = useCallback(() => {
    const node = containerRef.current;
    if (!node) {
      return;
    }
    pinnedRef.current = true;
    setAwayFromBottom(false);
    node.scrollTop = node.scrollHeight;
  }, [containerRef]);

  useLayoutEffect(() => {
    jumpToBottom();
  }, [jumpToBottom, pinToken]);

  useLayoutEffect(() => {
    const node = containerRef.current;
    if (!node || !pinnedRef.current) {
      return;
    }
    node.scrollTop = node.scrollHeight;
  }, [containerRef, contentKey]);

  useLayoutEffect(() => {
    const node = containerRef.current;
    if (!node) {
      return;
    }
    const onWheel = (event: WheelEvent) => {
      if (event.deltaY < 0) {
        pinnedRef.current = false;
        setAwayFromBottom(true);
        return;
      }
      if (gapFromBottom(node) <= BOTTOM_THRESHOLD_PX) {
        pinnedRef.current = true;
        setAwayFromBottom(false);
      }
    };
    const onScroll = () => {
      const away = gapFromBottom(node) > BOTTOM_THRESHOLD_PX;
      pinnedRef.current = !away;
      setAwayFromBottom(away);
    };
    node.addEventListener("wheel", onWheel, { passive: true });
    node.addEventListener("scroll", onScroll, { passive: true });
    const resize = new ResizeObserver(() => {
      if (!pinnedRef.current) {
        return;
      }
      node.scrollTop = node.scrollHeight;
    });
    resize.observe(node);
    const inner = innerRef?.current;
    if (inner) {
      resize.observe(inner);
    }
    return () => {
      node.removeEventListener("wheel", onWheel);
      node.removeEventListener("scroll", onScroll);
      resize.disconnect();
    };
  }, [containerRef, innerRef, pinToken]);

  return { awayFromBottom, jumpToBottom };
}
