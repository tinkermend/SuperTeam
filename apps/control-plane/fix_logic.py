import re

pg_repo_path = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane/internal/tenant/pg_repository.go'
with open(pg_repo_path, 'r') as f:
    pg_repo = f.read()

# Fix GetActiveTenantUserForTeamCreate
get_active = """	if _, err := qtx.GetActiveTenantUserForTeamCreate(ctx, queries.GetActiveTenantUserForTeamCreateParams{
		ID:       params.OwnerUserID,
		TenantID: params.TenantID,
	}); err != nil {
		return TeamRecord{}, mapNoRows(err)
	}"""
get_active_new = """	for _, ownerID := range params.OwnerUserIDs {
		if _, err := qtx.GetActiveTenantUserForTeamCreate(ctx, queries.GetActiveTenantUserForTeamCreateParams{
			ID:       ownerID,
			TenantID: params.TenantID,
		}); err != nil {
			return TeamRecord{}, mapNoRows(err)
		}
	}"""
pg_repo = pg_repo.replace(get_active, get_active_new)

# Fix AddTeamOwnerMembership
add_membership = """	ownerMembership, err := qtx.AddTeamOwnerMembership(ctx, queries.AddTeamOwnerMembershipParams{
		TenantID: params.TenantID,
		TeamID:   team.ID,
		UserID:   params.OwnerUserID,
	})
	if err != nil {
		return TeamRecord{}, mapConstraintError(err)
	}
	if err := createTeamAuditEvent(ctx, qtx, params, team.ID); err != nil {
		return TeamRecord{}, err
	}
	if err := createTeamMemberAuditEvent(ctx, qtx, params, team.ID, ownerMembership.ID, params.OwnerUserID, TeamRoleOwner); err != nil {
		return TeamRecord{}, err
	}"""
add_membership_new = """	if err := createTeamAuditEvent(ctx, qtx, params, team.ID); err != nil {
		return TeamRecord{}, err
	}
	for _, ownerID := range params.OwnerUserIDs {
		ownerMembership, err := qtx.AddTeamOwnerMembership(ctx, queries.AddTeamOwnerMembershipParams{
			TenantID: params.TenantID,
			TeamID:   team.ID,
			UserID:   ownerID,
		})
		if err != nil {
			return TeamRecord{}, mapConstraintError(err)
		}
		if err := createTeamMemberAuditEvent(ctx, qtx, params, team.ID, ownerMembership.ID, ownerID, TeamRoleOwner); err != nil {
			return TeamRecord{}, err
		}
	}"""
pg_repo = pg_repo.replace(add_membership, add_membership_new)

# Fix createTeamAuditEvent
audit_event = """func createTeamAuditEvent(ctx context.Context, q *queries.Queries, params CreateTeamWithInitialMembersParams, teamID uuid.UUID) error {
	details, err := json.Marshal(map[string]any{
		"team_id":             teamID.String(),
		"slug":                params.Slug,
		"human_owner_user_id": params.OwnerUserID.String(),
		"initial_members":     len(params.InitialMembers),
	})"""
audit_event_new = """func createTeamAuditEvent(ctx context.Context, q *queries.Queries, params CreateTeamWithInitialMembersParams, teamID uuid.UUID) error {
	var ownerIDStrs []string
	for _, id := range params.OwnerUserIDs {
		ownerIDStrs = append(ownerIDStrs, id.String())
	}
	details, err := json.Marshal(map[string]any{
		"team_id":              teamID.String(),
		"slug":                 params.Slug,
		"human_owner_user_ids": ownerIDStrs,
		"initial_members":      len(params.InitialMembers),
	})"""
pg_repo = pg_repo.replace(audit_event, audit_event_new)

with open(pg_repo_path, 'w') as f:
    f.write(pg_repo)


service_path = '/Users/wangpei/src/singe/SuperTeam/apps/control-plane/internal/tenant/service.go'
with open(service_path, 'r') as f:
    service = f.read()

# Fix service CreateTeam owner validation
svc_val = """	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = TeamStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	initialMembers, err := normalizeInitialMembers(*req.HumanOwnerUserID, req.InitialMembers)"""
svc_val_new = """	if len(req.HumanOwnerUserIDs) == 0 {
		return nil, fmt.Errorf("%w: human_owner_user_ids is required", ErrInvalidInput)
	}
	for _, ownerID := range req.HumanOwnerUserIDs {
		if ownerID == uuid.Nil {
			return nil, fmt.Errorf("%w: owner_user_id cannot be nil", ErrInvalidInput)
		}
	}
	status := req.Status
	if status == "" {
		status = TeamStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	initialMembers, err := normalizeInitialMembers(req.HumanOwnerUserIDs, req.InitialMembers)"""
service = service.replace(svc_val, svc_val_new)

# Fix CreateTeamWithInitialMembersParams
svc_create = """		OwnerUserID:    *req.HumanOwnerUserID,"""
svc_create_new = """		OwnerUserIDs:   req.HumanOwnerUserIDs,"""
service = service.replace(svc_create, svc_create_new)

# Fix normalizeInitialMembers
norm_init = """func normalizeInitialMembers(ownerUserID uuid.UUID, members []InitialTeamMemberInput) ([]InitialTeamMemberInput, error) {
	seen := map[uuid.UUID]struct{}{ownerUserID: {}}"""
norm_init_new = """func normalizeInitialMembers(ownerUserIDs []uuid.UUID, members []InitialTeamMemberInput) ([]InitialTeamMemberInput, error) {
	seen := map[uuid.UUID]struct{}{}
	for _, id := range ownerUserIDs {
		seen[id] = struct{}{}
	}"""
service = service.replace(norm_init, norm_init_new)

with open(service_path, 'w') as f:
    f.write(service)

