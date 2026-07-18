package tenant

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

func TestTeamStatusOnlyActiveIsValid(t *testing.T) {
	if !TeamStatusActive.IsValid() {
		t.Fatalf("expected active team status to be valid")
	}
	// 生命周期收敛：archived/disabled 已撤销，仅 active 合法。
	for _, status := range []TeamStatus{"archived", "disabled", "paused"} {
		if status.IsValid() {
			t.Fatalf("expected %q team status to be invalid", status)
		}
	}
}

func TestNewServiceRequiresTeamAuditReader(t *testing.T) {
	repo := newMemoryRepository()
	if _, err := NewService(repo, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected missing team audit reader to fail with invalid input, got %v", err)
	}
	if _, err := NewService(repo, &fakeTeamAuditReader{}); err != nil {
		t.Fatalf("expected service with team audit reader: %v", err)
	}
}

func TestCreateTeamDefaultsActiveStatus(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()

	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          uuid.New(),
		ActorUserID:       uuid.New(),
		Slug:              "engineering",
		Name:              "Engineering",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if team.Team.Status != TeamStatusActive {
		t.Fatalf("expected active default status, got %q", team.Team.Status)
	}
}

func TestCreateTeamRequiresHumanOwner(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:    uuid.New(),
		ActorUserID: uuid.New(),
		Slug:        "engineering",
		Name:        "Engineering",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if repo.createTeamCalled {
		t.Fatalf("expected invalid team not to reach repository")
	}
}

func TestCreateTeamCreatesOwnerAndInitialMembers(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	memberID := uuid.New()
	viewerID := uuid.New()
	repo.activeUsers[ownerID] = true
	repo.activeUsers[memberID] = true
	repo.activeUsers[viewerID] = true

	overview, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          tenantID,
		ActorUserID:       actorID,
		Slug:              "security",
		Name:              "安全团队",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
		InitialMembers: []InitialTeamMemberInput{
			{UserID: memberID, Role: TeamRoleMember},
			{UserID: viewerID, Role: TeamRoleViewer},
		},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if overview.Team == nil || overview.Team.Slug != "security" {
		t.Fatalf("expected created overview team, got %#v", overview.Team)
	}
	if overview.MemberCount != 3 {
		t.Fatalf("expected owner plus two members in overview, got %d", overview.MemberCount)
	}
	if repo.createdTeamWithMembers.OwnerUserIDs[0] != ownerID {
		t.Fatalf("expected owner %s, got %s", ownerID, repo.createdTeamWithMembers.OwnerUserIDs[0])
	}
	if got := repo.createdTeamWithMembers.InitialMembers; !reflect.DeepEqual(got, []InitialTeamMemberInput{
		{UserID: memberID, Role: TeamRoleMember},
		{UserID: viewerID, Role: TeamRoleViewer},
	}) {
		t.Fatalf("expected initial members preserved, got %#v", got)
	}
	if len(repo.auditEvents) != 4 {
		t.Fatalf("expected team create and member audit events, got %#v", repo.auditEvents)
	}
	if repo.auditEvents[0].Action != "team.create" {
		t.Fatalf("expected first audit action team.create, got %#v", repo.auditEvents)
	}
}

func TestCreateTeamRejectsInitialDigitalEmployeesOverCapacity(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	initialDigitalEmployeeIDs := make([]uuid.UUID, 11)
	for index := range initialDigitalEmployeeIDs {
		initialDigitalEmployeeIDs[index] = uuid.New()
	}

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:                  uuid.New(),
		ActorUserID:               uuid.New(),
		Slug:                      "security",
		Name:                      "安全团队",
		HumanOwnerUserIDs:         []uuid.UUID{uuid.New()},
		InitialDigitalEmployeeIDs: initialDigitalEmployeeIDs,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "digital employee capacity") {
		t.Fatalf("expected digital employee capacity error, got %v", err)
	}
	if repo.createTeamWithMembersCalled {
		t.Fatalf("expected over-capacity team not to reach repository")
	}
}

