package cards

// humanTaskKindLabels is the Feishu-side HumanTask kind vocabulary.
// Must stay byte-identical to contracts/control-plane/human-task-kind-labels.json
// (guarded by labels_test.go). Console status-labels.ts is guarded the same way.
var humanTaskKindLabels = map[string]string{
	"plan_review":           "计划确认",
	"dispatch_release":      "执行放行",
	"downstream_release":    "下游放行",
	"acceptance_sign":       "验收签署",
	"closure_confirm":       "结项确认",
	"planning_failed":       "规划失败",
	"planning_gap":          "规划缺口",
	"task_failure_recovery": "任务失败恢复",
}

func humanTaskKindLabel(kind string) string {
	if label, ok := humanTaskKindLabels[kind]; ok {
		return label
	}
	if kind == "" {
		return "未知"
	}
	return kind
}
