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

export const teamRoleLabels = {
  admin: "管理员",
  approver: "审批人",
  member: "普通成员",
  owner: "负责人",
  viewer: "只读观察者",
} as const;

export type TeamRole = keyof typeof teamRoleLabels;

export const directTeamRoles = [
  { label: teamRoleLabels.member, value: "member" },
  { label: teamRoleLabels.viewer, value: "viewer" },
] as const;

export const privilegedTeamRoles = [
  { label: teamRoleLabels.owner, value: "owner" },
  { label: teamRoleLabels.admin, value: "admin" },
  { label: teamRoleLabels.approver, value: "approver" },
] as const;

export function teamRoleLabel(role: TeamRole) {
  return teamRoleLabels[role];
}

type TeamRoleBadgeProps = {
  role: TeamRole;
};

export function TeamRoleBadge({ role }: TeamRoleBadgeProps) {
  return <Badge variant={role === "member" || role === "viewer" ? "secondary" : "default"}>{teamRoleLabel(role)}</Badge>;
}

type TeamRoleSelectProps = {
  "aria-label"?: string;
  disabled?: boolean;
  onValueChange: (role: TeamRole) => void;
  placeholder?: string;
  value?: TeamRole;
};

export function TeamRoleSelect({
  "aria-label": ariaLabel = "团队角色",
  disabled = false,
  onValueChange,
  placeholder = "选择角色",
  value,
}: TeamRoleSelectProps) {
  return (
    <Select disabled={disabled} onValueChange={(role) => onValueChange(role as TeamRole)} value={value}>
      <SelectTrigger aria-label={ariaLabel} className="w-40" size="sm">
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        <SelectGroup>
          <SelectLabel>直接成员角色</SelectLabel>
          {directTeamRoles.map((role) => (
            <SelectItem key={role.value} value={role.value}>
              {role.label}
            </SelectItem>
          ))}
        </SelectGroup>
        <SelectGroup>
          <SelectLabel>特权角色</SelectLabel>
          {privilegedTeamRoles.map((role) => (
            <SelectItem key={role.value} value={role.value}>
              {role.label}
            </SelectItem>
          ))}
        </SelectGroup>
      </SelectContent>
    </Select>
  );
}