func TestCreateTeamAcceptsMetadataDisplay(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          tenantID,
		ActorUserID:       actorID,
		Slug:              "security",
		Name:              "安全团队",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
		Metadata: map[string]any{
			"display": map[string]any{
				"icon_key":   "security",
				"color_tone": "teal",
			},
		},
	})
	if err != nil {
		t.Fatalf("expected metadata display to pass validation, got %v", err)
	}
	display := repo.createdTeamWithMembers.Metadata["display"].(map[string]any)
	if display["icon_key"] != "security" || display["color_tone"] != "teal" {
		t.Fatalf("expected metadata display to be preserved, got %#v", display)
	}
}

func TestCreateTeamMetadataDisplayDoesNotMutateOrShareInput(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	ownerID := uuid.New()
	display := map[string]any{
		"icon_key":   " security ",
		"color_tone": " teal ",
	}
	metadata := map[string]any{"display": display}

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          uuid.New(),
		ActorUserID:       uuid.New(),
		Slug:              "security",
		Name:              "安全团队",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
		Metadata:          metadata,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if display["icon_key"] != " security " || display["color_tone"] != " teal " {
		t.Fatalf("expected input display map not to be trimmed, got %#v", display)
	}
	createdDisplay := repo.createdTeamWithMembers.Metadata["display"].(map[string]any)
	if createdDisplay["icon_key"] != "security" || createdDisplay["color_tone"] != "teal" {
		t.Fatalf("expected repository metadata display to be trimmed, got %#v", createdDisplay)
	}
	display["icon_key"] = "changed"
	if createdDisplay["icon_key"] != "security" {
		t.Fatalf("expected repository metadata display not to share input display map, got %#v", createdDisplay)
	}
	createdDisplay["color_tone"] = "blue"
	if display["color_tone"] != " teal " {
		t.Fatalf("expected input display map not to share repository metadata display, got %#v", display)
	}
}

func TestCreateTeamRejectsInvalidMetadataDisplay(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatal(err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	ownerID := uuid.New()
	longValue := strings.Repeat("x", 41)

	cases := []struct {
		name     string
		metadata map[string]any
	}{
		{name: "display is not object", metadata: map[string]any{"display": "security"}},
		{name: "icon key is not string", metadata: map[string]any{"display": map[string]any{"icon_key": 123}}},
		{name: "color tone is not string", metadata: map[string]any{"display": map[string]any{"color_tone": 123}}},
		{name: "icon key too long", metadata: map[string]any{"display": map[string]any{"icon_key": longValue}}},
		{name: "color tone too long", metadata: map[string]any{"display": map[string]any{"color_tone": longValue}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
				TenantID:          tenantID,
				ActorUserID:       actorID,
				Slug:              "security",
				Name:              "安全团队",
				HumanOwnerUserIDs: []uuid.UUID{ownerID},
				Metadata:          tc.metadata,
			})
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected invalid input, got %v", err)
			}
		})
	}
}

func TestCreateTeamRejectsPrivilegedInitialMemberRoles(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()
	actorID := uuid.New()
	targetID := uuid.New()
	repo.activeUsers[ownerID] = true
	repo.activeUsers[targetID] = true

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          uuid.New(),
		ActorUserID:       actorID,
		Slug:              "security",
		Name:              "安全团队",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
		InitialMembers:    []InitialTeamMemberInput{{UserID: targetID, Role: TeamRoleAdmin}},
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for admin initial member, got %v", err)
	}
	if repo.createTeamWithMembersCalled {
		t.Fatalf("expected invalid request not to reach repository")
	}
}

func TestCreateTeamRejectsOwnerDuplicatedAsInitialMember(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()
	actorID := uuid.New()
	repo.activeUsers[ownerID] = true

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          uuid.New(),
		ActorUserID:       actorID,
		Slug:              "security",
		Name:              "安全团队",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
		InitialMembers:    []InitialTeamMemberInput{{UserID: ownerID, Role: TeamRoleMember}},
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for duplicated owner, got %v", err)
	}
}

