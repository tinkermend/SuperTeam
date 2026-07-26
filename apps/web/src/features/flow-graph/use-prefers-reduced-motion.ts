import { useEffect, useState } from "react";

const REDUCED_MOTION_QUERY = "(prefers-reduced-motion: reduce)";

function matchesReducedMotion(): boolean {
  if (typeof window === "undefined" || typeof window.matchMedia !== "function") {
    return false;
  }
  return window.matchMedia(REDUCED_MOTION_QUERY).matches;
}

/**
 * 活图粒子层的降级开关（spec 2026-07-27 §1.1）：用户偏好减弱动效时返回 true，
 * 粒子流动降级为静态呼吸边；偏好变化实时响应。
 */
export function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(matchesReducedMotion);

  useEffect(() => {
    if (typeof window.matchMedia !== "function") return;
    const query = window.matchMedia(REDUCED_MOTION_QUERY);
    const onChange = (event: MediaQueryListEvent) => setReduced(event.matches);
    query.addEventListener("change", onChange);
    return () => query.removeEventListener("change", onChange);
  }, []);

  return reduced;
}
