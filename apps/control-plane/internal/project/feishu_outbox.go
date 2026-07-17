package project

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/storage/queries"
)

// 飞书 outbox 写入钩子:与业务写同事务(同一 *queries.Queries),保证决策/终态
// 与其对外投影原子一致。飞书是投影不是事实源——任何 outbox 写入失败都不该
// 阻断业务,因此除行插入外不做外部调用;投递与重试归 connector。

const (
	feishuOutboxKindDecisionCard = "decision_card"
	feishuOutboxKindCardUpdate   = "card_update"
	feishuOutboxKindResultNotice = "result_notice"

	feishuOutboxResourceDecision = "decision_request"
	feishuOutboxResourceDemand   = "project_demand"
)

// feishuRecipient 是收件人展开结果:合格处理人 ∩ 已绑定飞书。
type feishuRecipient struct {
	UserID uuid.UUID
	OpenID string
}

// expandFeishuRecipients 纯函数:active human_user 成员 ∪ human_owner,再与绑定表求交。
func expandFeishuRecipients(ownerID uuid.UUID, members []queries.ProjectMember, identities []queries.UserFeishuIdentity) []feishuRecipient {
	eligible := map[uuid.UUID]bool{}
	if ownerID != uuid.Nil {
		eligible[ownerID] = true
	}
	for _, member := range members {
		if member.PrincipalType == string(PrincipalTypeHumanUser) && member.Status == "active" {
			eligible[member.PrincipalID] = true
		}
	}
	recipients := make([]feishuRecipient, 0, len(identities))
	seen := map[uuid.UUID]bool{}
	for _, identity := range identities {
		if !eligible[identity.AuthUserID] || seen[identity.AuthUserID] {
			continue
		}
		seen[identity.AuthUserID] = true
		recipients = append(recipients, feishuRecipient{UserID: identity.AuthUserID, OpenID: identity.OpenID})
	}
	return recipients
}

