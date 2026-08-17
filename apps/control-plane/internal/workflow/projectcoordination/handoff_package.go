package projectcoordination

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/project"
)

// 阶段交接包编译（spec 2026-08-16-stage-handoff-package-design.md §3.2/§4.2）。
// 把派发期"上游结果整包倾倒 + 4KB summary 截断"替换为按下游声明槽位的确定性
// 编译：信封（模板锚定）+ 槽位（平台事实，带 source 溯源）+ 结论（上游按下游
// notes 声明写的结构化散文）+ 紧凑索引（未声明命中的 deliverables 只留指针）。
// 编译失败降级回 v1 collectUpstreamResults；缺口按"闸覆盖边/未闸覆盖边"分档
// （§3.4）：前者是异常态（unavailable 占位不阻断），后者是设计内路径（派发前
// 澄清卡，不静默派发）。

const (
	// handoffNoteValueLimitBytes 是单条结论值的截断上界（§3.2 拍板 7）。
	handoffNoteValueLimitBytes = 2048
	// handoffNotesPackageBudgetBytes 是一个交接包内结论值的总预算（§3.2 拍板 7）。
	handoffNotesPackageBudgetBytes = 8192
)

// handoffSlotDecl 是下游任务声明的一个平台槽位（v3 结构化形态；v2 字符串形态
// 在解析时归一）。Required 缺省 true。
type handoffSlotDecl struct {
	Name     string
	Kind     string
	Required bool
}

// handoffNoteDecl 是下游任务 handoff_contract.notes 声明的一个结论槽位。
type handoffNoteDecl struct {
	Name        string
	Description string
}

// handoffClarification 是一条需要人裁决的派发期缺口（未闸覆盖边缺失或同名冲突）。
type handoffClarification struct {
	Kind    string   // missing_input | name_conflict
	Name    string
	Sources []string // 冲突来源/应供给该槽位的上游任务标题
}

// compiledHandoffPackage 是 compileHandoffPackage 的产物。Package 写进
// execution_context_packet["handoff_package"]（版本走既有
// execution_context_packet_version 列升 "v2"，包内不造第二套版本键——拍板 6）。
type compiledHandoffPackage struct {
	Package        map[string]any
	Clarifications []handoffClarification
	// RetryWhenResultsLand 为 true 表示某直接 blocker 已是完成态但结果契约尚未
	// 落库（写回在途——真链 E2E 实证：完成放行可比结果落库早约 1 分钟）。此时
	// 编译出的包必然缺槽缺 notes，派发只会造成下游误报缺口；应暂缓派发，等
	// 迟到的结果写回触发完成信号后重派。
	RetryWhenResultsLand bool
}

// handoffBlockerMaterial 是槽位解析的纯输入：一个直接 blocker 的最新成功结果。
type handoffBlockerMaterial struct {
	TaskID      string
	TaskTitle   string
	EmployeeID  string
	ResultID    string
	AttemptID   string
	Deliverables []project.TaskResultDeliverable
	Notes       []project.TaskResultHandoffNote
	EvidenceRefs []project.TaskResultRef
	ArtifactRefs []project.TaskResultRef
}

// dynamicTaskMetadataKeys 出现任一即表示任务不是"已接受计划分解快照"的原生
// 任务，而是动态插入/重接家族（revision/对抗返工/补做/恢复替换——探索结论：
// 这些路径全部是 clone 源任务 PlannerMetadata 再追加自己的标记键）。
var dynamicTaskMetadataKeys = []string{"source_task_id", "revision_root_task_id", "supplement_for", "recovery_action"}

// taskBelongsToAcceptedPlanDecomposition 是缺口分档判据（§4.2）：任务来自已接受
// 计划分解（其 required_inputs 过了图校验、上游 produces 过了回写闸，两闸链式
// 覆盖静态图供给），派发期缺槽位属异常态；动态插入/重接的任务其供给关系未经
// ValidateRouteDecisionGraph，缺槽位是设计内路径，走澄清卡。
func taskBelongsToAcceptedPlanDecomposition(task project.ProjectTask) bool {
	if task.AcceptedPlanRevisionID == nil || task.RevisionOfTaskID != nil || task.PlanIteration > 0 {
		return false
	}
	for _, key := range dynamicTaskMetadataKeys {
		if _, ok := task.PlannerMetadata[key]; ok {
			return false
		}
	}
	return true
}

