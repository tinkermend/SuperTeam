package tenant

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestTeamSummaryAvatarFallsBackToUsernameWhenSeedMissing(t *testing.T) {
	row := tenantTeamSummaryRowWithLegacyOwnerAvatar("owner")

	listRecord, err := teamListItemRecordFromQuery(row)
	if err != nil {
		t.Fatalf("list team item record: %v", err)
	}
	if listRecord.HumanOwners == nil || listRecord.HumanOwners[0].Avatar == nil || listRecord.HumanOwners[0].Avatar.Seed != "user:owner" {
		t.Fatalf("expected list owner avatar seed fallback, got %#v", listRecord.HumanOwners)
	}

	getRow := getTenantTeamSummaryRowFromListRow(row)
	getRecord, err := teamListItemRecordFromGetSummaryQuery(getRow)
	if err != nil {
		t.Fatalf("get team item record: %v", err)
	}
	if getRecord.HumanOwners == nil || getRecord.HumanOwners[0].Avatar == nil || getRecord.HumanOwners[0].Avatar.Seed != "user:owner" {
		t.Fatalf("expected get owner avatar seed fallback, got %#v", getRecord.HumanOwners)
	}
}

func TestTeamMemberAvatarFallsBackToUsernameWhenSeedMissing(t *testing.T) {
	row := listTeamMembersRowWithLegacyAvatar("member")

	listRecord, err := teamMemberRecordFromListRow(row)
	if err != nil {
		t.Fatalf("list team member record: %v", err)
	}
	if listRecord.Avatar == nil || listRecord.Avatar.Seed != "user:member" {
		t.Fatalf("expected list member avatar seed fallback, got %#v", listRecord.Avatar)
	}

	getRow := getTeamMemberRowFromListRow(row)
	getRecord, err := teamMemberRecordFromGetRow(getRow)
	if err != nil {
		t.Fatalf("get team member record: %v", err)
	}
	if getRecord.Avatar == nil || getRecord.Avatar.Seed != "user:member" {
		t.Fatalf("expected get member avatar seed fallback, got %#v", getRecord.Avatar)
	}
}

func tenantTeamSummaryRowWithLegacyOwnerAvatar(username string) queries.ListTenantTeamSummariesRow {
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	return queries.ListTenantTeamSummariesRow{
		ID:                  uuid.New(),
		TenantID:            uuid.New(),
		Slug:                "ops",
		Name:                "Ops",
		Status:              string(TeamStatusActive),
		Metadata:            []byte(`{}`),
		CreatedAt:           now,
		UpdatedAt:           now,
		HumanOwners: []byte(`[{"id": "00000000-0000-0000-0000-000000000000", "username": "owner", "status": "active", "avatar_provider": "dicebear", "avatar_style": "adventurer"}]`),
																GovernanceStatus:    string(GovernanceSummaryActive),
	}
}

func getTenantTeamSummaryRowFromListRow(row queries.ListTenantTeamSummariesRow) queries.GetTenantTeamSummaryRow {
	return queries.GetTenantTeamSummaryRow{
		ID:                   row.ID,
		TenantID:             row.TenantID,
		Slug:                 row.Slug,
		Name:                 row.Name,
		Status:               row.Status,
		HumanOwnerUserIds:     row.HumanOwnerUserIds,
		Metadata:             row.Metadata,
		ArchivedAt:           row.ArchivedAt,
		DisabledAt:           row.DisabledAt,
		DeletedAt:            row.DeletedAt,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		HumanOwners:          row.HumanOwners,
																		MemberCount:          row.MemberCount,
		DigitalEmployeeCount: row.DigitalEmployeeCount,
		CapabilityCount:      row.CapabilityCount,
		CurrentRevision:      row.CurrentRevision,
		PendingDraftCount:    row.PendingDraftCount,
		GovernanceStatus:     row.GovernanceStatus,
		RiskSummary:          row.RiskSummary,
	}
}

func listTeamMembersRowWithLegacyAvatar(username string) queries.ListTeamMembersRow {
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	return queries.ListTeamMembersRow{
		MembershipID:     uuid.New(),
		TenantID:         uuid.New(),
		TeamID:           uuid.NullUUID{UUID: uuid.New(), Valid: true},
		UserID:           uuid.New(),
		Username:         username,
		DisplayName:      pgtype.Text{String: "Member", Valid: true},
		Email:            pgtype.Text{String: "member@example.com", Valid: true},
		AccountStatus:    "active",
		AvatarProvider:   "dicebear",
		AvatarStyle:      "adventurer",
		AvatarOptions:    []byte(`{"backgroundColor":["e6fbf5"]}`),
		Role:             TeamRoleMember,
		MembershipStatus: "active",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func getTeamMemberRowFromListRow(row queries.ListTeamMembersRow) queries.GetTeamMemberRow {
	return queries.GetTeamMemberRow{
		MembershipID:     row.MembershipID,
		TenantID:         row.TenantID,
		TeamID:           row.TeamID,
		UserID:           row.UserID,
		Username:         row.Username,
		DisplayName:      row.DisplayName,
		Email:            row.Email,
		AccountStatus:    row.AccountStatus,
		AvatarProvider:   row.AvatarProvider,
		AvatarStyle:      row.AvatarStyle,
		AvatarSeed:       row.AvatarSeed,
		AvatarOptions:    row.AvatarOptions,
		Role:             row.Role,
		MembershipStatus: row.MembershipStatus,
		DisabledAt:       row.DisabledAt,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
