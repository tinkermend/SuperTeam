import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import type { TeamListItem } from "@/lib/api";
import { cn } from "@/lib/utils";

type SelectableTeamListProps = {
  disabled?: boolean;
  onChange: (teamIds: string[]) => void;
  selectedTeamIds: string[];
  teams: TeamListItem[];
};

export function SelectableTeamList({
  disabled = false,
  onChange,
  selectedTeamIds,
  teams
}: SelectableTeamListProps) {
  function toggleTeam(teamId: string) {
    if (selectedTeamIds.includes(teamId)) {
      onChange(selectedTeamIds.filter((id) => id !== teamId));
      return;
    }

    onChange([...selectedTeamIds, teamId]);
  }

  return (
    <div className="grid gap-2">
      {teams.map((team) => {
        const checked = selectedTeamIds.includes(team.id);
        const checkboxId = `create-user-team-${team.id}`;

        return (
          <Label
            className={cn(
              "flex min-w-0 cursor-pointer items-start gap-3 rounded-md border bg-background/70 p-3 text-sm transition-colors",
              checked ? "border-primary bg-secondary/70" : "hover:bg-accent/50",
              disabled ? "cursor-not-allowed opacity-60" : "",
            )}
            htmlFor={checkboxId}
            key={team.id}
          >
            <Checkbox
              aria-label={team.name}
              checked={checked}
              disabled={disabled}
              id={checkboxId}
              onCheckedChange={() => toggleTeam(team.id)}
            />
            <span className="min-w-0 flex-1">
              <span className="block truncate font-medium">{team.name}</span>
              <span className="mt-1 block truncate text-xs text-muted-foreground">
                {team.slug} / {team.governance_status}
              </span>
            </span>
          </Label>
        );
      })}
    </div>
  );
}
