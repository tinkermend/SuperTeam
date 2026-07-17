package projectcoordination

// 本文件实现审阅门的预置条件注册表 + coordination_policy 配置读取层（见
// docs/superpowers/specs/2026-07-17-review-gate-violation-detection-design.md §1/§10，Task 3）。
//
// 注册表（standardConditions）是"平台内置哪些条件、每个条件的默认动作是什么"的唯一事实源；
// 配置层（enabledConditions）是"某个项目实际启用了哪些条件、动作是否被覆盖"的解析逻辑，读
// projects.coordination_policy.review_gate_conditions（仿 adversarialJudgeCount /
// maxPlanIterations 的 policy 读法，见 adversarial_trigger.go:365-372、project_store.go:4355-4361）。

// ConditionSpec 描述注册表里的一个预置条件：它的检测器实现、条件类型（rule|llm）和默认动作
// （block|need_human|record_only）。DefaultAction 在项目未在 coordination_policy 里显式覆盖
// 该条件时生效。
type ConditionSpec struct {
	Key           string
	Kind          string // "rule" | "llm"
	DefaultAction string // "block" | "need_human" | "record_only"
	Detector      ConditionDetector
}

// reviewGateActionBlock / NeedHuman / RecordOnly 是审阅门条件动作的合法取值。
const (
	reviewGateActionBlock      = "block"
	reviewGateActionNeedHuman  = "need_human"
	reviewGateActionRecordOnly = "record_only"
	reviewGateActionOff        = "off"
)

// isValidReviewGateAction 判断 s 是否是一个合法的（非 off）审阅门动作取值。
func isValidReviewGateAction(s string) bool {
	switch s {
	case reviewGateActionBlock, reviewGateActionNeedHuman, reviewGateActionRecordOnly:
		return true
	default:
		return false
	}
}

// standardConditions 返回平台预置的条件库。P1 只有两个条件：
//   - secret_leak（规则型，默认 block）：密钥泄漏是安全类问题，默认动作最严格。
//   - code_review（LLM 型，默认 need_human）：代码审查违反检测存在误判可能，默认转人工而不
//     是自动拦截。
//
// client/model 透传给需要调用 LLM 的检测器（目前只有 code_review）。
func standardConditions(client chatCompletionClient, model string) []ConditionSpec {
	return []ConditionSpec{
		{
			Key:           "secret_leak",
			Kind:          "rule",
			DefaultAction: reviewGateActionBlock,
			Detector:      newSecretLeakDetector(),
		},
		{
			Key:           "code_review",
			Kind:          "llm",
			DefaultAction: reviewGateActionNeedHuman,
			Detector:      newCodeReviewDetector(client, model),
		},
	}
}

// EnabledCondition 是一个条件在某个项目的 coordination_policy 下解析出的最终状态：条件定义
// 本身 + 已解析（覆盖后）的动作。Task 4 的调用方只需要遍历这个结果，不需要再看 policy 原始
// 形状。
type EnabledCondition struct {
	Spec   ConditionSpec
	Action string
}

// enabledConditions 读取 policy["review_gate_conditions"]（形如
// {"secret_leak":"block","code_review":"off"}）解析出 all 中每个条件的启用状态与最终动作。
//
// 解析规则（按条件逐个决定，条件之间互不影响）：
//   - policy 里完全没有 review_gate_conditions 这个键（nil/absent/非 map）：审阅门整体走"默认
//     开"——all 中的每个条件都启用，动作取 DefaultAction。这是刻意选择：审阅门是安全/质量
//     兜底能力，缺省应该是打开的，不能因为项目没配置就整体失效。
//   - review_gate_conditions 存在，但某个条件的 key 不在这个 map 里：同样按"默认开"处理——
//     启用，动作取 DefaultAction。理由：这样平台新增预置条件时，已有项目会自动获得新条件的
//     默认保护，不需要每个项目逐一显式加开关；项目如果不想要某个新条件，可以显式写
//     "<key>": "off" 关掉。这与"整体未配置=默认开"是同一条原则在字段粒度上的延伸。
//   - 某个条件的 policy 值是 "off"：禁用，从返回结果里排除。
//   - 某个条件的 policy 值是合法动作（block/need_human/record_only）：启用，动作覆盖为该值。
//   - 某个条件的 policy 值是非法字符串（既不是 off 也不是合法动作，例如拼写错误）：不当成
//     禁用处理（避免一个拼写错误静默关掉安全检测），退回 DefaultAction 生效——见
//     isValidReviewGateAction。
//
// 返回顺序与 all 的顺序一致，调用方如需稳定顺序无需自行排序。
func enabledConditions(policy map[string]any, all []ConditionSpec) []EnabledCondition {
	overrides, hasOverrides := reviewGateConditionOverrides(policy)

	result := make([]EnabledCondition, 0, len(all))
	for _, spec := range all {
		action := spec.DefaultAction
		if hasOverrides {
			if raw, ok := overrides[spec.Key]; ok {
				switch {
				case raw == reviewGateActionOff:
					continue // explicitly disabled
				case isValidReviewGateAction(raw):
					action = raw
				default:
					// Unknown action string: fall back to DefaultAction rather than
					// crashing or silently disabling on a typo.
					action = spec.DefaultAction
				}
			}
			// key absent from overrides map: stays enabled at DefaultAction (see doc
			// comment above).
		}
		result = append(result, EnabledCondition{Spec: spec, Action: action})
	}
	return result
}

// reviewGateConditionOverrides 从 policy 里防御性地取出 review_gate_conditions 这个嵌套
// map，把值统一断言为 string。第二个返回值表示这个键在 policy 里是否存在且是一个 map 形状
// （区分"完全没配置"与"配置了但是个空 map"——空 map 也应该走"逐条目默认开"分支，因为它不代表
// 任何显式的 off）。
func reviewGateConditionOverrides(policy map[string]any) (map[string]string, bool) {
	if policy == nil {
		return nil, false
	}
	raw, ok := policy["review_gate_conditions"]
	if !ok || raw == nil {
		return nil, false
	}
	rawMap, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	overrides := make(map[string]string, len(rawMap))
	for k, v := range rawMap {
		if s, ok := v.(string); ok {
			overrides[k] = s
		}
	}
	return overrides, true
}

// reviewGateMinorTolerance reads coordination_policy.review_gate_minor_tolerance as a
// three-state int (int/float64/json.Number via int32FromAny, same reader used by
// adversarialJudgeCount/maxPlanIterations). Returns 0 (no tolerance — any minor hit
// counts) when unset, non-numeric, or negative.
func reviewGateMinorTolerance(policy map[string]any) int {
	if value, ok := int32FromAny(policy["review_gate_minor_tolerance"]); ok && value > 0 {
		return int(value)
	}
	return 0
}
