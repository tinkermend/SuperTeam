import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'

# 1. service_test.go
service_test_path = os.path.join(base, 'internal/tenant/service_test.go')
with open(service_test_path, 'r') as f:
    content = f.read()
content = content.replace('HumanOwnerUserIDss', 'HumanOwnerUserIDs')
with open(service_test_path, 'w') as f:
    f.write(content)

# 2. queries_test.go
queries_test_path = os.path.join(base, 'internal/storage/queries/queries_test.go')
with open(queries_test_path, 'r') as f:
    content = f.read()
content = re.sub(r'HumanOwnerUserIds:\s*\[\]uuid\.UUID\{uuid\.NullUUID\{[^}]+\}\},', r'HumanOwnerUserIds: []uuid.UUID{tests.MockUser().ID},', content)
content = re.sub(r'HumanOwnerUserIds:\s*\[\]uuid\.UUID\{uuid\.NullUUID\{.*\}\},', r'HumanOwnerUserIds: []uuid.UUID{uuid.New()},', content)
# Sometimes it's just uuid.NullUUID{...}
content = re.sub(r'HumanOwnerUserIds:\s*uuid\.NullUUID\{[^}]+\},', r'HumanOwnerUserIds: []uuid.UUID{tests.MockUser().ID},', content)

with open(queries_test_path, 'w') as f:
    f.write(content)

# 3. pg_repository_test.go
pg_repo_test_path = os.path.join(base, 'internal/tenant/pg_repository_test.go')
with open(pg_repo_test_path, 'r') as f:
    content = f.read()
content = re.sub(r'HumanOwners:\s*uuid\.NullUUID\{[^}]+\},', r'HumanOwners: []byte(`[{"id": "00000000-0000-0000-0000-000000000000", "username": "test", "status": "active"}]`),', content)
with open(pg_repo_test_path, 'w') as f:
    f.write(content)

# 4. team_routes_test.go
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')
with open(team_routes_path, 'r') as f:
    content = f.read()
content = content.replace('HumanOwner:       &tests.MockTeamHumanOwner,', 'HumanOwners: []tenant.TeamHumanOwner{tests.MockTeamHumanOwner},')
with open(team_routes_path, 'w') as f:
    f.write(content)