// handoffSlotDeclarations 解析任务声明的平台槽位，兼容 v2 字符串形态与
// v3 对象形态（JSON round-trip 后是 []any 混排）。
func handoffSlotDeclarations(task project.ProjectTask) []handoffSlotDecl {
	raw, ok := task.InputRequirements["required_inputs"]
	if !ok || raw == nil {
		return nil
	}
	var decls []handoffSlotDecl
	appendDecl := func(name, kind string, required *bool) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		decls = append(decls, handoffSlotDecl{Name: name, Kind: strings.TrimSpace(kind), Required: required == nil || *required})
	}
	switch typed := raw.(type) {
	case []any:
		for _, entry := range typed {
			switch entryTyped := entry.(type) {
			case string:
				appendDecl(entryTyped, "", nil)
			case map[string]any:
				appendDecl(stringFromAny(entryTyped["name"]), stringFromAny(entryTyped["kind"]), boolPtrFromAny(entryTyped["required"]))
			}
		}
	case []map[string]any:
		for _, entry := range typed {
			appendDecl(stringFromAny(entry["name"]), stringFromAny(entry["kind"]), boolPtrFromAny(entry["required"]))
		}
	case []string:
		for _, entry := range typed {
			appendDecl(entry, "", nil)
		}
	}
	return decls
}

// handoffNoteDeclarations 解析任务 handoff_contract.notes 声明的结论槽位。
func handoffNoteDeclarations(contract map[string]any) []handoffNoteDecl {
	entries, ok := contract["notes"].([]any)
	if !ok {
		return nil
	}
	decls := make([]handoffNoteDecl, 0, len(entries))
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringFromAny(entryMap["name"]))
		if name == "" {
			continue
		}
		decls = append(decls, handoffNoteDecl{Name: name, Description: strings.TrimSpace(stringFromAny(entryMap["description"]))})
	}
	return decls
}

func boolPtrFromAny(value any) *bool {
	if typed, ok := value.(bool); ok {
		return &typed
	}
	return nil
}

// compileHandoffPackage 在派发期编译交接包。返回的 Clarifications 非空时调用方
// 必须停住派发转澄清卡（§3.4）。错误时调用方降级 v1，绝不因编译器缺陷阻断调度。
func (s *ProjectStore) compileHandoffPackage(ctx context.Context, tenantID, projectID uuid.UUID, task project.ProjectTask) (*compiledHandoffPackage, error) {
	deps, err := s.repository.ListProjectTaskDependencies(ctx, tenantID, projectID, []uuid.UUID{task.ID})
	if err != nil {
		return nil, err
	}
	if len(deps) == 0 {
		return &compiledHandoffPackage{Package: emptyHandoffPackage(task)}, nil
	}
	blockers := make([]handoffBlockerMaterial, 0, len(deps))
	resultPending := false
	for _, dep := range deps {
		blocker, err := s.repository.GetProjectTask(ctx, tenantID, dep.BlockerTaskID)
		if err != nil {
			return nil, err
		}
		material := handoffBlockerMaterial{
			TaskID:    blocker.ID.String(),
			TaskTitle: blocker.Title,
		}
		if blocker.AssignedDigitalEmployeeID != nil {
			material.EmployeeID = blocker.AssignedDigitalEmployeeID.String()
		}
		result, resultErr := s.latestTaskResult(ctx, blocker)
		if resultErr != nil {
			return nil, resultErr
		}
		if result == nil && projectTaskTerminalStatus(blocker.Status) {
			// 完成态但无结果：写回在途，等结果落地后由完成信号重派。
			resultPending = true
		}
		if result != nil {
			material.ResultID = result.ID.String()
			if result.AttemptID != nil {
				material.AttemptID = result.AttemptID.String()
			}
			material.Deliverables = result.Contract.Deliverables
			material.Notes = result.Contract.HandoffNotes
			material.EvidenceRefs = result.Contract.EvidenceRefs
			material.ArtifactRefs = result.Contract.ArtifactRefs
		}
		blockers = append(blockers, material)
	}
	compiled := resolveHandoffSlots(handoffSlotDeclarations(task), handoffNoteDeclarations(task.HandoffContract), blockers, taskBelongsToAcceptedPlanDecomposition(task), handoffEnvelope(task))
	compiled.RetryWhenResultsLand = resultPending
	return compiled, nil
}

