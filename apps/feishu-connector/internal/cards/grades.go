package cards

import "strings"

// InteractionMode is the Feishu channel grade for a HumanTask kind
// (2026-07-25-human-task-load-budget-and-channel-grading.md §5.3).
type InteractionMode string

const (
	// ModeCardActions: card-local approve/reject (or equivalent) buttons.
	ModeCardActions InteractionMode = "card_actions"
	// ModePerCriterionSign: acceptance_sign — per-criterion buttons, no bulk approve.
	ModePerCriterionSign InteractionMode = "per_criterion_sign"
	// ModeDeepLink: console-only CTA (evidence incomplete on IM).
	ModeDeepLink InteractionMode = "deep_link"
)

// cardAction describes one Feishu button keyed to ResolveDecision's decision value.
type cardAction struct {
	Label           string
	ButtonType      string // primary / danger / default
	Decision        string
	RequiresComment bool
}

// interactionGrade is the explicit kind → Feishu interaction table (§5.3).
type interactionGrade struct {
	Mode    InteractionMode
	Actions []cardAction
}

// kindInteractionGrades is the single Feishu grading table. Card rendering looks
// up by HumanTask kind from the outbox payload — it must not re-derive kind from
// decision_type for new writes (CP stamps kind via humantask.KindAndLayer).
var kindInteractionGrades = map[string]interactionGrade{
	"plan_review": {
		Mode: ModeCardActions,
		Actions: []cardAction{
			{Label: "批准", ButtonType: "primary", Decision: "approved"},
			{Label: "请求修改", ButtonType: "danger", Decision: "request_changes", RequiresComment: true},
		},
	},
	"planning_gap": {
		Mode: ModeCardActions,
		Actions: []cardAction{
			{Label: "已补员,重新规划", ButtonType: "primary", Decision: "restaffed"},
			{Label: "豁免约束", ButtonType: "default", Decision: "exempted"},
			{Label: "关闭需求", ButtonType: "danger", Decision: "rejected"},
		},
	},
	"acceptance_sign": {
		Mode: ModePerCriterionSign,
		// Per-criterion buttons come from acceptanceSignElements; no bulk approve.
	},
	"dispatch_release": {
		Mode: ModeCardActions,
		Actions: []cardAction{
			{Label: "放行", ButtonType: "primary", Decision: "approved"},
			{Label: "驳回", ButtonType: "danger", Decision: "rejected", RequiresComment: true},
		},
	},
	"downstream_release": {
		Mode: ModeCardActions,
		Actions: []cardAction{
			{Label: "放行", ButtonType: "primary", Decision: "approved"},
			{Label: "驳回", ButtonType: "danger", Decision: "rejected", RequiresComment: true},
		},
	},
	"closure_confirm": {
		Mode: ModeCardActions,
		Actions: []cardAction{
			{Label: "确认结项并归档", ButtonType: "primary", Decision: "approved"},
			{Label: "退回返工", ButtonType: "danger", Decision: "rejected", RequiresComment: true},
			{Label: "要求补证", ButtonType: "default", Decision: "needs_more_evidence", RequiresComment: true},
		},
	},
	"planning_failed": {
		Mode: ModeCardActions,
		Actions: []cardAction{
			{Label: "重新规划", ButtonType: "primary", Decision: "retry_planning"},
			{Label: "关闭需求", ButtonType: "danger", Decision: "close_demand", RequiresComment: true},
		},
	},
	"task_failure_recovery": {
		Mode: ModeCardActions,
		Actions: []cardAction{
			{Label: "重试任务", ButtonType: "primary", Decision: "retry"},
			{Label: "取消下游", ButtonType: "danger", Decision: "cancel_downstream", RequiresComment: true},
		},
	},
}

// resolvePayloadKind returns the HumanTask kind for rendering.
// Prefer payload["kind"] (stamped by CP). Legacy outbox rows without kind fall
// back to a minimal alias of the historical decision_type axis so already-sent
// cards keep working — not a second KindAndLayer registry for new writes.
func resolvePayloadKind(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if kind, _ := payload["kind"].(string); strings.TrimSpace(kind) != "" {
		return strings.TrimSpace(kind)
	}
	decisionType, _ := payload["decision_type"].(string)
	switch strings.TrimSpace(decisionType) {
	case "demand_acceptance":
		return "acceptance_sign"
	case "project_acceptance":
		return "closure_confirm"
	case "project_task_approval":
		return "dispatch_release"
	case "project_task_acceptance":
		return "downstream_release"
	default:
		return strings.TrimSpace(decisionType)
	}
}

func gradeForKind(kind string) (interactionGrade, bool) {
	grade, ok := kindInteractionGrades[kind]
	return grade, ok
}
