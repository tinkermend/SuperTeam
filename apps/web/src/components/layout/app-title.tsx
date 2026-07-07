const BRAND_MARK_SRC = "/images/brand/jushu-platform-mark.png";

export function AppTitle() {
  return (
    <div className="flex min-w-0 flex-col">
      <div
        aria-label="炬枢平台 - 新炬网络"
        data-testid="app-title-brand-lockup"
        className="relative isolate flex min-h-[82px] min-w-0 items-center gap-3.5 px-1 py-2.5 group-data-[collapsible=icon]:min-h-12 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:gap-0 group-data-[collapsible=icon]:p-0"
      >
        <span
          aria-hidden="true"
          data-testid="brand-route-line"
          className="pointer-events-none absolute inset-x-1 top-1 h-px bg-gradient-to-r from-transparent via-v3-brand/35 to-transparent group-data-[collapsible=icon]:hidden"
        />
        <span
          aria-hidden="true"
          data-testid="brand-mark-field"
          className="pointer-events-none absolute left-0 top-3 size-[76px] rounded-[24px] bg-[radial-gradient(circle_at_50%_50%,rgba(47,95,255,0.13),rgba(47,95,255,0.045)_48%,transparent_74%)] group-data-[collapsible=icon]:hidden dark:bg-[radial-gradient(circle_at_50%_50%,rgba(91,139,255,0.20),rgba(91,139,255,0.065)_48%,transparent_74%)]"
        />
        <span className="relative flex size-[72px] shrink-0 items-center justify-center group-data-[collapsible=icon]:size-11">
          <span
            aria-hidden="true"
            className="absolute left-1/2 top-1/2 h-px w-[82px] -translate-x-1/2 -translate-y-1/2 bg-gradient-to-r from-transparent via-v3-brand/20 to-transparent group-data-[collapsible=icon]:hidden"
          />
          <span
            aria-hidden="true"
            className="absolute left-1/2 top-1/2 h-[82px] w-px -translate-x-1/2 -translate-y-1/2 bg-gradient-to-b from-transparent via-v3-brand/20 to-transparent group-data-[collapsible=icon]:hidden"
          />
          <img
            src={BRAND_MARK_SRC}
            alt=""
            aria-hidden="true"
            className="relative h-[68px] w-[68px] object-contain opacity-100 drop-shadow-[0_10px_18px_rgba(47,95,255,0.14)] dark:brightness-[1.12] dark:saturate-[1.08] dark:drop-shadow-[0_10px_18px_rgba(91,139,255,0.18)] group-data-[collapsible=icon]:h-11 group-data-[collapsible=icon]:w-11 group-data-[collapsible=icon]:drop-shadow-none"
          />
        </span>
        <span className="relative flex min-w-0 flex-col leading-none group-data-[collapsible=icon]:hidden">
          <span className="truncate text-[24px] font-extrabold leading-none text-v3-brand-deep">
            炬枢平台
          </span>
          <span className="mt-2 flex min-w-0 items-center gap-2 text-[13px] font-bold leading-none text-v3-ink-2">
            <span
              aria-hidden="true"
              data-testid="brand-subtitle-rule"
              className="h-px w-7 shrink-0 bg-gradient-to-r from-v3-brand/70 to-transparent"
            />
            <span className="truncate">新炬网络</span>
          </span>
        </span>
      </div>
    </div>
  );
}