func emptyHandoffPackage(task project.ProjectTask) map[string]any {
	return map[string]any{
		"envelope":        handoffEnvelope(task),
		"resolved_inputs": []map[string]any{},
		"upstream_notes":  []map[string]any{},
		"upstream_index":  []map[string]any{},
	}
}

// handoffEnvelope 只锚定模板（任务/attempt/demand 锚定复用 packet 既有扁平
// 字段——§3.2 拍板 6）。template_key/version 取 DecomposeAcceptedPlanRevision
// 写进 PlannerMetadata 的实例化时锁定快照。
func handoffEnvelope(task project.ProjectTask) map[string]any {
	envelope := map[string]any{}
	if key := strings.TrimSpace(stringFromAny(task.PlannerMetadata["template_key"])); key != "" {
		envelope["template_key"] = key
		if version := stringFromAny(task.PlannerMetadata["template_version"]); strings.TrimSpace(version) != "" {
			envelope["template_version"] = version
		}
	}
	return envelope
}

// resolveHandoffSlots 是槽位解析的纯函数核心（可单测）：
//  1. 按声明 name 精确匹配 blockers 的 deliverables；未命中且声明了 kind 时按
//     kind 回落（唯一命中才采用）。
//  2. 多来源命中 = 同名冲突 → 澄清卡（附全部来源），不按顺序取首（§4.2）。
//  3. required 缺失按边分档：闸覆盖边 = unavailable 占位（异常态留痕不阻断）；
//     未闸覆盖边 = 澄清卡。
//  4. notes 按下游声明 name 提取合并，应用 2KB/条、8KB/包上界。
//  5. 未被声明命中的 deliverables 进紧凑索引（name+kind+ref，不带值全文）。
func resolveHandoffSlots(decls []handoffSlotDecl, noteDecls []handoffNoteDecl, blockers []handoffBlockerMaterial, gateCovered bool, envelope map[string]any) *compiledHandoffPackage {
	resolvedInputs := make([]map[string]any, 0, len(decls))
	consumed := map[string]bool{} // "<taskID>\x00<name>" 已被声明槽位命中的 deliverable
	var clarifications []handoffClarification

	type slotHit struct {
		material handoffBlockerMaterial
		entry    project.TaskResultDeliverable
	}
	for _, decl := range decls {
		var hits []slotHit
		for _, material := range blockers {
			for _, deliverable := range material.Deliverables {
				if strings.TrimSpace(deliverable.Name) == decl.Name {
					hits = append(hits, slotHit{material: material, entry: deliverable})
				}
			}
		}
		if len(hits) == 0 && decl.Kind != "" {
			for _, material := range blockers {
				for _, deliverable := range material.Deliverables {
					if strings.TrimSpace(deliverable.Kind) == decl.Kind {
						hits = append(hits, slotHit{material: material, entry: deliverable})
					}
				}
			}
		}
		switch {
		case len(hits) == 1:
			hit := hits[0]
			value := strings.TrimSpace(hit.entry.Value)
			ref := strings.TrimSpace(hit.entry.Ref)
			entry := map[string]any{
				"name":             decl.Name,
				"kind":             nonEmptyDefault(strings.TrimSpace(hit.entry.Kind), decl.Kind),
				"source_task_id":   hit.material.TaskID,
				"source_result_id": hit.material.ResultID,
				// 形态核验：值或 ref 非空即 delivered（判据与 handoff assessment
				// 一致）。真值核验（commit 是否真为 HEAD 等）为 spec §8 遗留。
				"verified": value != "" || ref != "",
			}
			if value != "" {
				entry["value"] = value
			}
			if ref != "" {
				entry["ref"] = ref
				if _, err := uuid.Parse(ref); err == nil {
					// 回写侧 resolveDeclaredDeliverableRefs 已把文件形态交付物的
					// ref 改写为 artifact_ref_id（血缘），UUID 形态即已物化工件。
					entry["artifact_ref_id"] = ref
				}
			}
			resolvedInputs = append(resolvedInputs, entry)
			consumed[hit.material.TaskID+"\x00"+strings.TrimSpace(hit.entry.Name)] = true
		case len(hits) > 1:
			sources := make([]string, 0, len(hits))
			for _, hit := range hits {
				sources = append(sources, hit.material.TaskTitle)
			}
			clarifications = append(clarifications, handoffClarification{Kind: "name_conflict", Name: decl.Name, Sources: sources})
		default:
			if !decl.Required {
				continue
			}
			if gateCovered {
				// 两闸（图校验 + 回写履约）链式覆盖下的缺失是异常态：占位留痕，
				// 不阻断派发（对齐 v1 collectUpstreamResults 的降级哲学）。
				entry := map[string]any{"name": decl.Name, "missing": "unavailable"}
				if decl.Kind != "" {
					entry["kind"] = decl.Kind
				}
				resolvedInputs = append(resolvedInputs, entry)
				slog.Warn("handoff slot unresolved on gate-covered edge", "slot", decl.Name, "kind", decl.Kind, "task_id", firstBlockerTaskID(blockers))
			} else {
				clarifications = append(clarifications, handoffClarification{Kind: "missing_input", Name: decl.Name, Sources: blockerTitles(blockers)})
			}
		}
	}

	upstreamNotes, notesTruncated := collectUpstreamNotes(noteDecls, blockers)
	upstreamIndex := buildUpstreamIndex(blockers, consumed)

	pkg := map[string]any{
		"envelope":        envelope,
		"resolved_inputs": resolvedInputs,
		"upstream_notes":  upstreamNotes,
		"upstream_index":  upstreamIndex,
	}
	if notesTruncated {
		pkg["notes_truncated"] = true
	}
	return &compiledHandoffPackage{Package: pkg, Clarifications: clarifications}
}

