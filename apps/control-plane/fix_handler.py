import re

filepath = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane/internal/tenant/handler.go'

with open(filepath, 'r') as f:
    content = f.read()

# Replace struct definitions
content = re.sub(r'HumanOwnerUserID\s+\*uuid\.UUID\s+`json:"human_owner_user_id"(?:,omitempty)?`',
                 r'HumanOwnerUserIDs []uuid.UUID `json:"human_owner_user_ids,omitempty"`', content)

content = re.sub(r'HumanOwnerUserID\s+\*string\s+`json:"human_owner_user_id,omitempty"`',
                 r'HumanOwnerUserIDs []string `json:"human_owner_user_ids,omitempty"`', content)

content = re.sub(r'HumanOwner\s+\*teamHumanOwnerResponse\s+`json:"human_owner,omitempty"`',
                 r'HumanOwners []teamHumanOwnerResponse `json:"human_owners,omitempty"`', content)

# Replace assignments in CreateTeam, UpdateTeam, CreateTeamConfigRevision
content = re.sub(r'HumanOwnerUserID:\s*req\.HumanOwnerUserID,',
                 r'HumanOwnerUserIDs: req.HumanOwnerUserIDs,', content)

content = re.sub(r'HumanOwnerUserID:\s*input\.HumanOwnerUserID,',
                 r'HumanOwnerUserIDs: input.HumanOwnerUserIDs,', content)

# Replace domain -> response assignments
content = re.sub(r'HumanOwnerUserID:\s*uuidStringPtr\(team\.HumanOwnerUserID\),',
                 r'HumanOwnerUserIDs: uuidStringSlice(team.HumanOwnerUserIDs),', content)

content = re.sub(r'HumanOwner:\s*teamHumanOwnerResponseFromDomain\(team\.HumanOwner\),',
                 r'HumanOwners: teamHumanOwnersResponseFromDomain(team.HumanOwners),', content)

content = re.sub(r'HumanOwnerUserID:\s*uuidStringPtr\(item\.HumanOwnerUserID\),',
                 r'HumanOwnerUserIDs: uuidStringSlice(item.HumanOwnerUserIDs),', content)

content = re.sub(r'HumanOwner:\s*teamHumanOwnerResponseFromDomain\(item\.HumanOwner\),',
                 r'HumanOwners: teamHumanOwnersResponseFromDomain(item.HumanOwners),', content)

content = re.sub(r'HumanOwnerUserID:\s*uuidStringPtr\(revision\.HumanOwnerUserID\),',
                 r'HumanOwnerUserIDs: uuidStringSlice(revision.HumanOwnerUserIDs),', content)

# Add uuidStringSlice if not exists
if 'func uuidStringSlice' not in content:
    idx = content.find('func uuidStringPtr')
    if idx != -1:
        helper = """
func uuidStringSlice(values []uuid.UUID) []string {
	if values == nil {
		return nil
	}
	var res []string
	for _, v := range values {
		res = append(res, v.String())
	}
	return res
}

"""
        content = content[:idx] + helper + content[idx:]

# Rename teamHumanOwnerResponseFromDomain and change logic
old_func = """func teamHumanOwnerResponseFromDomain(owner *TeamHumanOwner) *teamHumanOwnerResponse {
	if owner == nil {
		return nil
	}
	return &teamHumanOwnerResponse{
		UserID:      owner.UserID.String(),
		Username:    owner.Username,
		DisplayName: owner.DisplayName,
		Email:       owner.Email,
		Status:      owner.Status,
		Avatar:      userAvatarConfigResponseFromDomain(owner.Avatar),
	}
}"""
new_func = """func teamHumanOwnersResponseFromDomain(owners []TeamHumanOwner) []teamHumanOwnerResponse {
	if owners == nil {
		return nil
	}
	var res []teamHumanOwnerResponse
	for _, owner := range owners {
		res = append(res, teamHumanOwnerResponse{
			UserID:      owner.UserID.String(),
			Username:    owner.Username,
			DisplayName: owner.DisplayName,
			Email:       owner.Email,
			Status:      owner.Status,
			Avatar:      userAvatarConfigResponseFromDomain(owner.Avatar),
		})
	}
	return res
}"""

content = content.replace(old_func, new_func)

with open(filepath, 'w') as f:
    f.write(content)
