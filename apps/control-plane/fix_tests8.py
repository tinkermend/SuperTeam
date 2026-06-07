import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')

with open(team_routes_path, 'r') as f:
    content = f.read()

# Replace struct field `HumanOwner *struct` with `HumanOwners []struct`
content = content.replace('HumanOwner *struct {', 'HumanOwners []struct {')
content = content.replace('`json:"human_owner"`', '`json:"human_owners"`')

# Replace listed[0].HumanOwner with listed[0].HumanOwners[0]
content = content.replace('listed[0].HumanOwner == nil', 'len(listed[0].HumanOwners) == 0')
content = content.replace('listed[0].HumanOwner.', 'listed[0].HumanOwners[0].')

# Fix overview[0] or whatever in get team overview
content = content.replace('got.HumanOwner == nil', 'len(got.HumanOwners) == 0')
content = content.replace('got.HumanOwner.', 'got.HumanOwners[0].')
content = content.replace('got.Team.HumanOwnerUserIDs != ownerID.String()', 'len(got.Team.HumanOwnerUserIDs) == 0 || got.Team.HumanOwnerUserIDs[0] != ownerID.String()')
content = content.replace('got.Team.HumanOwnerUserIDs != updateOwnerID.String()', 'len(got.Team.HumanOwnerUserIDs) == 0 || got.Team.HumanOwnerUserIDs[0] != updateOwnerID.String()')

with open(team_routes_path, 'w') as f:
    f.write(content)
