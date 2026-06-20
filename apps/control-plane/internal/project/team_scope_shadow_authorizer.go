package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

type TeamScopeShadowOptions struct {
	Recorder authz.DecisionRecorder
	StoreID  string
	ModelID  string
}

type TeamScopeShadowAuthorizer struct {
	primary ProjectTeamScopeAuthorizer
	shadow  authz.OpenFGAChecker
	options TeamScopeShadowOptions
}

func NewTeamScopeShadowAuthorizer(primary ProjectTeamScopeAuthorizer, shadow authz.OpenFGAChecker, options TeamScopeShadowOptions) *TeamScopeShadowAuthorizer {
	return &TeamScopeShadowAuthorizer{primary: primary, shadow: shadow, options: options}
}

func (a *TeamScopeShadowAuthorizer) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	if a == nil || a.primary == nil {
		return false, nil
	}
	allowed, err := a.primary.CanUseTeamForProject(ctx, tenantID, userID, teamID)
	if err != nil {
		return allowed, err
	}
	if a.shadow == nil {
		return allowed, nil
	}
	shadowAllowed, shadowErr := a.shadow.Check(ctx, authz.OpenFGACheck{
		User:     authz.ActorUser + ":" + userID.String(),
		Relation: authz.OpenFGARelationProjectScopeUser,
		Object:   authz.ResourceTeam + ":" + teamID.String(),
	})
	if shadowErr != nil {
		a.record(ctx, tenantID, userID, teamID, allowed, false, shadowErr.Error())
		return allowed, nil
	}
	if shadowAllowed != allowed {
		a.record(ctx, tenantID, userID, teamID, allowed, shadowAllowed, "")
	}
	return allowed, nil
}

func (a *TeamScopeShadowAuthorizer) record(ctx context.Context, tenantID, userID, teamID uuid.UUID, dbAllowed, openFGAAllowed bool, openFGAError string) {
	if a.options.Recorder == nil {
		return
	}
	snapshot := map[string]any{
		"diff":               openFGAError == "" && dbAllowed != openFGAAllowed,
		"db_allowed":         dbAllowed,
		"openfga_allowed":    openFGAAllowed,
		"openfga_store_id":   a.options.StoreID,
		"openfga_model_id":   a.options.ModelID,
		"shadow_engine_mode": "openfga_shadow",
	}
	if openFGAError != "" {
		snapshot["openfga_error"] = openFGAError
		snapshot["diff"] = true
	}
	_ = a.options.Recorder.RecordDecision(ctx, authz.DecisionRecord{
		TenantID:     tenantID,
		ActorType:    authz.ActorUser,
		ActorID:      userID.String(),
		Action:       "project.team_scope.use",
		ResourceType: authz.ResourceTeam,
		ResourceID:   teamID.String(),
		Allowed:      openFGAAllowed,
		Reason:       "openfga shadow decision differs from db decision",
		MatchedRule:  "openfga.project_team_scope",
		Engine:       "openfga_shadow",
		Snapshot:     snapshot,
	})
}
