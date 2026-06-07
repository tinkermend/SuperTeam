import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'

# 1. api/team_routes_test.go
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')
with open(team_routes_path, 'r') as f:
    content = f.read()

# Fix string concatenation in JSON
content = re.sub(r'\"human_owner_user_id\":\"`\s*\+\s*([^`]+)\s*\+\s*`\"', r'"human_owner_user_ids":["` + \1 + `"]', content)
content = re.sub(r'\"human_owner_user_id\":\"`\+([^`]+)\+`\"', r'"human_owner_user_ids":["`+\1+`"]', content)
content = re.sub(r'\"human_owner_user_id\":\"` \+ ([^`]+) \+ `\"', r'"human_owner_user_ids":["` + \1 + `"]', content)

# Fix struct fields
content = content.replace('HumanOwnerUserIDs string         `json:"human_owner_user_id"`', 'HumanOwnerUserIDs []string       `json:"human_owner_user_ids"`')
content = content.replace('created.Team.HumanOwnerUserIDs != ownerID.String()', 'len(created.Team.HumanOwnerUserIDs) == 0 || created.Team.HumanOwnerUserIDs[0] != ownerID.String()')

with open(team_routes_path, 'w') as f:
    f.write(content)
