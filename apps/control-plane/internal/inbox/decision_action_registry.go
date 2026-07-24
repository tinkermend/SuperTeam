package inbox

// Decision action → handler registry (spec §4.4, treating F1).
//
// The root cause of F1 was that the inbox could render an action button with no
// server handler behind it: the button "worked" (submitted 200) but wrote no
// business fact and stranded the demand. To make "能点" and "点了有用" contractual,
// every project-decision action the inbox emits is declared here together with the
// server handler that executes it, and decision_action_registry_test.go asserts:
//
//   1. DecisionActions only ever emits actions declared in this registry
//      (structural: DecisionActions derives its special-kind sets from here, and
//      the generic set from DefaultActions which is also registered);
//   2. every declared action names a handler that is in implementedDecisionHandlers
//      (a hand-maintained set of handlers the Control Plane actually implements),
//      so an action can never be added without wiring a real handler.
//
// Handler identifiers are documentation-grade references to the concrete server
// code (project.Service.ResolveDecision's dispatch and the coordinator handlers it
// drives). Adding an action with an unknown handler id fails the CI assertion.

const (
	// handlerResolveDemandAcceptance: project.Service.resolveDemandAcceptanceDecision
	// (approved → SignAllPendingDemandCriteria, rejected → first pending unsatisfied).
	handlerResolveDemandAcceptance = "project.Service.resolveDemandAcceptanceDecision"
	// handlerPlanningGapDecision: projectcoordination.handlePlanningGapDecision
	// (restaffed / exempted reopen+replan; rejected closes the gap).
	handlerPlanningGapDecision = "projectcoordination.handlePlanningGapDecision"
	// handlerPlanningFailedDecision: project.Service.resolvePlanningFailedDecision
	// (retry_planning/reassign reopen+replan; close_demand cancels the demand).
	handlerPlanningFailedDecision = "project.Service.resolvePlanningFailedDecision"
	// handlerFailureRecoveryDecision: projectcoordination.applyFailureRecoveryDecision
	// (retry re-dispatches; cancel_downstream cancels dependents).
	handlerFailureRecoveryDecision = "projectcoordination.applyFailureRecoveryDecision"
	// handlerResolveDecisionGeneric: project.Service.ResolveDecision generic path —
	// approved / rejected / needs_more_evidence for plan_review, project_acceptance,
	// project_task_* gates etc., dispatched by decision type inside the coordinator.
	handlerResolveDecisionGeneric = "project.Service.ResolveDecision.generic"
)

// implementedDecisionHandlers is the set of handler ids the Control Plane actually
// implements. The registry test asserts every emitted action's handler is here; a
// dangling handler id (typo, or a handler removed without removing its action)
// fails the build.
var implementedDecisionHandlers = map[string]struct{}{
	handlerResolveDemandAcceptance: {},
	handlerPlanningGapDecision:     {},
	handlerPlanningFailedDecision:  {},
	handlerFailureRecoveryDecision: {},
	handlerResolveDecisionGeneric:  {},
}

// registeredDecisionAction pairs an inbox Action with the handler that executes it.
type registeredDecisionAction struct {
	action  Action
	handler string
}