// listFeishuRecipientsWithQueries 展开某项目的飞书收件人(合格处理人×绑定表)。
func (r *PgRepository) listFeishuRecipientsWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID uuid.UUID) ([]feishuRecipient, uuid.UUID, error) {
	projectRow, err := q.GetProject(ctx, queries.GetProjectParams{TenantID: tenantID, ID: projectID})
	if err != nil {
		return nil, uuid.Nil, err
	}
	members, err := q.ListProjectMembers(ctx, queries.ListProjectMembersParams{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return nil, uuid.Nil, err
	}
	eligible := map[uuid.UUID]bool{projectRow.HumanOwnerUserID: true}
	userIDs := []uuid.UUID{projectRow.HumanOwnerUserID}
	for _, member := range members {
		if member.PrincipalType == string(PrincipalTypeHumanUser) && member.Status == "active" && !eligible[member.PrincipalID] {
			eligible[member.PrincipalID] = true
			userIDs = append(userIDs, member.PrincipalID)
		}
	}
	identities, err := q.ListFeishuIdentitiesByUsers(ctx, queries.ListFeishuIdentitiesByUsersParams{
		TenantID:    tenantID,
		AuthUserIds: userIDs,
	})
	if err != nil {
		return nil, uuid.Nil, err
	}
	return expandFeishuRecipients(projectRow.HumanOwnerUserID, members, identities), projectRow.HumanOwnerUserID, nil
}

// enqueueDecisionCardOutboxWithQueries 决策创建时展开收件人并入队审批卡。
// 全员未绑定 → 单行 skipped_unbound 留痕(recipient=owner, open_id 空)。
func (r *PgRepository) enqueueDecisionCardOutboxWithQueries(ctx context.Context, q *queries.Queries, decision DecisionRequest) error {
	recipients, ownerID, err := r.listFeishuRecipientsWithQueries(ctx, q, decision.TenantID, decision.ProjectID)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"decision_type":  decision.DecisionType,
		"title":          decision.TitleSnapshot,
		"project_id":     decision.ProjectID.String(),
		"plan_revision":  uuidPtrString(decision.PlanRevisionID),
		"target_user_id": decision.TargetUserID.String(),
	}
	if decision.SummarySnapshot != nil {
		payload["summary"] = *decision.SummarySnapshot
	}
	if decision.RiskLevelSnapshot != nil {
		payload["risk_level"] = *decision.RiskLevelSnapshot
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	projectID := decision.ProjectID
	if len(recipients) == 0 {
		_, err := q.CreateFeishuOutbox(ctx, queries.CreateFeishuOutboxParams{
			TenantID:        decision.TenantID,
			ProjectID:       uuid.NullUUID{UUID: projectID, Valid: true},
			Kind:            feishuOutboxKindDecisionCard,
			ResourceType:    feishuOutboxResourceDecision,
			ResourceID:      decision.ID,
			RecipientUserID: ownerID,
			RecipientOpenID: "",
			Payload:         payloadJSON,
			Status:          pgtype.Text{String: "skipped_unbound", Valid: true},
		})
		return err
	}
	for _, recipient := range recipients {
		if _, err := q.CreateFeishuOutbox(ctx, queries.CreateFeishuOutboxParams{
			TenantID:        decision.TenantID,
			ProjectID:       uuid.NullUUID{UUID: projectID, Valid: true},
			Kind:            feishuOutboxKindDecisionCard,
			ResourceType:    feishuOutboxResourceDecision,
			ResourceID:      decision.ID,
			RecipientUserID: recipient.UserID,
			RecipientOpenID: recipient.OpenID,
			Payload:         payloadJSON,
		}); err != nil {
			return err
		}
	}
	return nil
}

// supersedeDecisionOutboxWithQueries 决策 resolve 后:pending 卡片作废,已发送的
// 卡片按 feishu_message_id 入队更新(已处理态/已由他人处理态由 connector 渲染)。
func (r *PgRepository) supersedeDecisionOutboxWithQueries(ctx context.Context, q *queries.Queries, decision DecisionRequest) error {
	if err := q.SupersedePendingFeishuOutboxByResource(ctx, queries.SupersedePendingFeishuOutboxByResourceParams{
		TenantID:     decision.TenantID,
		ResourceType: feishuOutboxResourceDecision,
		ResourceID:   decision.ID,
	}); err != nil {
		return err
	}
	sentRows, err := q.ListSentFeishuOutboxByResource(ctx, queries.ListSentFeishuOutboxByResourceParams{
		TenantID:     decision.TenantID,
		ResourceType: feishuOutboxResourceDecision,
		ResourceID:   decision.ID,
	})
	if err != nil {
		return err
	}
	for _, row := range sentRows {
		if !row.FeishuMessageID.Valid || row.FeishuMessageID.String == "" {
			continue
		}
		payloadJSON, err := json.Marshal(map[string]any{
			"decision_type":     decision.DecisionType,
			"title":             decision.TitleSnapshot,
			"resolved_status":   decision.StatusSnapshot,
			"feishu_message_id": row.FeishuMessageID.String,
		})
		if err != nil {
			return err
		}
		if _, err := q.CreateFeishuOutbox(ctx, queries.CreateFeishuOutboxParams{
			TenantID:        decision.TenantID,
			ProjectID:       uuid.NullUUID{UUID: decision.ProjectID, Valid: true},
			Kind:            feishuOutboxKindCardUpdate,
			ResourceType:    feishuOutboxResourceDecision,
			ResourceID:      decision.ID,
			RecipientUserID: row.RecipientUserID,
			RecipientOpenID: row.RecipientOpenID,
			Payload:         payloadJSON,
		}); err != nil {
			return err
		}
	}
	return nil
}

// enqueueDemandResultNoticeWithQueries 需求终态(completed/failed)只读通知。
// acceptance_pending 不发通知——它由 demand_acceptance 决策卡承载,避免双消息。
func (r *PgRepository) enqueueDemandResultNoticeWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID, demandID uuid.UUID, status ProjectDemandStatus) error {
	if status != ProjectDemandStatusCompleted && status != ProjectDemandStatusFailed {
		return nil
	}
	recipients, _, err := r.listFeishuRecipientsWithQueries(ctx, q, tenantID, projectID)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return nil
	}
	demand, err := q.GetProjectDemand(ctx, queries.GetProjectDemandParams{TenantID: tenantID, ID: demandID})
	if err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(map[string]any{
		"demand_id":  demandID.String(),
		"title":      demand.Title,
		"status":     string(status),
		"project_id": projectID.String(),
	})
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		if _, err := q.CreateFeishuOutbox(ctx, queries.CreateFeishuOutboxParams{
			TenantID:        tenantID,
			ProjectID:       uuid.NullUUID{UUID: projectID, Valid: true},
			Kind:            feishuOutboxKindResultNotice,
			ResourceType:    feishuOutboxResourceDemand,
			ResourceID:      demandID,
			RecipientUserID: recipient.UserID,
			RecipientOpenID: recipient.OpenID,
			Payload:         payloadJSON,
		}); err != nil {
			return err
		}
	}
	return nil
}

func uuidPtrString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}
