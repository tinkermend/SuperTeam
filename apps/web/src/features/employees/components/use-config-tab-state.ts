import { useEffect, useRef } from "react";

/**
 * 员工配置页 Tab 的「服务器快照 → 本地表单」重灌骨架(身份/执行/权限三 Tab 同构):
 * 同一员工且表单已脏时不回灌(避免打字被服务器刷新覆盖);换员工或字段变化时强制回灌。
 * deps 传触发重灌的 employee 字段(与原先手写 effect 的依赖数组一致);
 * hydrate 经 latest-ref 调用,不必为它做 useCallback。
 */
export function useTabRehydrate(
  employeeId: string,
  dirty: boolean,
  deps: unknown[],
  hydrate: () => void,
) {
  const hydrateRef = useRef(hydrate);
  useEffect(() => {
    hydrateRef.current = hydrate;
  });
  const hydratedIdRef = useRef(employeeId);
  useEffect(() => {
    if (hydratedIdRef.current === employeeId && dirty) return;
    hydrateRef.current();
    hydratedIdRef.current = employeeId;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [employeeId, ...deps]);
}

/** 向配置页外壳上报当前 Tab 脏态;卸载时归位为 false(三 Tab 逐字相同的样板)。 */
export function useDirtyReport(dirty: boolean, onDirtyChange: (dirty: boolean) => void) {
  useEffect(() => {
    onDirtyChange(dirty);
    return () => onDirtyChange(false);
  }, [dirty, onDirtyChange]);
}
