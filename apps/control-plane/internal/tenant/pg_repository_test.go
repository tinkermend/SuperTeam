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
	if listRecord.HumanOwner == nil || listRecord.HumanOwner.Avatar == nil || listRecord.HumanOwner.Avatar.Seed != "user:owner" {
		t.Fatalf("expected list owner avatar seed fallback, got %#v", listRecord.HumanOwner)
	}

	getRow := getTenantTeamSummaryRowFromListRow(row)
	getRecord, err := teamListItemRecordFromGetSummaryQuery(getRow)
	if err != nil {
		t.Fatalf("get team item record: %v", err)
	}
	if getRecord.HumanOwner == nil || getRecord.HumanOwner.Avatar == nil || getRecord.HumanOwner.Avatar.Seed != "user:owner" {
		t.Fatalf("expected get owner avatar seed fallback, got %#v", getRecord.HumanOwner)
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
		OwnerUserID:         uuid.NullUUID{UUID: uuid.New(), Valid: true},
		OwnerUsername:       pgtype.Text{String: username, Valid: true},
		OwnerDisplayName:    pgtype.Text{String: "Owner", Valid: true},
		OwnerEmail:          pgtype.Text{String: "owner@example.com", Valid: true},
		OwnerStatus:         pgtype.Text{String: "active", Valid: true},
		OwnerAvatarProvider: pgtype.Text{String: "dicebear", Valid: true},
		OwnerAvatarStyle:    pgtype.Text{String: "adventurer", Valid: true},
		OwnerAvatarOptions:  []byte(`{"backgroundColor":["e6fbf5"]}`),
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
		HumanOwnerUserID:     row.HumanOwnerUserID,
		Metadata:             row.Metadata,
		ArchivedAt:           row.ArchivedAt,
		DisabledAt:           row.DisabledAt,
		DeletedAt:            row.DeletedAt,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		OwnerUserID:          row.OwnerUserID,
		OwnerUsername:        row.OwnerUsername,
		OwnerDisplayName:     row.OwnerDisplayName,
		OwnerEmail:           row.OwnerEmail,
		OwnerStatus:          row.OwnerStatus,
		OwnerAvatarProvider:  row.OwnerAvatarProvider,
		OwnerAvatarStyle:     row.OwnerAvatarStyle,
		OwnerAvatarSeed:      row.OwnerAvatarSeed,
		OwnerAvatarOptions:   row.OwnerAvatarOptions,
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
