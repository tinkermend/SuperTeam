import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'

# 1. api/team_routes_test.go
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')
with open(team_routes_path, 'r') as f:
    content = f.read()

content = re.sub(r'HumanOwner:\s*&tenant\.TeamHumanOwner{', 'HumanOwners: []tenant.TeamHumanOwner{{', content)
content = re.sub(r'HumanOwner:\s*&tests\.MockTeamHumanOwner,?', 'HumanOwners: []tenant.TeamHumanOwner{tests.MockTeamHumanOwner},', content)

with open(team_routes_path, 'w') as f:
    f.write(content)

# 2. tenant/service_test.go
service_test_path = os.path.join(base, 'internal/tenant/service_test.go')
with open(service_test_path, 'r') as f:
    content = f.read()

# Replace any remaining &ownerID in HumanOwnerUserIDs
content = content.replace('HumanOwnerUserIDs: &ownerID,', 'HumanOwnerUserIDs: []uuid.UUID{ownerID},')
content = content.replace('HumanOwnerUserIDs: &newOwnerID,', 'HumanOwnerUserIDs: []uuid.UUID{newOwnerID},')
content = content.replace('HumanOwnerUserIDs: &uuid.UUID{},', 'HumanOwnerUserIDs: []uuid.UUID{},')

# Fix cannot indirect updated.HumanOwnerUserIDs
content = re.sub(r'\*updated\.HumanOwnerUserIDs', r'updated.HumanOwnerUserIDs[0]', content)

# Fix cannot use params.OwnerUserIDs as map index
content = content.replace('seen := map[uuid.UUID]struct{}{params.OwnerUserIDs: {}}', 'seen := map[uuid.UUID]struct{}{params.OwnerUserIDs[0]: {}}')

# Fix cannot use &params.OwnerUserIDs as []uuid.UUID value in struct literal
content = content.replace('HumanOwnerUserIDs: &params.OwnerUserIDs,', 'HumanOwnerUserIDs: params.OwnerUserIDs,')

# Fix cannot use params.OwnerUserIDs as uuid.UUID value in struct literal
content = content.replace('HumanOwnerUserID: params.OwnerUserIDs,', 'HumanOwnerUserIDs: params.OwnerUserIDs,')

# Fix validUUIDPtr(record.HumanOwnerUserIDs)
content = content.replace('validUUIDPtr(record.HumanOwnerUserIDs)', 'record.HumanOwnerUserIDs')

with open(service_test_path, 'w') as f:
    f.write(content)
