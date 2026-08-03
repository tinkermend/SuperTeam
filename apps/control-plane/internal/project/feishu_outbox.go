package project

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/humantask"
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

// feishuProjectMeta 是收件人展开顺带取到的项目元信息,用于卡片自足呈现。
type feishuProjectMeta struct {
	OwnerID uuid.UUID
	Name    string
}

// listFeishuRecipientsWithQueries 展开某项目的飞书收件人(合格处理人×绑定表)。
func (r *PgRepository) listFeishuRecipientsWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID uuid.UUID) ([]feishuRecipient, feishuProjectMeta, error) {
	projectRow, err := q.GetProject(ctx, queries.GetProjectParams{TenantID: tenantID, ID: projectID})
	if err != nil {
		return nil, feishuProjectMeta{}, err
	}
	meta := feishuProjectMeta{OwnerID: projectRow.HumanOwnerUserID, Name: projectRow.Name}
	members, err := q.ListProjectMembers(ctx, queries.ListProjectMembersParams{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return nil, feishuProjectMeta{}, err
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
		return nil, feishuProjectMeta{}, err
	}
	return expandFeishuRecipients(projectRow.HumanOwnerUserID, members, identities), meta, nil
}

// BuildDecisionCardPayload 组装决策卡快照:业务快照字段 + approval 富上下文。
// 富上下文(计划任务清单/验收判据/规划缺口等)best-effort:approval 行缺失或解析失败
// 时静默降级为薄快照——投影永不阻断业务。导出供 connector resolve 端点即时置换复用,
// 保证飞书终态卡与 outbox 决策卡同源同貌。
func BuildDecisionCardPayload(ctx context.Context, q *queries.Queries, decision DecisionRequest, projectName string) map[string]any {
	payload := map[string]any{
		"decision_type":  decision.DecisionType,
		"title":          decision.TitleSnapshot,
		"project_id":     decision.ProjectID.String(),
		"project_name":   projectName,
		"plan_revision":  uuidPtrString(decision.PlanRevisionID),
		"target_user_id": decision.TargetUserID.String(),
	}
	// 2026-07-25 §5.2: outbox carries kind from the single KindAndLayer map so
	// the connector never re-derives decision_type → kind.
	if kind, _ := humantask.KindAndLayer(decision.DecisionType); kind != "" {
		payload["kind"] = kind
	}
	if decision.SummarySnapshot != nil {
		payload["summary"] = *decision.SummarySnapshot
	}
	if decision.RiskLevelSnapshot != nil {
		payload["risk_level"] = *decision.RiskLevelSnapshot
	}
	approvalRow, err := q.GetApprovalRequest(ctx, queries.GetApprovalRequestParams{TenantID: decision.TenantID, ID: decision.ApprovalRequestID})
	if err != nil || len(approvalRow.ContextPayload) == 0 {
		return payload
	}
	var contextPayload map[string]any
	if json.Unmarshal(approvalRow.ContextPayload, &contextPayload) != nil || len(contextPayload) == 0 {
		return payload
	}
	payload["context"] = contextPayload
	if names := resolveFeishuEmployeeNames(ctx, q, decision.TenantID, contextPayload); len(names) > 0 {
		payload["employee_names"] = names
	}
	return payload
}

// resolveFeishuEmployeeNames 把计划任务里的 selected_employee_id 反查为展示名,
// 让卡片能显示"谁来干"。best-effort,查不到就不带该条。
func resolveFeishuEmployeeNames(ctx context.Context, q *queries.Queries, tenantID uuid.UUID, contextPayload map[string]any) map[string]string {
	tasks, _ := contextPayload["tasks"].([]any)
	names := map[string]string{}
	const maxLookups = 15
	for _, item := range tasks {
		task, _ := item.(map[string]any)
		rawID, _ := task["selected_employee_id"].(string)
		if rawID == "" {
			continue
		}
		if _, done := names[rawID]; done {
			continue
		}
		if len(names) >= maxLookups {
			break
		}
		employeeID, err := uuid.Parse(rawID)
		if err != nil {
			continue
		}
		employee, err := q.GetDigitalEmployee(ctx, queries.GetDigitalEmployeeParams{ID: employeeID, TenantID: tenantID})
		if err != nil {
			continue
		}
		names[rawID] = employee.Name
	}
	return names
}

// enqueueDecisionCardOutboxWithQueries 决策创建时展开收件人并入队审批卡。
// 全员未绑定 → 单行 skipped_unbound 留痕(recipient=owner, open_id 空)。
func (r *PgRepository) enqueueDecisionCardOutboxWithQueries(ctx context.Context, q *queries.Queries, decision DecisionRequest) error {
	recipients, projectMeta, err := r.listFeishuRecipientsWithQueries(ctx, q, decision.TenantID, decision.ProjectID)
	if err != nil {
		return err
	}
	ownerID := projectMeta.OwnerID
	payload := BuildDecisionCardPayload(ctx, q, decision, projectMeta.Name)
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
// 卡片按 feishu_message_id 入队更新。card_update payload 以原卡快照为底合并终态
// 信息——终态卡必须保留原始详情,飞书端不回控制台也能看清"批的是什么"。
//
// 幂等:可在 resolve 与 outbox ack 竞态恢复路径重复调用;已存在 pending/sent
// card_update 的 message_id 不再重复入队。sent 却缺 message_id 的行写 last_error
// 留痕,禁止静默跳过。
func (r *PgRepository) supersedeDecisionOutboxWithQueries(ctx context.Context, q *queries.Queries, decision DecisionRequest, resolvedBy uuid.UUID, comment string) error {
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
	existingUpdates, err := q.ListPendingOrSentCardUpdatesByResource(ctx, queries.ListPendingOrSentCardUpdatesByResourceParams{
		TenantID:     decision.TenantID,
		ResourceType: feishuOutboxResourceDecision,
		ResourceID:   decision.ID,
	})
	if err != nil {
		return err
	}
	alreadyQueued := cardUpdateMessageIDs(existingUpdates)
	resolvedByName := r.lookupUserNameWithQueries(ctx, q, resolvedBy)
	for _, row := range sentRows {
		if !row.FeishuMessageID.Valid || row.FeishuMessageID.String == "" {
			// 可观测:sent 却无 message_id,无法 Patch 原卡——写 last_error 留痕。
			_ = q.SetFeishuOutboxLastError(ctx, queries.SetFeishuOutboxLastErrorParams{
				TenantID:  decision.TenantID,
				ID:        row.ID,
				LastError: "missing_feishu_message_id_on_resolve",
			})
			continue
		}
		messageID := row.FeishuMessageID.String
		if _, exists := alreadyQueued[messageID]; exists {
			continue
		}
		payload := map[string]any{}
		// 原卡快照 best-effort 打底;历史行 payload 损坏时仍能发出薄终态卡。
		_ = json.Unmarshal(row.Payload, &payload)
		mergeDecisionResolvedPayload(payload, decision, messageID, resolvedBy, resolvedByName, comment)
		payloadJSON, err := json.Marshal(payload)
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
		alreadyQueued[messageID] = struct{}{}
	}
	return nil
}

// EnsureDecisionCardsTerminal 是 supersede 的导出入口:resolve 幂等重试与
// outbox ack 竞态恢复共用。决策仍为 pending 时 no-op。
func (r *PgRepository) EnsureDecisionCardsTerminal(ctx context.Context, decision DecisionRequest, resolvedBy uuid.UUID, comment string) error {
	if r == nil || r.q == nil {
		return nil
	}
	if isPendingDecisionStatus(decision.StatusSnapshot) {
		return nil
	}
	return r.supersedeDecisionOutboxWithQueries(ctx, r.q, decision, resolvedBy, comment)
}

// mergeDecisionResolvedPayload 把终态信息写入卡片 payload(原卡快照为底)。
func mergeDecisionResolvedPayload(payload map[string]any, decision DecisionRequest, messageID string, resolvedBy uuid.UUID, resolvedByName, comment string) {
	if payload == nil {
		return
	}
	payload["decision_type"] = decision.DecisionType
	if kind, _ := humantask.KindAndLayer(decision.DecisionType); kind != "" {
		payload["kind"] = kind
	}
	payload["title"] = decision.TitleSnapshot
	payload["resolved_status"] = decision.StatusSnapshot
	payload["feishu_message_id"] = messageID
	if resolvedByName != "" {
		payload["resolved_by_name"] = resolvedByName
	}
	if resolvedBy != uuid.Nil {
		payload["resolved_by_user_id"] = resolvedBy.String()
	}
	if comment != "" {
		payload["resolution_comment"] = comment
	}
	if decision.ResolvedAt != nil {
		payload["resolved_at"] = decision.ResolvedAt.Format(time.RFC3339)
	}
}

// cardUpdateMessageIDs 从已有 card_update 行提取目标 message_id(列优先,payload 兜底)。
func cardUpdateMessageIDs(rows []queries.FeishuOutbox) map[string]struct{} {
	out := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.FeishuMessageID.Valid && row.FeishuMessageID.String != "" {
			out[row.FeishuMessageID.String] = struct{}{}
			continue
		}
		if len(row.Payload) == 0 {
			continue
		}
		var payload map[string]any
		if json.Unmarshal(row.Payload, &payload) != nil {
			continue
		}
		if mid, _ := payload["feishu_message_id"].(string); mid != "" {
			out[mid] = struct{}{}
		}
	}
	return out
}

// latestTaskResultSummaryWithQueries 取任务最终 result 契约里的结论文本(summary)。
// best-effort:取不到返回空串。契约入库时已过脱敏链路,可直接投影。
func (r *PgRepository) latestTaskResultSummaryWithQueries(ctx context.Context, q *queries.Queries, task queries.ProjectTask) string {
	if !task.LatestTaskResultID.Valid || task.LatestTaskResultID.UUID == uuid.Nil {
		return ""
	}
	results, err := q.ListProjectTaskResults(ctx, queries.ListProjectTaskResultsParams{
		TenantID:      task.TenantID,
		ProjectID:     task.ProjectID,
		ProjectTaskID: task.ID,
		Limit:         20,
		Offset:        0,
	})
	if err != nil {
		return ""
	}
	for _, row := range results {
		if row.ID != task.LatestTaskResultID.UUID {
			continue
		}
		var contract struct {
			Summary string `json:"summary"`
		}
		if json.Unmarshal(row.ContractPayload, &contract) == nil {
			return contract.Summary
		}
		return ""
	}
	return ""
}

// lookupUserNameWithQueries 反查用户展示名(display_name 缺省回落 username)。
// best-effort:查不到返回空串,不阻断调用方。
func (r *PgRepository) lookupUserNameWithQueries(ctx context.Context, q *queries.Queries, userID uuid.UUID) string {
	if userID == uuid.Nil {
		return ""
	}
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		return ""
	}
	if user.DisplayName.Valid && user.DisplayName.String != "" {
		return user.DisplayName.String
	}
	return user.Username
}

