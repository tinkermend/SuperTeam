const BRAND_MARK_SRC = "/images/brand/jushu-platform-mark.png";

export function AppTitle() {
  return (
    <div className="flex min-w-0 flex-col">
      <div
        aria-label="炬枢平台 - 新炬网络"
        data-testid="app-title-brand-lockup"
        className="flex min-h-16 min-w-0 items-center gap-3 px-1 py-2 group-data-[collapsible=icon]:min-h-12 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:p-0"
      >
        <span className="flex size-[52px] shrink-0 items-center justify-center overflow-hidden rounded-2xl border border-white/55 bg-white/40 shadow-[inset_0_1px_0_rgba(255,255,255,0.6)] backdrop-blur-md dark:border-white/12 dark:bg-white/[0.06] group-data-[collapsible=icon]:size-11 group-data-[collapsible=icon]:border-0 group-data-[collapsible=icon]:bg-transparent group-data-[collapsible=icon]:shadow-none group-data-[collapsible=icon]:backdrop-blur-none">
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
