package authz

import (
	"context"
)

type OpenFGAChecker interface {
	Check(ctx context.Context, check OpenFGACheck) (bool, error)
}

type ShadowOptions struct {
	Recorder DecisionRecorder
	StoreID  string
	ModelID  string
}

type ShadowAuthorizer struct {
	primary Authorizer
	shadow  OpenFGAChecker
	options ShadowOptions
}

func NewShadowAuthorizer(primary Authorizer, shadow OpenFGAChecker, options ShadowOptions) *ShadowAuthorizer {
	return &ShadowAuthorizer{primary: primary, shadow: shadow, options: options}
}

func (a *ShadowAuthorizer) AuthzEngineStatus() EngineStatus {
	return EngineStatus{
		Engine:         "openfga_shadow",
		Status:         "healthy",
		EngineVersion:  "openfga-shadow-v1",
		OpenFGAStoreID: a.options.StoreID,
		OpenFGAModelID: a.options.ModelID,
	}
}

func (a *ShadowAuthorizer) CheckBulkTeamActions(ctx context.Context, req BulkTeamActionsRequest) ([]string, error) {
	if a == nil || a.primary == nil {
		return nil, nil
	}
	return a.primary.CheckBulkTeamActions(ctx, req)
}

func (a *ShadowAuthorizer) Check(ctx context.Context, req CheckRequest) (Decision, error) {
	if a == nil || a.primary == nil {
		return Decision{Allowed: false, Reason: "authorizer is not configured", RequiresAudit: true}, nil
	}
	primaryDecision, primaryErr := a.primary.Check(ctx, req)
	if primaryErr != nil {
		return primaryDecision, primaryErr
	}
	if a.shadow == nil {
		return primaryDecision, nil
	}
	check, ok := OpenFGACheckForRequest(req)
	if !ok {
		return primaryDecision, nil
	}
	allowed, shadowErr := a.shadow.Check(ctx, check)
	if shadowErr != nil {
		a.record(ctx, req, primaryDecision, false, shadowErr.Error())
		return primaryDecision, nil
	}
	if allowed != primaryDecision.Allowed {
		a.record(ctx, req, primaryDecision, allowed, "")
	}
	return primaryDecision, nil
}

func (a *ShadowAuthorizer) record(ctx context.Context, req CheckRequest, dbDecision Decision, openFGAAllowed bool, openFGAError string) {
	if a.options.Recorder == nil {
		return
	}
	snapshot := map[string]any{
		"diff":               openFGAError == "" && dbDecision.Allowed != openFGAAllowed,
		"db_allowed":         dbDecision.Allowed,
		"openfga_allowed":    openFGAAllowed,
		"openfga_store_id":   a.options.StoreID,
		"openfga_model_id":   a.options.ModelID,
		"db_reason":          dbDecision.Reason,
		"db_matched_rule":    dbDecision.MatchedRule,
		"shadow_engine_mode": "openfga_shadow",
	}
	if openFGAError != "" {
		snapshot["openfga_error"] = openFGAError
		snapshot["diff"] = true
	}
	_ = a.options.Recorder.RecordDecision(ctx, DecisionRecord{
		TenantID:     req.TenantID,
		TeamID:       req.TeamID,
		ActorType:    req.Actor.Type,
		ActorID:      req.Actor.ID,
		Action:       req.Action,
		ResourceType: req.Resource.Type,
		ResourceID:   req.Resource.ID,
		Allowed:      openFGAAllowed,
		Reason:       "openfga shadow decision differs from db decision",
		MatchedRule:  "openfga.shadow",
		Engine:       "openfga_shadow",
		Snapshot:     snapshot,
	})
}