func TestUpdateTeamConstitutionPersistsHardRules(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          tenantID,
		ActorUserID:       uuid.New(),
		Slug:              "platform",
		Name:              "Platform",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	updated, err := svc.UpdateTeamConstitution(context.Background(), tenantID, team.Team.ID, map[string]any{
		"hard_rules": []string{"r1", "r2"},
	})
	if err != nil {
		t.Fatalf("update team constitution: %v", err)
	}

	hardRules, ok := updated.Constitution["hard_rules"].([]string)
	if !ok {
		t.Fatalf("expected []string hard_rules, got %#v", updated.Constitution["hard_rules"])
	}
	if !reflect.DeepEqual(hardRules, []string{"r1", "r2"}) {
		t.Fatalf("expected updated hard_rules, got %#v", hardRules)
	}

	reloaded, err := svc.GetTeam(context.Background(), tenantID, team.Team.ID)
	if err != nil {
		t.Fatalf("get team: %v", err)
	}
	reloadedHardRules, ok := reloaded.Constitution["hard_rules"].([]string)
	if !ok {
		t.Fatalf("expected persisted []string hard_rules, got %#v", reloaded.Constitution["hard_rules"])
	}
	if !reflect.DeepEqual(reloadedHardRules, []string{"r1", "r2"}) {
		t.Fatalf("expected persisted hard_rules, got %#v", reloadedHardRules)
	}
}