func firstBlockerTaskID(blockers []handoffBlockerMaterial) string {
	if len(blockers) == 0 {
		return ""
	}
	return blockers[0].TaskID
}

func blockerTitles(blockers []handoffBlockerMaterial) []string {
	titles := make([]string, 0, len(blockers))
	for _, material := range blockers {
		titles = append(titles, material.TaskTitle)
	}
	return titles
}

func nonEmptyDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func collectUpstreamNotes(noteDecls []handoffNoteDecl, blockers []handoffBlockerMaterial) ([]map[string]any, bool) {
	if len(noteDecls) == 0 {
		return []map[string]any{}, false
	}
	notes := make([]map[string]any, 0, len(noteDecls))
	budget := handoffNotesPackageBudgetBytes
	truncated := false
	for _, decl := range noteDecls {
		for _, material := range blockers {
			for _, note := range material.Notes {
				if strings.TrimSpace(note.Name) != decl.Name {
					continue
				}
				value := note.Value
				if len(value) > handoffNoteValueLimitBytes {
					value = truncateUTF8(value, handoffNoteValueLimitBytes) + "…[truncated]"
					truncated = true
				}
				if budget-len(value) < 0 {
					truncated = true
					break
				}
				budget -= len(value)
				entry := map[string]any{"name": decl.Name, "source_task_id": material.TaskID, "value": value}
				if len(value) != len(note.Value) {
					entry["truncated"] = true
				}
				notes = append(notes, entry)
			}
		}
	}
	return notes, truncated
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}