// enqueueDemandResultNoticeWithQueries 需求终态(completed/failed)只读通知。
// acceptance_pending 不发通知——它由 demand_acceptance 决策卡承载,避免双消息。
// 通知带需求原文摘录与任务完成/失败清单——手机端不回控制台也能看清结果全貌;
// 结论快照(project_demand_summaries)由 coordinator 在终态后异步补写,此刻取不到,
// 故从同事务可见的任务事实取材。
func (r *PgRepository) enqueueDemandResultNoticeWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID, demandID uuid.UUID, status ProjectDemandStatus) error {
	if status != ProjectDemandStatusCompleted && status != ProjectDemandStatusFailed {
		return nil
	}
	recipients, projectMeta, err := r.listFeishuRecipientsWithQueries(ctx, q, tenantID, projectID)
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
	payload := map[string]any{
		"demand_id":    demandID.String(),
		"title":        demand.Title,
		"status":       string(status),
		"project_id":   projectID.String(),
		"project_name": projectMeta.Name,
	}
	if demand.Content.Valid && demand.Content.String != "" {
		payload["content_excerpt"] = clampRunes(demand.Content.String, 300)
	}
	// 任务清单与执行结论 best-effort:失败时点名失败任务;把各任务最终 result 的
	// 结论文本(员工交付的收尾结论,已脱敏)推进卡片——收到结果通知就能看到"做出了
	// 什么",不用回控制台。需求级结论快照由 coordinator 终态后异步补写,此刻取不到,
	// 任务级结论在同事务里是现成的。
	if tasks, err := q.ListProjectTasksByDemand(ctx, queries.ListProjectTasksByDemandParams{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID,
	}); err == nil && len(tasks) > 0 {
		completed, failed := 0, 0
		failedTitles := make([]string, 0, 3)
		type taskConclusion struct {
			title      string
			conclusion string
			updatedAt  time.Time
		}
		conclusions := make([]taskConclusion, 0, len(tasks))
		for _, task := range tasks {
			switch task.Status {
			case "completed":
				completed++
			case "failed":
				failed++
				if len(failedTitles) < 3 {
					failedTitles = append(failedTitles, task.Title)
				}
			}
			if summary := r.latestTaskResultSummaryWithQueries(ctx, q, task); summary != "" {
				conclusions = append(conclusions, taskConclusion{title: task.Title, conclusion: summary, updatedAt: task.UpdatedAt.Time})
			}
		}
		payload["task_total"] = len(tasks)
		payload["task_completed"] = completed
		payload["task_failed"] = failed
		if len(failedTitles) > 0 {
			payload["failed_task_titles"] = failedTitles
		}
		if len(conclusions) > 0 {
			// 最后完成的任务在前——多任务需求里它最接近"最终答案"。
			sort.Slice(conclusions, func(i, j int) bool { return conclusions[i].updatedAt.After(conclusions[j].updatedAt) })
			if len(conclusions) > 3 {
				conclusions = conclusions[:3]
			}
			entries := make([]map[string]any, 0, len(conclusions))
			for _, c := range conclusions {
				entries = append(entries, map[string]any{
					"title":      c.title,
					"conclusion": clampRunes(c.conclusion, 800),
				})
			}
			payload["task_conclusions"] = entries
		}
	}
	payloadJSON, err := json.Marshal(payload)
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

func clampRunes(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "…"
}
