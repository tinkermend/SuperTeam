import os
import re

base = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane'

# 1. service_test.go
service_test_path = os.path.join(base, 'internal/tenant/service_test.go')
with open(service_test_path, 'r') as f:
    content = f.read()

content = content.replace('HumanOwnerUserIDs: &ownerID,', 'HumanOwnerUserIDs: []uuid.UUID{ownerID},')
# To handle lines 330, 526, 544, 601, 621 where there might be &ownerID without a comma or trailing
content = re.sub(r'HumanOwnerUserIDs:\s*&ownerID\b', r'HumanOwnerUserIDs: []uuid.UUID{ownerID}', content)

active_users = """	if len(r.activeUsers) > 0 && !r.activeUsers[params.OwnerUserIDs] {
		return TeamRecord{}, ErrNotFound
	}"""
active_users_new = """	if len(r.activeUsers) > 0 {
		for _, ownerID := range params.OwnerUserIDs {
			if !r.activeUsers[ownerID] {
				return TeamRecord{}, ErrNotFound
			}
		}
	}"""
content = content.replace(active_users, active_users_new)

membership = """	ownerMembership := TeamMemberRecord{
		MembershipID:     uuid.New(),
		TenantID:         params.TenantID,
		TeamID:           team.ID,
		UserID:           params.OwnerUserIDs,
		Role:             TeamRoleOwner,
		MembershipStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.teamMembers[ownerMembership.MembershipID] = ownerMembership
	r.auditEvents = append(r.auditEvents,
		memoryAuditEvent{Action: "team.create", ResourceType: "team", ResourceID: team.ID},
		memoryAuditEvent{Action: "team.member.add", ResourceType: "team_member", ResourceID: ownerMembership.MembershipID},
	)"""
membership_new = """	r.auditEvents = append(r.auditEvents, memoryAuditEvent{Action: "team.create", ResourceType: "team", ResourceID: team.ID})
	for _, ownerID := range params.OwnerUserIDs {
		ownerMembership := TeamMemberRecord{
			MembershipID:     uuid.New(),
			TenantID:         params.TenantID,
			TeamID:           team.ID,
			UserID:           ownerID,
			Role:             TeamRoleOwner,
			MembershipStatus: "active",
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		r.teamMembers[ownerMembership.MembershipID] = ownerMembership
		r.auditEvents = append(r.auditEvents,
			memoryAuditEvent{Action: "team.member.add", ResourceType: "team_member", ResourceID: ownerMembership.MembershipID},
		)
	}"""
content = content.replace(membership, membership_new)

with open(service_test_path, 'w') as f:
    f.write(content)

# 2. team_routes_test.go
team_routes_path = os.path.join(base, 'internal/api/team_routes_test.go')
with open(team_routes_path, 'r') as f:
    content = f.read()

# Fix internal/api/team_routes_test.go:1065:8: missing ',' before newline in composite literal
# The error is likely due to the replace `HumanOwners: []tenant.TeamHumanOwner{{`
# Let's fix line 1065 manually if we can, or just use sed
# "HumanOwner:       &tests.MockTeamHumanOwner,"
# Let's see what is near line 1065
content = content.replace('HumanOwners: []tenant.TeamHumanOwner{{', 'HumanOwners: []tenant.TeamHumanOwner{')
with open(team_routes_path, 'w') as f:
    f.write(content)