func buildUpstreamIndex(blockers []handoffBlockerMaterial, consumed map[string]bool) []map[string]any {
	index := make([]map[string]any, 0, len(blockers))
	for _, material := range blockers {
		entry := map[string]any{
			"task_id":    material.TaskID,
			"task_title": material.TaskTitle,
		}
		if material.EmployeeID != "" {
			entry["employee_id"] = material.EmployeeID
		}
		var others []map[string]any
		for _, deliverable := range material.Deliverables {
			name := strings.TrimSpace(deliverable.Name)
			if name != "" && consumed[material.TaskID+"\x00"+name] {
				continue
			}
			item := map[string]any{"name": name}
			if kind := strings.TrimSpace(deliverable.Kind); kind != "" {
				item["kind"] = kind
			}
			if ref := strings.TrimSpace(deliverable.Ref); ref != "" {
				item["ref"] = ref
			}
			others = append(others, item)
		}
		if others != nil {
			entry["other_deliverables"] = others
		}
		if refs := compactTaskResultRefs(material.EvidenceRefs); refs != nil {
			entry["evidence_refs"] = refs
		}
		if refs := compactTaskResultRefs(material.ArtifactRefs); refs != nil {
			entry["artifact_refs"] = refs
		}
		if material.ResultID != "" && material.AttemptID != "" {
			entry["log_ref"] = "runs/" + material.AttemptID + "/manifest.json"
		}
		index = append(index, entry)
	}
	return index
}

// compactTaskResultRefs 把结构化引用压成最短可读字符串（按引用透传、按需
// 解引用——07-13 §3.2；完整结构在 result 契约与 raw log 里）。
func compactTaskResultRefs(refs []project.TaskResultRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		for _, candidate := range []string{ref.Ref, ref.URI, ref.URL, ref.ID} {
			if trimmed := strings.TrimSpace(candidate); trimmed != "" {
				out = append(out, trimmed)
				break
			}
		}
	}
	return out
}

// downstreamNoteSpec 是"结论槽位的需求回传"（§4.3）：本任务某个下游声明的
// notes 字段。全图在计划确认时一次建好，上游派发时即可回传需求——声明有
// 消费方，字段才不会退化成空壳。
type downstreamNoteSpec struct {
	DependentTitle string
	Notes          []handoffNoteDecl
}

func (s *ProjectStore) handoffNotesExpectations(ctx context.Context, tenantID, projectID uuid.UUID, task project.ProjectTask) []downstreamNoteSpec {
	dependentIDs, err := s.repository.ListDependentsOfTask(ctx, tenantID, projectID, task.ID)
	if err != nil || len(dependentIDs) == 0 {
		return nil
	}
	specs := make([]downstreamNoteSpec, 0, len(dependentIDs))
	for _, dependentID := range dependentIDs {
		dependent, err := s.repository.GetProjectTask(ctx, tenantID, dependentID)
		if err != nil {
			continue
		}
		decls := handoffNoteDeclarations(dependent.HandoffContract)
		if len(decls) == 0 {
			continue
		}
		specs = append(specs, downstreamNoteSpec{DependentTitle: dependent.Title, Notes: decls})
	}
	return specs
}

// handoffSupplementedEdges 判定申报方的补链预算是否已支出（spec 2026-08-16
// 交接包 §4.4：同一申报方只自动补一轮——"同边一次"，边的身份随 rewire 演进，
// 申报方是不变量）。存在 supplement_for==申报方 的补做任务即已支出；返回的
// 集合含该补做及其整条 revision 根链上的 owner，供按 owner 查询的调用点使用。
func handoffSupplementedEdges(siblings []project.ProjectTask, sourceTaskID uuid.UUID) map[uuid.UUID]bool {
	supplemented := map[uuid.UUID]bool{}
	for _, task := range siblings {
		if task.PlanIteration == 0 {
			continue
		}
		supplementFor, _ := task.PlannerMetadata["supplement_for"].(string)
		if supplementFor != sourceTaskID.String() {
			continue
		}
		supplemented[task.ID] = true
		if task.RevisionOfTaskID != nil {
			supplemented[*task.RevisionOfTaskID] = true
		}
	}
	return supplemented
}