func TestListTeamsRejectsNegativeOffset(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.ListTeams(context.Background(), ListTeamsRequest{
		TenantID: uuid.New(),
		Offset:   -1,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for negative offset, got %v", err)
	}
	if repo.listTeamsCalled {
		t.Fatalf("expected invalid list request not to reach repository")
	}
}

func TestUpdateTeamRejectsEmptyName(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.UpdateTeam(context.Background(), UpdateTeamRequest{
		TenantID: uuid.New(),
		TeamID:   uuid.New(),
		Slug:     "ops",
		Name:     "   ",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for empty name, got %v", err)
	}
	if repo.updateTeamCalled {
		t.Fatalf("expected invalid update request not to reach repository")
	}
}

func TestUpdateTeamPreservesOwnerAndMetadataWhenOmitted(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          tenantID,
		ActorUserID:       uuid.New(),
		Slug:              "ops",
		Name:              "Ops",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
		Metadata:          map[string]any{"cost_center": "ops"},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	updated, err := svc.UpdateTeam(context.Background(), UpdateTeamRequest{
		TenantID: tenantID,
		TeamID:   team.Team.ID,
		Slug:     "platform-ops",
		Name:     "Platform Ops",
	})
	if err != nil {
		t.Fatalf("update team: %v", err)
	}

	if updated.HumanOwnerUserIDs == nil || updated.HumanOwnerUserIDs[0] != ownerID {
		t.Fatalf("expected owner to be preserved, got %#v", updated.HumanOwnerUserIDs)
	}
	if updated.Metadata["cost_center"] != "ops" {
		t.Fatalf("expected metadata to be preserved, got %#v", updated.Metadata)
	}
}

func TestUpdateTeamMetadataDisplayDoesNotMutateOrShareInput(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          tenantID,
		ActorUserID:       uuid.New(),
		Slug:              "ops",
		Name:              "Ops",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	display := map[string]any{
		"icon_key":   " ops ",
		"color_tone": " teal ",
	}
	metadata := map[string]any{"display": display}

	updated, err := svc.UpdateTeam(context.Background(), UpdateTeamRequest{
		TenantID: tenantID,
		TeamID:   team.Team.ID,
		Slug:     "platform-ops",
		Name:     "Platform Ops",
		Metadata: metadata,
	})
	if err != nil {
		t.Fatalf("update team: %v", err)
	}

	if display["icon_key"] != " ops " || display["color_tone"] != " teal " {
		t.Fatalf("expected input display map not to be trimmed, got %#v", display)
	}
	updatedDisplay := updated.Metadata["display"].(map[string]any)
	if updatedDisplay["icon_key"] != "ops" || updatedDisplay["color_tone"] != "teal" {
		t.Fatalf("expected updated metadata display to be trimmed, got %#v", updatedDisplay)
	}
	display["icon_key"] = "changed"
	storedDisplay := repo.teams[team.Team.ID].Metadata["display"].(map[string]any)
	if storedDisplay["icon_key"] != "ops" {
		t.Fatalf("expected stored metadata display not to share input display map, got %#v", storedDisplay)
	}
	updatedDisplay["color_tone"] = "blue"
	if display["color_tone"] != " teal " {
		t.Fatalf("expected input display map not to share updated metadata display, got %#v", display)
	}
}

func TestGetOverviewUsesTeamSummaryAggregate(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:          tenantID,
		ActorUserID:       uuid.New(),
		Slug:              "ops",
		Name:              "Ops",
		HumanOwnerUserIDs: []uuid.UUID{ownerID},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	repo.teamSummaries[team.Team.ID] = TeamListItemRecord{
		Team:                 *team.Team,
		MemberCount:          18,
		DigitalEmployeeCount: 6,
		CapabilityCount:      12,
		PendingDraftCount:    3,
	}

	overview, err := svc.GetOverview(context.Background(), tenantID, team.Team.ID)
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if !repo.getTeamSummaryCalled {
		t.Fatalf("expected overview to use team-scoped summary aggregate")
	}
	if repo.listTeamSummariesCalled {
		t.Fatalf("expected overview not to use paginated summary list")
	}
	if overview.MemberCount != 18 || overview.DigitalEmployeeCount != 6 || overview.CapabilityCount != 12 || overview.PendingItemCount != 3 {
		t.Fatalf("unexpected overview counts: %#v", overview)
	}
}

func TestListTeamSummariesDefaultsLimit(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.ListTeamSummaries(context.Background(), ListTeamsRequest{
		TenantID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("list team summaries: %v", err)
	}

	if !repo.listTeamSummariesCalled {
		t.Fatalf("expected list summary request to reach repository")
	}
	if repo.lastListTeamSummariesParams.Limit != 50 {
		t.Fatalf("expected default limit 50, got %d", repo.lastListTeamSummariesParams.Limit)
	}
}

func TestListTeamSummariesPassesGovernanceStatusFilter(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.ListTeamSummaries(context.Background(), ListTeamsRequest{
		TenantID:         uuid.New(),
		GovernanceStatus: GovernanceSummaryDraftPending,
	})
	if err != nil {
		t.Fatalf("list team summaries: %v", err)
	}

	if repo.lastListTeamSummariesParams.GovernanceStatus != GovernanceSummaryDraftPending {
		t.Fatalf("expected governance status filter to reach repository, got %#v", repo.lastListTeamSummariesParams)
	}
}

func TestAddTeamMemberRejectsPrivilegedRole(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, role := range []string{TeamRoleOwner, TeamRoleAdmin, TeamRoleApprover} {
		t.Run(role, func(t *testing.T) {
			repo.addTeamMemberCalled = false
			_, err := svc.AddTeamMember(context.Background(), AddTeamMemberRequest{
				TenantID: uuid.New(),
				TeamID:   uuid.New(),
				UserID:   uuid.New(),
				Role:     role,
			})

			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected invalid input for privileged role %q, got %v", role, err)
			}
			if repo.addTeamMemberCalled {
				t.Fatalf("expected privileged role %q not to reach repository", role)
			}
		})
	}
}

func TestRemoveTeamMemberRejectsLastOwner(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	membershipID := uuid.New()
	repo.teamMembers[membershipID] = TeamMemberRecord{
		MembershipID:     membershipID,
		TenantID:         tenantID,
		TeamID:           teamID,
		UserID:           uuid.New(),
		Username:         "owner",
		AccountStatus:    "active",
		Role:             TeamRoleOwner,
		MembershipStatus: "active",
	}

	err = svc.RemoveTeamMember(context.Background(), RemoveTeamMemberRequest{
		TenantID:     tenantID,
		TeamID:       teamID,
		MembershipID: membershipID,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input when removing last owner, got %v", err)
	}
	if repo.disableTeamMemberCalled {
		t.Fatalf("expected last owner not to be disabled")
	}
}

func TestApprovePrivilegedRoleRequestAddsRole(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	requestID := uuid.New()
	targetUserID := uuid.New()
	decidedBy := uuid.New()
	repo.roleRequests[requestID] = TeamMemberRoleRequestRecord{
		ID:            requestID,
		TenantID:      tenantID,
		TeamID:        teamID,
		TargetUserID:  targetUserID,
		RequestedRole: TeamRoleAdmin,
		RequestedBy:   uuid.New(),
		Status:        TeamMemberRoleRequestStatusPending,
		Reason:        "需要维护成员",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	request, err := svc.ApproveRoleRequest(context.Background(), DecideRoleRequestRequest{
		TenantID:       tenantID,
		TeamID:         teamID,
		RequestID:      requestID,
		DecidedBy:      decidedBy,
		DecisionReason: "允许",
	})
	if err != nil {
		t.Fatalf("approve role request: %v", err)
	}

	if request.Status != TeamMemberRoleRequestStatusApproved {
		t.Fatalf("expected approved request, got %q", request.Status)
	}
	if !repo.addTeamMemberCalled {
		t.Fatalf("expected approval to add requested team role")
	}
	if repo.lastAddTeamMemberParams.UserID != targetUserID || repo.lastAddTeamMemberParams.Role != TeamRoleAdmin {
		t.Fatalf("expected admin role add for target user, got %#v", repo.lastAddTeamMemberParams)
	}
}

type memoryRepository struct {
	teams                       map[uuid.UUID]TeamRecord
	teamSummaries               map[uuid.UUID]TeamListItemRecord
	teamMembers                 map[uuid.UUID]TeamMemberRecord
	roleRequests                map[uuid.UUID]TeamMemberRoleRequestRecord
	activeUsers                 map[uuid.UUID]bool
	auditEvents                 []memoryAuditEvent
	createTeamCalled            bool
	createTeamWithMembersCalled bool
	listTeamsCalled             bool
	listTeamSummariesCalled     bool
	getTeamSummaryCalled        bool
	updateTeamCalled            bool
	addTeamMemberCalled         bool
	disableTeamMemberCalled     bool
	decideRoleRequestCalled     bool
	lastListTeamSummariesParams ListTeamSummariesParams
	lastAddTeamMemberParams     AddTeamMemberParams
	createdTeamWithMembers      CreateTeamWithInitialMembersParams

	bindTeamDigitalEmployeeParams []BindTeamDigitalEmployeeParams
	bindTeamDigitalEmployeeErr    error
}

type memoryAuditEvent struct {
	Action       string
	ResourceType string
	ResourceID   uuid.UUID
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		teams:         map[uuid.UUID]TeamRecord{},
		teamSummaries: map[uuid.UUID]TeamListItemRecord{},
		teamMembers:   map[uuid.UUID]TeamMemberRecord{},
		roleRequests:  map[uuid.UUID]TeamMemberRoleRequestRecord{},
		activeUsers:   map[uuid.UUID]bool{},
		auditEvents:   []memoryAuditEvent{},
	}
}

func (r *memoryRepository) CreateTeam(_ context.Context, params CreateTeamParams) (TeamRecord, error) {
	r.createTeamCalled = true
	now := time.Now().UTC()
	record := TeamRecord{
		ID:                uuid.New(),
		TenantID:          params.TenantID,
		Slug:              params.Slug,
		Name:              params.Name,
		Status:            params.Status,
		HumanOwnerUserIDs: params.HumanOwnerUserIDs,
		Metadata:          cloneMap(params.Metadata),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	r.teams[record.ID] = record
	return record, nil
}

func (r *memoryRepository) CreateTeamWithInitialMembers(_ context.Context, params CreateTeamWithInitialMembersParams) (TeamRecord, error) {
	r.createTeamWithMembersCalled = true
	r.createdTeamWithMembers = params
	if len(r.activeUsers) > 0 {
		for _, ownerID := range params.OwnerUserIDs {
			if !r.activeUsers[ownerID] {
				return TeamRecord{}, ErrNotFound
			}
		}
	}
	for _, member := range params.InitialMembers {
		if len(r.activeUsers) > 0 && !r.activeUsers[member.UserID] {
			return TeamRecord{}, ErrNotFound
		}
	}
	now := time.Now().UTC()
	team := TeamRecord{
		ID:                uuid.New(),
		TenantID:          params.TenantID,
		Slug:              params.Slug,
		Name:              params.Name,
		Status:            params.Status,
		HumanOwnerUserIDs: params.OwnerUserIDs,
		Metadata:          cloneMap(params.Metadata),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	r.teams[team.ID] = team
	r.auditEvents = append(r.auditEvents, memoryAuditEvent{Action: "team.create", ResourceType: "team", ResourceID: team.ID})
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
	}
	for _, member := range params.InitialMembers {
		membership := TeamMemberRecord{
			MembershipID:     uuid.New(),
			TenantID:         params.TenantID,
			TeamID:           team.ID,
			UserID:           member.UserID,
			Role:             member.Role,
			MembershipStatus: "active",
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		r.teamMembers[membership.MembershipID] = membership
		r.auditEvents = append(r.auditEvents, memoryAuditEvent{
			Action:       "team.member.add",
			ResourceType: "team_member",
			ResourceID:   membership.MembershipID,
		})
	}
	return team, nil
}

func (r *memoryRepository) ListTeams(_ context.Context, params ListTeamsParams) ([]TeamRecord, error) {
	r.listTeamsCalled = true
	records := make([]TeamRecord, 0, len(r.teams))
	for _, record := range r.teams {
		if record.TenantID == params.TenantID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r *memoryRepository) ListTeamSummaries(_ context.Context, params ListTeamSummariesParams) ([]TeamListItemRecord, error) {
	r.listTeamSummariesCalled = true
	r.lastListTeamSummariesParams = params
	records := make([]TeamListItemRecord, 0, len(r.teams))
	for _, record := range r.teams {
		if record.TenantID == params.TenantID {
			records = append(records, TeamListItemRecord{Team: record})
		}
	}
	return records, nil
}

func (r *memoryRepository) GetTeamSummary(_ context.Context, tenantID, teamID uuid.UUID) (TeamListItemRecord, error) {
	r.getTeamSummaryCalled = true
	if record, ok := r.teamSummaries[teamID]; ok && record.TenantID == tenantID {
		return record, nil
	}
	record, ok := r.teams[teamID]
	if !ok || record.TenantID != tenantID {
		return TeamListItemRecord{}, ErrNotFound
	}
	var memberCount int32
	for _, member := range r.teamMembers {
		if member.TenantID == tenantID && member.TeamID == teamID && member.MembershipStatus == "active" {
			memberCount++
		}
	}
	return TeamListItemRecord{Team: record, MemberCount: memberCount}, nil
}

func (r *memoryRepository) GetTeam(_ context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error) {
	record, ok := r.teams[teamID]
	if !ok || record.TenantID != tenantID {
		return TeamRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) UpdateTeam(_ context.Context, params UpdateTeamParams) (TeamRecord, error) {
	r.updateTeamCalled = true
	record, ok := r.teams[params.TeamID]
	if !ok || record.TenantID != params.TenantID {
		return TeamRecord{}, ErrNotFound
	}
	record.Slug = params.Slug
	record.Name = params.Name
	record.HumanOwnerUserIDs = params.HumanOwnerUserIDs
	record.Metadata = cloneMap(params.Metadata)
	record.UpdatedAt = time.Now().UTC()
	r.teams[record.ID] = record
	return record, nil
}

func (r *memoryRepository) UpdateTeamConstitution(_ context.Context, tenantID, teamID uuid.UUID, constitution map[string]any) (TeamRecord, error) {
	record, ok := r.teams[teamID]
	if !ok || record.TenantID != tenantID {
		return TeamRecord{}, ErrNotFound
	}
	record.Constitution = cloneMap(constitution)
	record.UpdatedAt = time.Now().UTC()
	r.teams[record.ID] = record
	return record, nil
}

func (r *memoryRepository) DeleteTeam(_ context.Context, tenantID, teamID uuid.UUID) error {
	if _, ok := r.teams[teamID]; !ok {
		return ErrNotFound
	}
	delete(r.teams, teamID)
	return nil
}

func (r *memoryRepository) ListTeamMembers(_ context.Context, params ListTeamMembersParams) ([]TeamMemberRecord, error) {
	records := make([]TeamMemberRecord, 0, len(r.teamMembers))
	for _, record := range r.teamMembers {
		if record.TenantID == params.TenantID && record.TeamID == params.TeamID && record.MembershipStatus == "active" {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r *memoryRepository) GetTeamMember(_ context.Context, tenantID, teamID, membershipID uuid.UUID) (TeamMemberRecord, error) {
	record, ok := r.teamMembers[membershipID]
	if !ok || record.TenantID != tenantID || record.TeamID != teamID || record.MembershipStatus != "active" {
		return TeamMemberRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) BindTeamDigitalEmployee(_ context.Context, params BindTeamDigitalEmployeeParams) error {
	r.bindTeamDigitalEmployeeParams = append(r.bindTeamDigitalEmployeeParams, params)
	return r.bindTeamDigitalEmployeeErr
}

func (r *memoryRepository) AddTeamMember(_ context.Context, params AddTeamMemberParams) (TeamMemberRecord, error) {
	r.addTeamMemberCalled = true
	r.lastAddTeamMemberParams = params
	now := time.Now().UTC()
	record := TeamMemberRecord{
		MembershipID:     uuid.New(),
		TenantID:         params.TenantID,
		TeamID:           params.TeamID,
		UserID:           params.UserID,
		Username:         "member",
		AccountStatus:    "active",
		Role:             params.Role,
		MembershipStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.teamMembers[record.MembershipID] = record
	return record, nil
}

func (r *memoryRepository) DisableTeamMemberRole(_ context.Context, params DisableTeamMemberRoleParams) (TeamMemberRecord, error) {
	r.disableTeamMemberCalled = true
	record, ok := r.teamMembers[params.MembershipID]
	if !ok || record.TenantID != params.TenantID || record.TeamID != params.TeamID {
		return TeamMemberRecord{}, ErrNotFound
	}
	record.MembershipStatus = "disabled"
	record.UpdatedAt = time.Now().UTC()
	r.teamMembers[record.MembershipID] = record
	return record, nil
}

func (r *memoryRepository) CountTeamOwners(_ context.Context, tenantID, teamID uuid.UUID) (int32, error) {
	var count int32
	for _, record := range r.teamMembers {
		if record.TenantID == tenantID && record.TeamID == teamID && record.Role == TeamRoleOwner && record.MembershipStatus == "active" {
			count++
		}
	}
	return count, nil
}

func (r *memoryRepository) CreateTeamMemberRoleRequest(_ context.Context, params CreateTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error) {
	now := time.Now().UTC()
	record := TeamMemberRoleRequestRecord{
		ID:            uuid.New(),
		TenantID:      params.TenantID,
		TeamID:        params.TeamID,
		TargetUserID:  params.TargetUserID,
		RequestedRole: params.RequestedRole,
		RequestedBy:   params.RequestedBy,
		Status:        TeamMemberRoleRequestStatusPending,
		Reason:        params.Reason,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	r.roleRequests[record.ID] = record
	return record, nil
}

func (r *memoryRepository) GetTeamMemberRoleRequest(_ context.Context, tenantID, teamID, requestID uuid.UUID) (TeamMemberRoleRequestRecord, error) {
	record, ok := r.roleRequests[requestID]
	if !ok || record.TenantID != tenantID || record.TeamID != teamID || record.Status != TeamMemberRoleRequestStatusPending {
		return TeamMemberRoleRequestRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) ListTeamMemberRoleRequests(_ context.Context, params ListTeamMemberRoleRequestsParams) ([]TeamMemberRoleRequestRecord, error) {
	records := make([]TeamMemberRoleRequestRecord, 0, len(r.roleRequests))
	for _, record := range r.roleRequests {
		if record.TenantID == params.TenantID && record.TeamID == params.TeamID && (params.Status == "" || record.Status == params.Status) {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r *memoryRepository) ApproveTeamMemberRoleRequest(ctx context.Context, params DecideTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error) {
	pending, err := r.GetTeamMemberRoleRequest(ctx, params.TenantID, params.TeamID, params.RequestID)
	if err != nil {
		return TeamMemberRoleRequestRecord{}, err
	}
	if _, err := r.AddTeamMember(ctx, AddTeamMemberParams{
		TenantID: pending.TenantID,
		TeamID:   pending.TeamID,
		UserID:   pending.TargetUserID,
		Role:     pending.RequestedRole,
	}); err != nil {
		return TeamMemberRoleRequestRecord{}, err
	}
	params.Status = TeamMemberRoleRequestStatusApproved
	return r.DecideTeamMemberRoleRequest(ctx, params)
}

func (r *memoryRepository) DecideTeamMemberRoleRequest(_ context.Context, params DecideTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error) {
	r.decideRoleRequestCalled = true
	record, ok := r.roleRequests[params.RequestID]
	if !ok || record.TenantID != params.TenantID || record.TeamID != params.TeamID || record.Status != TeamMemberRoleRequestStatusPending {
		return TeamMemberRoleRequestRecord{}, ErrNotFound
	}
	now := time.Now().UTC()
	record.Status = params.Status
	record.DecidedBy = &params.DecidedBy
	record.DecidedAt = &now
	record.DecisionReason = params.DecisionReason
	record.UpdatedAt = now
	r.roleRequests[record.ID] = record
	return record, nil
}

type fakeTeamAuditReader struct{}

func (r *fakeTeamAuditReader) ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*audit.Event, error) {
	return []*audit.Event{}, nil
}

// --- BindTeamDigitalEmployee (团队归属参与门禁的归队入口) ---

func TestBindTeamDigitalEmployeeBindsIntoActiveTeam(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo, &fakeTeamAuditReader{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID, teamID, employeeID, actorID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	repo.teams[teamID] = TeamRecord{ID: teamID, TenantID: tenantID, Status: TeamStatusActive}

	if err := service.BindTeamDigitalEmployee(context.Background(), BindTeamDigitalEmployeeRequest{
		TenantID:    tenantID,
		TeamID:      teamID,
		EmployeeID:  employeeID,
		ActorUserID: actorID,
	}); err != nil {
		t.Fatalf("bind team digital employee: %v", err)
	}
	if len(repo.bindTeamDigitalEmployeeParams) != 1 {
		t.Fatalf("expected one bind call, got %d", len(repo.bindTeamDigitalEmployeeParams))
	}
	got := repo.bindTeamDigitalEmployeeParams[0]
	if got.TeamID != teamID || got.EmployeeID != employeeID || got.ActorUserID != actorID {
		t.Fatalf("unexpected bind params: %#v", got)
	}
}

func TestBindTeamDigitalEmployeeRejectsInactiveTeam(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo, &fakeTeamAuditReader{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID, teamID := uuid.New(), uuid.New()
	repo.teams[teamID] = TeamRecord{ID: teamID, TenantID: tenantID, Status: TeamStatusDisabled}

	err = service.BindTeamDigitalEmployee(context.Background(), BindTeamDigitalEmployeeRequest{
		TenantID:    tenantID,
		TeamID:      teamID,
		EmployeeID:  uuid.New(),
		ActorUserID: uuid.New(),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for inactive team, got %v", err)
	}
	if len(repo.bindTeamDigitalEmployeeParams) != 0 {
		t.Fatalf("expected no bind call, got %d", len(repo.bindTeamDigitalEmployeeParams))
	}
}

func TestBindTeamDigitalEmployeeRejectsUnknownTeam(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo, &fakeTeamAuditReader{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = service.BindTeamDigitalEmployee(context.Background(), BindTeamDigitalEmployeeRequest{
		TenantID:    uuid.New(),
		TeamID:      uuid.New(),
		EmployeeID:  uuid.New(),
		ActorUserID: uuid.New(),
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found for unknown team, got %v", err)
	}
}
