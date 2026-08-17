package project

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// handoffDependentsMap 从依赖行构建 blocker→dependents 映射（只保留评估集内的
// 边，供 notes 软条目汇聚下游声明）。
func handoffDependentsMap(tasks []ProjectTask, dependencies []ProjectTaskDependency) map[uuid.UUID][]uuid.UUID {
	inSet := make(map[uuid.UUID]struct{}, len(tasks))
	for _, task := range tasks {
		inSet[task.ID] = struct{}{}
	}
	dependents := map[uuid.UUID][]uuid.UUID{}
	for _, dependency := range dependencies {
		if _, ok := inSet[dependency.BlockerTaskID]; !ok {
			continue
		}
		dependents[dependency.BlockerTaskID] = append(dependents[dependency.BlockerTaskID], dependency.DependentTaskID)
	}
	return dependents
}

// buildProjectTaskGraphHandoffAssessments 计算 task-graph 的结构化交接 verdict
// (spec 2026-07-27 §5 P2-V)。纯函数:只消费读路径已加载的任务(planner
// produces 声明)与最新任务结果契约(deliverables,v2 声明管道含 Ref 回填),
// 不发查询、不持久化。
//
// 规则(诚实边界,禁止启发式):
//   - 任务没有已记录的结果契约 → unknown(交接尚未产生交付数据,不预判)。
//   - 契约存在但声明集为空(契约无 deliverables 且 planner 无 produces) → unknown。
//   - 声明集 = 契约 deliverables ∪ planner produces(按名去重);逐条 verdict:
//     delivered = Ref 已回填或 Value 非空(与平台 produces 核对判据一致,
//     见 validateCompletedTaskResult);produces 声明但契约未交付 → missing。
//   - 汇总:全 delivered=fulfilled / 部分=partial / 全 missing=unfulfilled。
//   - notes 是**软条目**(spec 2026-08-16 交接包 §4.4):下游声明的结论槽位
//     逐条 delivered/missing,不进 Status 汇总、不打回——人看得见,机器不拦。
func buildProjectTaskGraphHandoffAssessments(tasks []ProjectTask, latestContracts map[uuid.UUID]*TaskResultContract, dependents map[uuid.UUID][]uuid.UUID) []ProjectTaskGraphHandoffAssessment {
	taskByID := make(map[uuid.UUID]ProjectTask, len(tasks))
	for _, task := range tasks {
		taskByID[task.ID] = task
	}
	assessments := make([]ProjectTaskGraphHandoffAssessment, 0, len(tasks))
	for _, task := range tasks {
		assessments = append(assessments, assessProjectTaskHandoff(task, latestContracts[task.ID], declaredDownstreamNotes(dependents[task.ID], taskByID)))
	}
	return assessments
}

// declaredDownstreamNotes 汇聚一个任务全部下游声明的结论槽位名（按名去重，
// 保序）。下游不在评估集内时其声明不可见——与 delivers 的诚实边界一致。
func declaredDownstreamNotes(dependentIDs []uuid.UUID, taskByID map[uuid.UUID]ProjectTask) []string {
	if len(dependentIDs) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var names []string
	for _, dependentID := range dependentIDs {
		dependent, ok := taskByID[dependentID]
		if !ok {
			continue
		}
		entries, ok := dependent.HandoffContract["notes"].([]any)
		if !ok {
			continue
		}
		for _, entry := range entries {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(fmt.Sprint(entryMap["name"]))
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	return names
}

func assessProjectTaskHandoff(task ProjectTask, contract *TaskResultContract, downstreamNotes []string) ProjectTaskGraphHandoffAssessment {
	assessment := ProjectTaskGraphHandoffAssessment{
		ProjectTaskID: task.ID,
		Status:        ProjectTaskGraphHandoffStatusUnknown,
		Deliverables:  []ProjectTaskGraphHandoffDeliverable{},
		Notes:         []ProjectTaskGraphHandoffNote{},
	}
	if contract == nil {
		return assessment
	}

	seenNames := map[string]struct{}{}
	deliveredCount := 0
	for _, deliverable := range contract.Deliverables {
		name := strings.TrimSpace(deliverable.Name)
		verdict := ProjectTaskGraphHandoffDeliverableMissing
		if strings.TrimSpace(deliverable.Ref) != "" || strings.TrimSpace(deliverable.Value) != "" {
			verdict = ProjectTaskGraphHandoffDeliverableDelivered
			deliveredCount++
		}
		if name != "" {
			seenNames[name] = struct{}{}
		}
		assessment.Deliverables = append(assessment.Deliverables, ProjectTaskGraphHandoffDeliverable{
			Name:    name,
			Kind:    strings.TrimSpace(deliverable.Kind),
			Verdict: verdict,
			Ref:     strings.TrimSpace(deliverable.Ref),
			Summary: strings.TrimSpace(deliverable.Summary),
		})
	}
	for _, name := range taskPlannerProduces(task) {
		if _, exists := seenNames[name]; exists {
			continue
		}
		seenNames[name] = struct{}{}
		assessment.Deliverables = append(assessment.Deliverables, ProjectTaskGraphHandoffDeliverable{
			Name:    name,
			Verdict: ProjectTaskGraphHandoffDeliverableMissing,
		})
	}

	// 结论槽位软条目：与 deliverables 的判据一致（值非空即 delivered），但
	// 不参与 fulfilled/partial/unfulfilled 汇总——散文不进闸。
	deliveredNotes := map[string]struct{}{}
	for _, note := range contract.HandoffNotes {
		name := strings.TrimSpace(note.Name)
		if name != "" && strings.TrimSpace(note.Value) != "" {
			deliveredNotes[name] = struct{}{}
		}
	}
	for _, name := range downstreamNotes {
		verdict := ProjectTaskGraphHandoffNoteMissing
		if _, delivered := deliveredNotes[name]; delivered {
			verdict = ProjectTaskGraphHandoffNoteDelivered
		}
		assessment.Notes = append(assessment.Notes, ProjectTaskGraphHandoffNote{Name: name, Verdict: verdict})
	}

	// 完成态申报的输入缺口：只投影，不进 Status 汇总、不触发补链（spec §4.4）。
	for _, gap := range contract.InputGaps {
		name := strings.TrimSpace(gap.Name)
		if name == "" {
			continue
		}
		assessment.InputGaps = append(assessment.InputGaps, ProjectTaskGraphHandoffInputGap{Name: name, Reason: strings.TrimSpace(gap.Reason)})
	}

	total := len(assessment.Deliverables)
	if total == 0 {
		return assessment
	}
	switch deliveredCount {
	case total:
		assessment.Status = ProjectTaskGraphHandoffStatusFulfilled
	case 0:
		assessment.Status = ProjectTaskGraphHandoffStatusUnfulfilled
	default:
		assessment.Status = ProjectTaskGraphHandoffStatusPartial
	}
	return assessment
}
