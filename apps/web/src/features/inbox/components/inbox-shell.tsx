import { Inbox } from "lucide-react";
import {
  LiquidCard,
  LiquidTabsList,
  LiquidTabsTrigger,
  SemanticIconTile,
  StatusBadge,
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { CardContent } from "@/components/ui/card";
import { Tabs } from "@/components/ui/tabs";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import type { InboxAction, InboxItem, InboxListResponse, InboxViewMode } from "@/lib/api/inbox";
import { InboxItemList } from "./inbox-item-list";

type InboxShellProps = {
  data?: InboxListResponse;
  error: Error | null;
  isFetching: boolean;
  isLoading: boolean;
  mutationError: Error | null;
  onAction: (item: InboxItem, action: InboxAction) => void;
  onRetry: () => void;
  onViewChange: (view: InboxViewMode) => void;
  view: InboxViewMode;
};

export function InboxShell({
  data,
  error,
  isFetching,
  isLoading,
  mutationError,
  onAction,
  onRetry,
  onViewChange,
  view,
}: InboxShellProps) {
  const hasItems = Boolean(data?.items.length);

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <SemanticIconTile tone="decision" size="lg">
                <Inbox />
              </SemanticIconTile>
              <div className="min-w-0">
                <h1 className="text-2xl font-bold tracking-normal">收件箱</h1>
                <p className="text-sm text-muted-foreground">需要你处理的事项</p>
              </div>
            </div>
            {data ? (
              <div className="flex flex-wrap items-center gap-2">
                <StatusBadge tone="decision">开放 {data.summary.open_count}</StatusBadge>
                <StatusBadge tone="danger">高风险 {data.summary.high_risk_count}</StatusBadge>
                <StatusBadge tone="warning">阻断 {data.summary.blocked_count}</StatusBadge>
              </div>
            ) : null}
          </div>

          <Tabs value={view} onValueChange={(value) => onViewChange(value as InboxViewMode)}>
            <LiquidTabsList className="max-w-md">
              <LiquidTabsTrigger value="mine">我的待办</LiquidTabsTrigger>
              <LiquidTabsTrigger value="team">团队待办</LiquidTabsTrigger>
            </LiquidTabsList>
          </Tabs>

          {mutationError ? (
            <Alert variant="destructive">
              <AlertTitle>操作未完成</AlertTitle>
              <AlertDescription>{mutationError.message}</AlertDescription>
            </Alert>
          ) : null}

          {error && !data ? (
            <Alert variant="destructive">
              <AlertTitle>收件箱加载失败</AlertTitle>
              <AlertDescription className="flex flex-wrap items-center gap-3">
                <span>{error.message}</span>
                <Button type="button" variant="outline" size="sm" onClick={onRetry}>
                  重试
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}

          {isFetching && hasItems ? (
            <div className="text-xs text-muted-foreground">正在刷新</div>
          ) : null}

          {isLoading && !data ? (
            <LiquidCard>
              <CardContent className="py-8 text-sm text-muted-foreground">
                加载收件箱中
              </CardContent>
            </LiquidCard>
          ) : null}

          {data ? (
            hasItems ? (
              <InboxItemList items={data.items} onAction={onAction} view={view} />
            ) : (
              <LiquidCard>
                <CardContent className="py-8 text-sm text-muted-foreground">
                  当前没有需要处理的事项。
                </CardContent>
              </LiquidCard>
            )
          ) : null}
        </div>
      </Main>
    </>
  );
}
