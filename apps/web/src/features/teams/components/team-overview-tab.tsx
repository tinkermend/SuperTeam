import { ShieldCheck, Users, Bot, Puzzle, TriangleAlert, Info } from "lucide-react";
import { MetricCard } from "@/components/superteam/liquid-components";
import { UserIdentity } from "@/components/superteam/user-identity";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import type { TeamOverview } from "@/lib/api/teams";

type TeamOverviewTabProps = {
  overview: TeamOverview;
};

export function TeamOverviewTab({ overview }: TeamOverviewTabProps) {
  const { team, member_count, digital_employee_count, capability_count, pending_item_count, current_revision } = overview;

  return (
    <div className="flex flex-col gap-6">
      {/* Metrics Section */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          title="人类成员"
          value={member_count}
          icon={<Users />}
          meta="当前团队成员"
        />
        <MetricCard
          title="数字员工"
          value={digital_employee_count}
          icon={<Bot />}
          meta="AI 代理执行引擎"
        />
        <MetricCard
          title="绑定能力"
          value={capability_count}
          icon={<Puzzle />}
          meta="MCP 与外部工具"
        />
        <MetricCard
          title="待审批项"
          value={pending_item_count}
          icon={<TriangleAlert />}
          meta="需人类介入决策"
          statusTone={pending_item_count > 0 ? "warning" : "success"}
        />
      </div>

      <div className="grid gap-6 md:grid-cols-2">
        {/* Team Metadata Section */}
        <Card className="rounded-2xl">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <Info className="size-5 text-muted-foreground" />
              团队元数据
            </CardTitle>
            <CardDescription>团队标识、状态与人类负责人。</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="grid grid-cols-[100px_1fr] items-center gap-4 text-sm">
              <span className="text-muted-foreground">唯一标识 (Slug)</span>
              <span className="font-mono bg-muted px-2 py-1 rounded-md w-fit">{team.slug}</span>
            </div>
            <Separator />
            <div className="grid grid-cols-[100px_1fr] items-start gap-4 text-sm">
              <span className="text-muted-foreground mt-2">负责人 (Owners)</span>
              <div className="flex flex-col gap-2">
                {team.human_owners && team.human_owners.length > 0 ? (
                  team.human_owners.map((owner) => (
                    <UserIdentity
                      key={owner.user_id}
                      user={{
                        id: owner.user_id,
                        username: owner.display_name || owner.email || owner.user_id,
                        display_name: owner.display_name,
                        email: owner.email,
                        avatar: owner.avatar,
                      }}
                      showSecondary
                    />
                  ))
                ) : (
                  <span className="text-muted-foreground">暂无</span>
                )}
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Governance Strategy Section */}
        <Card className="rounded-2xl">
          <CardHeader>
            <CardTitle className="text-lg flex items-center gap-2">
              <ShieldCheck className="size-5 text-superteam-permission" />
              治理策略
            </CardTitle>
            <CardDescription>当前生效的团队管控规则版本。</CardDescription>
          </CardHeader>
          <CardContent>
            {current_revision ? (
              <div className="flex flex-col gap-4">
                <div className="flex items-center justify-between rounded-lg border bg-muted/30 p-4">
                  <div className="flex flex-col gap-1">
                    <span className="text-sm font-medium">当前版本</span>
                    <span className="text-xs text-muted-foreground">Revision #{current_revision.revision_number}</span>
                  </div>
                  <Badge variant={current_revision.status === "active" ? "default" : "secondary"}>
                    {current_revision.status}
                  </Badge>
                </div>
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div className="flex flex-col gap-1">
                    <span className="text-muted-foreground text-xs">策略创建时间</span>
                    <span>{current_revision.created_at ? new Date(current_revision.created_at).toLocaleString() : "未知"}</span>
                  </div>
                  <div className="flex flex-col gap-1">
                    <span className="text-muted-foreground text-xs">策略审批时间</span>
                    <span>{current_revision.approved_at ? new Date(current_revision.approved_at).toLocaleString() : "未知"}</span>
                  </div>
                </div>
              </div>
            ) : (
              <div className="flex h-32 flex-col items-center justify-center gap-2 rounded-lg border border-dashed text-center">
                <ShieldCheck className="size-8 text-muted-foreground/50" />
                <p className="text-sm text-muted-foreground">尚未配置并生效治理策略版本。</p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
