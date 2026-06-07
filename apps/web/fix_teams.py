import re

filepath = '/Users/wangpei/src/singe/SuperTeam/apps/web/src/lib/api/teams.ts'

with open(filepath, 'r') as f:
    content = f.read()

# Replace struct definitions
content = re.sub(r'human_owner_user_id\?: string;',
                 r'human_owner_user_ids?: string[];', content)

content = re.sub(r'human_owner\?: TeamHumanOwner;',
                 r'human_owners?: TeamHumanOwner[];', content)

# CreateTeamInput
content = re.sub(r'human_owner_user_id: string;',
                 r'human_owner_user_ids: string[];', content)

# Remove interception from listTeamSummaries
list_fn = """export async function listTeamSummaries(
  options: ApiClientOptions,
  filters: ListTeamSummariesFilters = {},
): Promise<TeamListItem[]> {
  const teams = await getJson<TeamListItem[]>(
    options,
    teamListPath(filters),
    "team summaries",
  );

  return teams.map((t) => {
    if (t.name === "安全团队" && t.human_owner) {
      return {
        ...t,
        human_owners: [
          t.human_owner,
          { ...t.human_owner, user_id: "fake-1", display_name: "安全副总甲", email: "sec1@example.com", username: "sec1" },
          { ...t.human_owner, user_id: "fake-2", display_name: "安全副总乙", email: "sec2@example.com", username: "sec2" },
        ],
      };
    }
    if (t.human_owner) {
      return { ...t, human_owners: [t.human_owner] };
    }
    return t;
  });
}"""
list_fn_new = """export async function listTeamSummaries(
  options: ApiClientOptions,
  filters: ListTeamSummariesFilters = {},
): Promise<TeamListItem[]> {
  return getJson<TeamListItem[]>(
    options,
    teamListPath(filters),
    "team summaries",
  );
}"""

content = content.replace(list_fn, list_fn_new)

with open(filepath, 'w') as f:
    f.write(content)
