import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'

# 1. api/team_routes_test.go
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')
with open(team_routes_path, 'r') as f:
    content = f.read()

content = content.replace('HumanOwnerUserID', 'HumanOwnerUserIDs')
content = re.sub(r'HumanOwnerUserIDs:\s*&?(uuid\.New\(\)|uuid\.MustParse\("[^"]+"\)|tests\.MockUser\w+\(\)\.ID),?', 
                 lambda m: f'HumanOwnerUserIDs: []uuid.UUID{{{m.group(1).replace("&", "")}}},', content)

with open(team_routes_path, 'w') as f:
    f.write(content)

# 2. storage/queries/queries_test.go
queries_test_path = os.path.join(base, 'internal/storage/queries/queries_test.go')
with open(queries_test_path, 'r') as f:
    content = f.read()

content = content.replace('HumanOwnerUserID', 'HumanOwnerUserIds')
content = re.sub(r'HumanOwnerUserIds:\s*uuidPtr\(([^)]+)\),?',
                 r'HumanOwnerUserIds: []uuid.UUID{\1},', content)
content = re.sub(r'HumanOwnerUserIds:\s*uuidPtrFromNull\(.*\),?',
                 r'', content) # wait, uuidPtrFromNull might be used differently
content = re.sub(r'HumanOwnerUserIds:\s*(ownerID),',
                 r'HumanOwnerUserIds: []uuid.UUID{\1},', content)

with open(queries_test_path, 'w') as f:
    f.write(content)


# 3. tenant/pg_repository_test.go
pg_repo_test_path = os.path.join(base, 'internal/tenant/pg_repository_test.go')
with open(pg_repo_test_path, 'r') as f:
    content = f.read()

content = content.replace('HumanOwnerUserID', 'HumanOwnerUserIds')
content = content.replace('HumanOwner', 'HumanOwners')
content = content.replace('OwnerUserID', 'HumanOwners') # this might be wrong, wait
# For OwnerUsername, OwnerEmail, etc., we can just remove them from the test struct
content = re.sub(r'OwnerUsername:.*?\n', '', content)
content = re.sub(r'OwnerDisplayName:.*?\n', '', content)
content = re.sub(r'OwnerEmail:.*?\n', '', content)
content = re.sub(r'OwnerStatus:.*?\n', '', content)
content = re.sub(r'OwnerAvatarProvider:.*?\n', '', content)
content = re.sub(r'OwnerAvatarStyle:.*?\n', '', content)
content = re.sub(r'OwnerAvatarSeed:.*?\n', '', content)
content = re.sub(r'OwnerAvatarOptions:.*?\n', '', content)
content = re.sub(r'OwnerUserID:.*?\n', r'HumanOwners: []byte(`[{"id": "00000000-0000-0000-0000-000000000000", "username": "test", "status": "active"}]`),\n', content)

with open(pg_repo_test_path, 'w') as f:
    f.write(content)
