const BRAND_MARK_SRC = "/images/brand/jushu-platform-mark.png";

export function AppTitle() {
  return (
    <div className="flex min-w-0 flex-col">
      <div
        aria-label="炬枢平台 - 新炬网络"
        data-testid="app-title-brand-lockup"
        className="flex min-h-20 min-w-0 items-center gap-3 rounded-[18px] border border-[var(--v3-shell-glass-border)] bg-[linear-gradient(135deg,rgba(255,255,255,0.54),rgba(233,239,255,0.30))] px-3.5 py-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.72),0_8px_20px_rgba(47,95,255,0.045)] group-data-[collapsible=icon]:min-h-12 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:border-0 group-data-[collapsible=icon]:bg-transparent group-data-[collapsible=icon]:p-0 group-data-[collapsible=icon]:shadow-none"
      >
        <span className="flex size-[54px] shrink-0 items-center justify-center overflow-hidden rounded-[18px] border border-[rgba(47,95,255,0.12)] bg-[rgba(255,255,255,0.44)] shadow-[inset_0_1px_0_rgba(255,255,255,0.62)] group-data-[collapsible=icon]:size-11 group-data-[collapsible=icon]:border-0 group-data-[collapsible=icon]:bg-transparent group-data-[collapsible=icon]:shadow-none">
          <img
            src={BRAND_MARK_SRC}
            alt=""
            aria-hidden="true"
            className="h-[46px] w-[46px] object-contain opacity-100 drop-shadow-none dark:brightness-[1.12] dark:saturate-[1.08] group-data-[collapsible=icon]:h-11 group-data-[collapsible=icon]:w-11"
          />
        </span>
        <span className="flex min-w-0 flex-col leading-none group-data-[collapsible=icon]:hidden">
          <span className="truncate text-[22px] font-bold leading-[1.08] text-v3-brand-deep">
            炬枢平台
          </span>
          <span className="mt-2 text-[14px] font-semibold leading-none text-v3-ink-2">
            新炬网络
          </span>
        </span>
      </div>
    </div>
  );
}
