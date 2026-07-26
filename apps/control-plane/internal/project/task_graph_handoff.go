package project

import (
	"strings"

	"github.com/google/uuid"
)

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
func buildProjectTaskGraphHandoffAssessments(tasks []ProjectTask, latestContracts map[uuid.UUID]*TaskResultContract) []ProjectTaskGraphHandoffAssessment {
	assessments := make([]ProjectTaskGraphHandoffAssessment, 0, len(tasks))
	for _, task := range tasks {
		assessments = append(assessments, assessProjectTaskHandoff(task, latestContracts[task.ID]))
	}
	return assessments
}

func assessProjectTaskHandoff(task ProjectTask, contract *TaskResultContract) ProjectTaskGraphHandoffAssessment {
	assessment := ProjectTaskGraphHandoffAssessment{
		ProjectTaskID: task.ID,
		Status:        ProjectTaskGraphHandoffStatusUnknown,
		Deliverables:  []ProjectTaskGraphHandoffDeliverable{},
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
