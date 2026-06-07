import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'

# 1. api/team_routes_test.go
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')
with open(team_routes_path, 'r') as f:
    content = f.read()

content = re.sub(r'HumanOwner:\s*&tests\.MockTeamHumanOwner,', 'HumanOwners: []tenant.TeamHumanOwner{tests.MockTeamHumanOwner},', content)

with open(team_routes_path, 'w') as f:
    f.write(content)

# 2. storage/queries/queries_test.go
queries_test_path = os.path.join(base, 'internal/storage/queries/queries_test.go')
with open(queries_test_path, 'r') as f:
    content = f.read()

content = content.replace('tests.MockUser().ID', 'uuid.New()')

with open(queries_test_path, 'w') as f:
    f.write(content)

# 3. tenant/service_test.go
service_test_path = os.path.join(base, 'internal/tenant/service_test.go')
with open(service_test_path, 'r') as f:
    content = f.read()

content = content.replace('HumanOwnerUserIDs: &ownerID,', 'HumanOwnerUserIDs: []uuid.UUID{ownerID},')
content = content.replace('HumanOwnerUserIDs: &newOwnerID,', 'HumanOwnerUserIDs: []uuid.UUID{newOwnerID},')
content = content.replace('HumanOwnerUserIDs: &uuid.UUID{},', 'HumanOwnerUserIDs: []uuid.UUID{},')

with open(service_test_path, 'w') as f:
    f.write(content)
