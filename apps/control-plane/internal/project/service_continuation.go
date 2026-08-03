package project

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// 同单接续（spec 2026-08-01-demand-continuation-design）。
//
// 接续**不改原单**：demand 状态单调不可回退，验收闸在原单完成时已消费一次,
// 想在原单上追加就得同时放开单调性、让收敛闸二次触发、把终态通知改成按代次
// 幂等 —— 三处承重改动换一个用新 demand 就能表达的语义。所以接续 = 新开一单
// 并接上血缘链,「一单」的用户身份因此是链而不是行。
//
// 本文件只管**接续这件事**：校验 + 继承 + 建链。建完之后一律走 SubmitDemand
// 的既有通路(项目门禁 → 事件 → 协调线程 signal),不新开 signal 类型、不给
// 协调侧加分支——协调线程不需要知道这一单是不是接续来的。

const (
	// DefaultDemandContinuationMaxDepth 是接续链的深度上限。链头为 0,即默认
	// 允许接续 10 次。上限存在的意义不是限制用户,是让递归遍历在数据被手工
	// 改坏(成环)时一定能停(spec §5.2 D3)。
	DefaultDemandContinuationMaxDepth int32 = 10
)

var (
	// ErrDemandNotSettled 表示原单还没到终态。还在跑的单应该等它跑完或就地
	// 纠偏,不是另开一单接在后面——否则两条链会并行改同一批产物。
	ErrDemandNotSettled = errors.New("demand not settled")
	// ErrContinuationChainTooDeep 表示链深超过上限。
	ErrContinuationChainTooDeep = errors.New("continuation chain too deep")
)

type ContinueDemandRequest struct {
	TenantID    uuid.UUID
	DemandID    uuid.UUID
	ActorUserID uuid.UUID
	// Title 可留空，服务端按父单标题派生；面向用户的标题不得为空。
	Title string
	// Content 必填：人得说清楚"接着做什么"。接续继承的是剧本与血缘,
	// 不是诉求——诉求每次都得重新说。
	Content     string
	Attachments []any
}

// ContinueProjectDemand 在一条已结束的需求之后新开一单并接上血缘链。
func (s *Service) ContinueProjectDemand(ctx context.Context, req ContinueDemandRequest) (*ProjectDemand, error) {
	req.Title = strings.TrimSpace(req.Title)
	req.Content = strings.TrimSpace(req.Content)
	if req.TenantID == uuid.Nil || req.DemandID == uuid.Nil || req.ActorUserID == uuid.Nil || req.Content == "" {
		return nil, ErrInvalidProject
	}

	parent, err := s.repository.GetProjectDemand(ctx, req.TenantID, req.DemandID)
	if err != nil {
		return nil, err
	}
	// 允许 failed / cancelled 接续:"跑挂了接着修"正是最高频的接续场景,
	// 拦它等于把最痛的那条路堵死。只拦"还没结束"。
	if !isTerminalProjectDemandStatus(parent.Status) {
		return nil, fmt.Errorf("原单尚未结束,无法接续: %w", ErrDemandNotSettled)
	}

	depth, err := s.repository.CountProjectDemandContinuationDepth(ctx, req.TenantID, parent.ID, DefaultDemandContinuationMaxDepth)
	if err != nil {
		return nil, err
	}
	if depth+1 > DefaultDemandContinuationMaxDepth {
		return nil, fmt.Errorf("接续链已达上限 %d: %w", DefaultDemandContinuationMaxDepth, ErrContinuationChainTooDeep)
	}

	title := req.Title
	if title == "" {
		title = continuationTitle(parent.Title, depth+1)
	}

	// 继承规则(spec §4.2):项目、剧本、协调模式、优先级/风险等级跟着走;
	// 来源渠道**不继承**——接续单的渠道是本次发起的渠道(可能是 Web、飞书或
	// automation),与父单从哪来无关。source_refs 里放一份冗余指针便于排查,
	// 但权威始终是 continues_demand_id 这一列。
	continuesID := parent.ID
	submit := SubmitProjectDemandRequest{
		TenantID:          req.TenantID,
		ProjectID:         parent.ProjectID,
		SubmittedByUserID: req.ActorUserID,
		Title:             title,
		Content:           req.Content,
		SourceType:        DemandSourceManual,
		SourceRefs: map[string]any{
			"continues_demand_id": parent.ID.String(),
		},
		Attachments:         req.Attachments,
		CoordinationMode:    parent.CoordinationMode,
		ScenarioTemplateKey: parent.ScenarioTemplateKey,
		ContinuesDemandID:   &continuesID,
	}

	// 成环不可能发生:continues_demand_id 只在新建行时写一次,新行没有后代。
	// 因此这里不需要环校验,只需要深度上限兜住畸形历史数据。
	return s.SubmitDemand(ctx, submit)
}

// ListDemandContinuationChain 返回该 demand 所属接续链(链头在前)。
func (s *Service) ListDemandContinuationChain(ctx context.Context, tenantID, demandID uuid.UUID) ([]ProjectDemand, error) {
	if tenantID == uuid.Nil || demandID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	return s.repository.ListProjectDemandContinuationChain(ctx, tenantID, demandID, DefaultDemandContinuationMaxDepth)
}

// DemandContinuationAvailability 是"这一单现在能不能接续"的服务端判据。
// 前端不自己算:能不能接续是业务规则,散到前端就会两处不一致。
type DemandContinuationAvailability struct {
	Available     bool
	ReasonCode    string
	ReasonMessage string
}

const (
	DemandContinuationReasonOK          = "ok"
	DemandContinuationReasonNotSettled  = "demand_not_settled"
	DemandContinuationReasonChainTooDee = "chain_too_deep"
)

func evaluateDemandContinuation(demand ProjectDemand, chainDepth int32) DemandContinuationAvailability {
	if !isTerminalProjectDemandStatus(demand.Status) {
		return DemandContinuationAvailability{
			ReasonCode:    DemandContinuationReasonNotSettled,
			ReasonMessage: "这一单还在进行中，结束后才能接续",
		}
	}
	if chainDepth+1 > DefaultDemandContinuationMaxDepth {
		return DemandContinuationAvailability{
			ReasonCode:    DemandContinuationReasonChainTooDee,
			ReasonMessage: fmt.Sprintf("接续次数已达上限 %d，请另开一单", DefaultDemandContinuationMaxDepth),
		}
	}
	return DemandContinuationAvailability{Available: true, ReasonCode: DemandContinuationReasonOK}
}

func continuationTitle(parentTitle string, position int32) string {
	trimmed := strings.TrimSpace(parentTitle)
	if trimmed == "" {
		trimmed = "需求"
	}
	return fmt.Sprintf("%s（接续 %d）", trimmed, position)
}