// noteDeclaredByTask 报告 name 是否为该任务 handoff_contract.notes 声明的
// 结论槽位（blocked 申报白名单与补做 owner 解析共用）。
func noteDeclaredByTask(task project.ProjectTask, name string) bool {
	for _, decl := range handoffNoteDeclarations(task.HandoffContract) {
		if decl.Name == name {
			return true
		}
	}
	return false
}

// handoffPackageFromPacket 从已持久化的 execution_context_packet 取出 v2 交接包
// （恢复路径复用派发期编译的权威快照）。
func handoffPackageFromPacket(packet map[string]any) map[string]any {
	if packet == nil {
		return nil
	}
	if pkg, ok := packet["handoff_package"].(map[string]any); ok {
		return pkg
	}
	return nil
}

// holdForHandoffClarification 把派发停在澄清卡（§3.4 未闸覆盖边）：复用
// predispatch gate 的 waiting_human 家族（approval + decision request +
// MoveProjectTaskToWaitingHumanForPreDispatchGate），人批准后以
// human_resolved 理由重派并重新编译。
func (s *ProjectStore) holdForHandoffClarification(ctx context.Context, input DispatchProjectTaskInput, task project.ProjectTask, clarifications []handoffClarification) error {
	if s.approvals == nil {
		return errPreDispatchGateApprovalCreatorRequired
	}
	gateInput := project.PreDispatchGateInput{
		ProjectID:              input.ProjectID,
		ProjectTaskID:          input.TaskID,
		AcceptedPlanRevisionID: task.AcceptedPlanRevisionID,
		PlannedTaskKey:         task.PlannedTaskKey,
		SelectedEmployeeID:     selectedEmployeeID(task),
		AttemptNo:              task.AttemptCount + 1,
		DispatchReason:         defaultDispatchReason(input.DispatchReason),
	}
	gapLines := make([]string, 0, len(clarifications))
	for _, gap := range clarifications {
		if gap.Kind == "name_conflict" {
			gapLines = append(gapLines, fmt.Sprintf("槽位 %s 有多个上游来源：%s", gap.Name, strings.Join(gap.Sources, "、")))
			continue
		}
		gapLines = append(gapLines, fmt.Sprintf("槽位 %s 未由任何直接上游交付（应供给方：%s）", gap.Name, strings.Join(gap.Sources, "、")))
	}
	summary := fmt.Sprintf("交接包槽位解析发现 %d 处缺口，派发前需人工确认：\n%s", len(clarifications), strings.Join(gapLines, "\n"))
	evaluation := project.PreDispatchGateEvaluation{
		Status:        project.PreDispatchGateStatusWaitingHuman,
		CheckedAt:     s.now(),
		IdempotencyKey: project.PreDispatchGateIdempotencyKey(gateInput),
		DispatchToken: project.PreDispatchGateDispatchToken(gateInput),
		Checks: []project.PreDispatchGateCheck{{
			Key:     "handoff.slot_resolution",
			Status:  "failed",
			Details: map[string]any{"gaps": len(clarifications)},
		}},
		Blockers: []project.PreDispatchGateBlocker{{
			Key:      "handoff.slot_missing",
			Severity: "human",
			Details:  map[string]any{"clarifications": clarifications},
		}},
		HumanActionRequest: &project.PreDispatchHumanActionRequest{
			Type:          "clarification",
			WaitingReason: project.HumanWaitReasonClarification,
			DecisionType:  "project_task_clarification",
			Title:         "交接槽位缺口需确认：" + task.Title,
			Summary:       summary,
			RiskLevel:     "medium",
			Options:       []any{"approved", "rejected", "needs_more_evidence", "cancelled"},
			Context: map[string]any{
				"source":         "predispatch_gate",
				"check":          "handoff.slot_resolution",
				"clarifications": clarifications,
			},
		},
	}
	gate, err := s.recordEvaluatedGate(ctx, input, gateInput, evaluation)
	if err != nil {
		return err
	}
	if _, err := s.createGateHumanAction(ctx, input, task, gate, evaluation.HumanActionRequest); err != nil {
		return err
	}
	slog.WarnContext(ctx, "handoff package held for clarification", "project_task_id", task.ID, "gaps", len(clarifications))
	return nil
}