// decisionActionRegistry declares the action set for decision kinds whose inbox
// vocabulary differs from the generic approved/rejected/needs_more_evidence set.
// Kinds NOT listed here fall back to the generic set (genericDecisionActions),
// which is itself handler-covered by handlerResolveDecisionGeneric.
var decisionActionRegistry = map[string][]registeredDecisionAction{
	"demand_acceptance": {
		{action: Action{Key: "approved", Label: "同意", Tone: "positive", Metadata: map[string]any{"decision": "approved"}}, handler: handlerResolveDemandAcceptance},
		{action: Action{Key: "rejected", Label: "驳回", Tone: "destructive", RequiresComment: true, Metadata: map[string]any{"decision": "rejected"}}, handler: handlerResolveDemandAcceptance},
	},
	// §5.3 closure_confirm vocabulary (decision_type stays project_acceptance).
	"project_acceptance": {
		{action: Action{Key: "approved", Label: "确认结项并归档", Tone: "positive", Metadata: map[string]any{"decision": "approved"}}, handler: handlerResolveDecisionGeneric},
		{action: Action{Key: "rejected", Label: "退回返工", Tone: "destructive", RequiresComment: true, Metadata: map[string]any{"decision": "rejected"}}, handler: handlerResolveDecisionGeneric},
		{action: Action{Key: "needs_more_evidence", Label: "要求补证", Tone: "warning", RequiresComment: true, Metadata: map[string]any{"decision": "needs_more_evidence"}}, handler: handlerResolveDecisionGeneric},
	},
	"planning_gap": {
		{action: Action{Key: "restaffed", Label: "已补员，重新规划", Tone: "positive", Metadata: map[string]any{"decision": "restaffed"}}, handler: handlerPlanningGapDecision},
		{action: Action{Key: "exempted", Label: "豁免约束并重规划", Tone: "positive", Metadata: map[string]any{"decision": "exempted"}}, handler: handlerPlanningGapDecision},
		{action: Action{Key: "rejected", Label: "关闭", Tone: "destructive", Metadata: map[string]any{"decision": "rejected"}}, handler: handlerPlanningGapDecision},
	},
	// §5.5 planning_failed vocabulary (planner timeout / upstream error after retries).
	"planning_failed": {
		{action: Action{Key: "retry_planning", Label: "重新规划", Tone: "positive", Metadata: map[string]any{"decision": "retry_planning"}}, handler: handlerPlanningFailedDecision},
		{action: Action{Key: "reassign", Label: "已补员，重新规划", Tone: "positive", Metadata: map[string]any{"decision": "reassign"}}, handler: handlerPlanningFailedDecision},
		{action: Action{Key: "close_demand", Label: "关闭需求", Tone: "destructive", RequiresComment: true, Metadata: map[string]any{"decision": "close_demand"}}, handler: handlerPlanningFailedDecision},
	},
	"task_failure_recovery": {
		{action: Action{Key: "retry", Label: "重试任务", Tone: "positive", Metadata: map[string]any{"decision": "retry"}}, handler: handlerFailureRecoveryDecision},
		{action: Action{Key: "cancel_downstream", Label: "取消下游", Tone: "destructive", RequiresComment: true, Metadata: map[string]any{"decision": "cancel_downstream"}}, handler: handlerFailureRecoveryDecision},
	},
}

// genericDecisionActions is the generic project-decision vocabulary (the same set
// DefaultActions(ItemTypeProjectDecision) returns), each mapped to the generic
// ResolveDecision handler. Declared here so the registry test can assert handler
// coverage for the generic keys too.
var genericDecisionActions = []registeredDecisionAction{
	{action: Action{Key: "approved", Label: "同意", Tone: "positive", Metadata: map[string]any{"decision": "approved"}}, handler: handlerResolveDecisionGeneric},
	{action: Action{Key: "rejected", Label: "驳回", Tone: "destructive", RequiresComment: true, Metadata: map[string]any{"decision": "rejected"}}, handler: handlerResolveDecisionGeneric},
	{action: Action{Key: "needs_more_evidence", Label: "要求补证", Tone: "warning", RequiresComment: true, Metadata: map[string]any{"decision": "needs_more_evidence"}}, handler: handlerResolveDecisionGeneric},
}

// registeredActionsForDecision returns the declared actions for a decision kind:
// its explicit registry entry when present, else the generic set. This is the
// single source DecisionActions renders from, so every emitted button is
// registered (and therefore handler-covered) by construction.
func registeredActionsForDecision(decisionType string) []registeredDecisionAction {
	if entries, ok := decisionActionRegistry[decisionType]; ok {
		return entries
	}
	return genericDecisionActions
}

func decisionActionsFromRegistry(entries []registeredDecisionAction) []Action {
	actions := make([]Action, 0, len(entries))
	for _, entry := range entries {
		actions = append(actions, entry.action)
	}
	return actions
}
