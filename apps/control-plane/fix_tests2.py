import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'

# 1. api/team_routes_test.go
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')
with open(team_routes_path, 'r') as f:
    content = f.read()

content = re.sub(r'\*service\.createReq\.HumanOwnerUserIDs', r'service.createReq.HumanOwnerUserIDs[0]', content)
content = re.sub(r'\*service\.createRevisionReq\.HumanOwnerUserIDs', r'service.createRevisionReq.HumanOwnerUserIDs[0]', content)
content = re.sub(r'\*service\.updateDraftInput\.HumanOwnerUserIDs', r'service.updateDraftInput.HumanOwnerUserIDs[0]', content)
content = content.replace('HumanOwnerUserIDs: &ownerID,', 'HumanOwnerUserIDs: []uuid.UUID{ownerID},')
content = content.replace('HumanOwnerUserIDs: &uuid.UUID{},', 'HumanOwnerUserIDs: []uuid.UUID{},')
content = content.replace('HumanOwner:       &tests.MockTeamHumanOwner,', 'HumanOwners: []tenant.TeamHumanOwner{tests.MockTeamHumanOwner},')

with open(team_routes_path, 'w') as f:
    f.write(content)

# 2. storage/queries/queries_test.go
queries_test_path = os.path.join(base, 'internal/storage/queries/queries_test.go')
with open(queries_test_path, 'r') as f:
    content = f.read()

content = re.sub(r'HumanOwnerUserIds:\s*\[\]uuid\.UUID\{ownerID\},', r'HumanOwnerUserIds: []uuid.UUID{ownerID.UUID},', content)
content = re.sub(r'HumanOwnerUserIds:\s*\[\]uuid\.UUID\{uuid\.NullUUID\{.*\}\},', r'HumanOwnerUserIds: []uuid.UUID{tests.MockUser().ID},', content)

with open(queries_test_path, 'w') as f:
    f.write(content)

# 3. tenant/pg_repository_test.go
pg_repo_test_path = os.path.join(base, 'internal/tenant/pg_repository_test.go')
with open(pg_repo_test_path, 'r') as f:
    content = f.read()

content = content.replace('HumanOwners.Avatar', 'HumanOwners[0].Avatar')
content = content.replace('HumanOwners: uuid.NullUUID{UUID: uuid.New(), Valid: true},', 'HumanOwners: []byte(`[{"id": "00000000-0000-0000-0000-000000000000", "username": "test", "status": "active"}]`),')
content = content.replace('HumanOwnersUserIds', 'HumanOwnerUserIds')

with open(pg_repo_test_path, 'w') as f:
    f.write(content)

# 4. tenant/service_test.go
service_test_path = os.path.join(base, 'internal/tenant/service_test.go')
with open(service_test_path, 'r') as f:
    content = f.read()

content = content.replace('HumanOwnerUserID:', 'HumanOwnerUserIDs:')
content = re.sub(r'HumanOwnerUserIDs:\s*uuidPtr\(([^)]+)\),?', r'HumanOwnerUserIDs: []uuid.UUID{\1},', content)
content = content.replace('OwnerUserID', 'OwnerUserIDs')
content = re.sub(r'OwnerUserIDs:\s*([^,]+),', lambda m: f'OwnerUserIDs: []uuid.UUID{{{m.group(1).replace("[]uuid.UUID{", "").replace("}", "")}}},' if not '[]uuid.UUID' in m.group(1) else m.group(0), content)
content = content.replace('repo.createdTeamWithMembers.OwnerUserIDs', 'repo.createdTeamWithMembers.OwnerUserIDs[0]')

with open(service_test_path, 'w') as f:
    f.write(content)
