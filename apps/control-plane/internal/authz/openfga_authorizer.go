package authz

import "context"

type OpenFGAAuthorizerOptions struct {
	Recorder DecisionRecorder
	StoreID  string
	ModelID  string
}

type OpenFGAAuthorizer struct {
	checker OpenFGAChecker
	options OpenFGAAuthorizerOptions
}

func NewOpenFGAAuthorizer(checker OpenFGAChecker, options OpenFGAAuthorizerOptions) *OpenFGAAuthorizer {
	return &OpenFGAAuthorizer{checker: checker, options: options}
}

func (a *OpenFGAAuthorizer) AuthzEngineStatus() EngineStatus {
	return EngineStatus{
		Engine:         "openfga",
		Status:         "healthy",
		EngineVersion:  "openfga-direct-v1",
		OpenFGAStoreID: a.options.StoreID,
		OpenFGAModelID: a.options.ModelID,
	}
}

func (a *OpenFGAAuthorizer) CheckBulkTeamActions(ctx context.Context, req BulkTeamActionsRequest) ([]string, error) {
	allowed := make([]string, 0, len(req.Actions))
	for _, action := range req.Actions {
		decision, err := a.Check(ctx, CheckRequest{
			Actor:    req.Actor,
			Action:   action,
			Resource: ResourceRef{Type: ResourceTeam, ID: req.TeamID.String()},
			TenantID: req.TenantID,
			TeamID:   &req.TeamID,
		})
		if err != nil {
			return nil, err
		}
		if decision.Allowed {
			allowed = append(allowed, action)
		}
	}
	return allowed, nil
}

func (a *OpenFGAAuthorizer) Check(ctx context.Context, req CheckRequest) (Decision, error) {
	if a == nil {
		return Decision{Allowed: false, Reason: "authorizer is not configured", RequiresAudit: true}, nil
	}
	decision := Decision{
		Allowed:       false,
		Reason:        "authorizer is not configured",
		RequiresAudit: true,
		Snapshot:      a.snapshot(nil),
	}
	if a.checker != nil {
		if check, ok := OpenFGACheckForRequest(req); ok {
			allowed, err := a.checker.Check(ctx, check)
			switch {
			case err != nil:
				decision = Decision{
					Allowed:       false,
					Reason:        "openfga check failed",
					RequiresAudit: true,
					Snapshot:      a.snapshot(map[string]any{"openfga_error": err.Error()}),
				}
			case allowed:
				decision = Decision{
					Allowed:     true,
					Reason:      ReasonAllowed,
					MatchedRule: "openfga." + check.Relation,
					Snapshot: a.snapshot(map[string]any{
						"openfga_relation": check.Relation,
						"openfga_object":   check.Object,
					}),
				}
			default:
				decision = Decision{
					Allowed:       false,
					Reason:        ReasonNoMembership,
					MatchedRule:   "openfga." + check.Relation,
					RequiresAudit: true,
					Snapshot: a.snapshot(map[string]any{
						"openfga_relation": check.Relation,
						"openfga_object":   check.Object,
					}),
				}
			}
		} else {
			decision = Decision{
				Allowed:       false,
				Reason:        ReasonUnsupportedAction,
				RequiresAudit: true,
				Snapshot:      a.snapshot(nil),
			}
		}
	}
	if err := a.record(ctx, req, decision); err != nil {
		return Decision{}, err
	}
	return decision, nil
}

func (a *OpenFGAAuthorizer) snapshot(extra map[string]any) map[string]any {
	snapshot := map[string]any{
		"engine":           "openfga",
		"openfga_store_id": a.options.StoreID,
		"openfga_model_id": a.options.ModelID,
	}
	for key, value := range extra {
		snapshot[key] = value
	}
	return snapshot
}

func (a *OpenFGAAuthorizer) record(ctx context.Context, req CheckRequest, decision Decision) error {
	if a == nil || a.options.Recorder == nil {
		return nil
	}
	return a.options.Recorder.RecordDecision(ctx, DecisionRecord{
		TenantID:     req.TenantID,
		TeamID:       req.TeamID,
		ActorType:    req.Actor.Type,
		ActorID:      req.Actor.ID,
		Action:       req.Action,
		ResourceType: req.Resource.Type,
		ResourceID:   req.Resource.ID,
		Allowed:      decision.Allowed,
		Reason:       decision.Reason,
		MatchedRule:  decision.MatchedRule,
		Engine:       "openfga",
		Snapshot:     decision.Snapshot,
	})
}
