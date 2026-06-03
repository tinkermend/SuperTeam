import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { TeamMemberRole } from "@/lib/api/teams";

export const teamRoleLabels = {
  admin: "管理员",
  approver: "审批人",
  member: "普通成员",
  owner: "负责人",
  viewer: "只读观察者",
} as const satisfies Record<TeamMemberRole, string>;

export type TeamRole = TeamMemberRole;
export type DirectTeamRole = Extract<TeamMemberRole, "member" | "viewer">;
export type PrivilegedTeamRole = Extract<TeamMemberRole, "owner" | "admin" | "approver">;

export const directTeamRoles = [
  { label: teamRoleLabels.member, value: "member" },
  { label: teamRoleLabels.viewer, value: "viewer" },
] as const satisfies ReadonlyArray<{ label: string; value: DirectTeamRole }>;

export const privilegedTeamRoles = [
  { label: teamRoleLabels.owner, value: "owner" },
  { label: teamRoleLabels.admin, value: "admin" },
  { label: teamRoleLabels.approver, value: "approver" },
] as const satisfies ReadonlyArray<{ label: string; value: PrivilegedTeamRole }>;

export function teamRoleLabel(role: TeamRole) {
  return teamRoleLabels[role];
}

type TeamRoleBadgeProps = {
  role: TeamMemberRole;
};

export function TeamRoleBadge({ role }: TeamRoleBadgeProps) {
  return <Badge variant={role === "member" || role === "viewer" ? "secondary" : "default"}>{teamRoleLabel(role)}</Badge>;
}

type TeamRoleSelectProps =
  | {
      disabled?: boolean;
      mode: "direct";
      onChange: (role: DirectTeamRole) => void;
      value: DirectTeamRole;
    }
  | {
      disabled?: boolean;
      mode: "privileged";
      onChange: (role: PrivilegedTeamRole) => void;
      value: PrivilegedTeamRole;
    };

type TeamRoleOption = {
  label: string;
  value: TeamMemberRole;
};

export function TeamRoleSelect(props: TeamRoleSelectProps) {
  const roles: ReadonlyArray<TeamRoleOption> = props.mode === "direct" ? directTeamRoles : privilegedTeamRoles;
  const label = props.mode === "direct" ? "直接成员角色" : "特权角色";

  function handleValueChange(role: string) {
    if (props.mode === "direct") {
      props.onChange(role as DirectTeamRole);
      return;
    }

    props.onChange(role as PrivilegedTeamRole);
  }

  return (
    <Select disabled={props.disabled} onValueChange={handleValueChange} value={props.value}>
      <SelectTrigger aria-label="团队角色" className="w-40" size="sm">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>{label}</SelectLabel>
          {roles.map((role) => (
            <SelectItem key={role.value} value={role.value}>
              {role.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
