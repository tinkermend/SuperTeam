// Package cards 渲染飞书交互卡片(纯函数,输出卡片 JSON 字符串)。
// 分级规则见 2026-07-25 human-task-load-budget §5.3：按 HumanTask kind 查
// kindInteractionGrades；acceptance_sign 卡内逐条签署、禁止一键全过；
// result_notice 只读。
package cards

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/superteam/feishu-connector/internal/cpclient"
)

func render(card map[string]any) string {
	raw, err := json.Marshal(card)
	if err != nil {
		return `{"config":{},"elements":[]}`
	}
	return string(raw)
}

func header(title, template string) map[string]any {
	return map[string]any{
		"title":    map[string]any{"tag": "plain_text", "content": title},
		"template": template,
	}
}

func mdBlock(content string) map[string]any {
	return map[string]any{"tag": "div", "text": map[string]any{"tag": "lark_md", "content": content}}
}

func actionButton(text, buttonType string, value map[string]any) map[string]any {
	return map[string]any{
		"tag":   "button",
		"text":  map[string]any{"tag": "plain_text", "content": text},
		"type":  buttonType,
		"value": value,
	}
}

func linkButton(text, url string) map[string]any {
	return map[string]any{
		"tag":  "button",
		"text": map[string]any{"tag": "plain_text", "content": text},
		"type": "default",
		"url":  url,
	}
}

func card(headerBlock map[string]any, elements ...map[string]any) string {
	return render(map[string]any{
		"config":   map[string]any{"wide_screen_mode": true},
		"header":   headerBlock,
		"elements": elements,
	})
}

// GuideCard 普通消息的引导卡:显式意图,不隐式建单。
func GuideCard() string {
	return card(
		header("SuperTeam 助手", "blue"),
		mdBlock("我能帮你把工作带进 SuperTeam:\n- 发送 **提需求** 发起一条项目需求\n- 审批与结果会主动推送给你\n- 其余操作请使用 Console"),
	)
}

// ProjectPickCard 项目选择(飞书单卡按钮上限保守取前 10 个)。
func ProjectPickCard(projects []cpclient.MyProject) string {
	buttons := make([]map[string]any, 0, 10)
	for i, project := range projects {
		if i >= 10 {
			break
		}
		buttons = append(buttons, actionButton(project.Name, "default", map[string]any{
			"action":       "pick_project",
			"project_id":   project.ID,
			"project_name": project.Name,
		}))
	}
	elements := []map[string]any{mdBlock("选择要提需求的项目:")}
	elements = append(elements, map[string]any{"tag": "action", "actions": buttons})
	if len(projects) > 10 {
		elements = append(elements, mdBlock(fmt.Sprintf("_仅显示前 10 个(共 %d 个),更多项目请用 Console。_", len(projects))))
	}
	return card(header("提需求 · 选择项目", "blue"), elements...)
}

// ModePickCard 协调模式选择。
func ModePickCard(projectName string) string {
	return card(
		header("提需求 · 协调模式", "blue"),
		mdBlock(fmt.Sprintf("项目:**%s**\n选择协调模式:\n- **计划(plan)**:先出计划,人确认后执行\n- **循环(loop)**:按节奏持续推进", projectName)),
		map[string]any{"tag": "action", "actions": []map[string]any{
			actionButton("计划 plan", "primary", map[string]any{"action": "pick_mode", "mode": "plan"}),
			actionButton("循环 loop", "default", map[string]any{"action": "pick_mode", "mode": "loop"}),
		}},
	)
}

// DemandConfirmCard 提交前确认。
func DemandConfirmCard(projectName, mode, title, content string) string {
	return card(
		header("提需求 · 确认提交", "turquoise"),
		mdBlock(fmt.Sprintf("**项目**:%s\n**模式**:%s\n**标题**:%s", projectName, mode, title)),
		mdBlock(clamp(content, 800)),
		map[string]any{"tag": "action", "actions": []map[string]any{
			actionButton("确认提交", "primary", map[string]any{"action": "confirm_demand"}),
			actionButton("取消", "default", map[string]any{"action": "cancel_demand"}),
		}},
	)
}

// DemandReceiptCard 提交回执。
func DemandReceiptCard(title, demandID, deepLink string) string {
	return card(
		header("需求已提交", "green"),
		mdBlock(fmt.Sprintf("**%s**\n需求 ID:`%s`\n协调线程已接手,审批与结果会推送给你。", title, demandID)),
		map[string]any{"tag": "action", "actions": []map[string]any{linkButton("在 Console 查看", deepLink)}},
	)
}

// strSlice 宽容取字符串数组(payload 经 JSON 往返后为 []any)。
func strSlice(v any) []string {
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// mapSlice 宽容取对象数组。
func mapSlice(v any) []map[string]any {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// listSection 把条目列表渲染成一个带标题的 markdown 区块,超过 maxItems 截断留痕。
func listSection(title string, lines []string, maxItems int) map[string]any {
	shown := lines
	if len(shown) > maxItems {
		shown = shown[:maxItems]
	}
	content := "**" + title + "**\n" + strings.Join(shown, "\n")
	if len(lines) > maxItems {
		content += fmt.Sprintf("\n_…共 %d 条,其余见 Console_", len(lines))
	}
	return mdBlock(content)
}

// decisionHeadElements 渲染决策卡头部信息区:项目/类型/风险/摘要。
func decisionHeadElements(payload map[string]any) []map[string]any {
	kind := resolvePayloadKind(payload)
	summary, _ := payload["summary"].(string)
	risk, _ := payload["risk_level"].(string)
	projectName, _ := payload["project_name"].(string)

	info := ""
	if projectName != "" {
		info = fmt.Sprintf("**项目**:%s\n", projectName)
	}
	info += fmt.Sprintf("**类型**:%s", humanTaskKindLabel(kind))
	if risk != "" {
		info += fmt.Sprintf("  **风险**:%s", riskLabel(risk))
	}
	elements := []map[string]any{mdBlock(info)}
	if summary != "" {
		elements = append(elements, mdBlock(clamp(summary, 1200)))
	}
	return elements
}

// decisionBodyElements 渲染决策卡信息区:头部+按 kind 展开的富上下文(静态,无按钮)。
// 决策卡与终态卡共用——批准之后卡片必须保留"批的是什么",不逼人回控制台。
func decisionBodyElements(payload map[string]any) []map[string]any {
	kind := resolvePayloadKind(payload)
	elements := decisionHeadElements(payload)
	context, _ := payload["context"].(map[string]any)
	if context != nil {
		elements = append(elements, decisionContextElements(kind, context, payload)...)
	}
	return elements
}

// verdictOverlay 宽容取 payload 里的判据 verdict 覆盖(JSON 往返后为 map[string]any)。
func verdictOverlay(v any) map[string]string {
	raw, _ := v.(map[string]any)
	out := map[string]string{}
	for key, value := range raw {
		if s, ok := value.(string); ok {
			out[key] = s
		}
	}
	return out
}

// acceptanceSignElements 渲染验收判据区:每条判据一行(verdict 图标+证据摘录紧邻),
// interactive 时未签条目后跟「通过/不通过」按钮——签署紧邻证据,防橡皮图章的卡内版。
func acceptanceSignElements(payload map[string]any, verdicts map[string]string, interactive bool, decisionID, projectID string) []map[string]any {
	context, _ := payload["context"].(map[string]any)
	if context == nil {
		return nil
	}
	demandID, _ := context["demand_id"].(string)
	detail := mapSlice(context["pending_criteria_detail"])
	if len(detail) == 0 {
		if pending := strSlice(context["pending_criteria"]); len(pending) > 0 {
			return []map[string]any{mdBlock(fmt.Sprintf("**待签署判据**:%d 条,明细见 Console。", len(pending)))}
		}
		return nil
	}
	elements := []map[string]any{mdBlock(fmt.Sprintf("**待签署判据(%d 条)**", len(detail)))}
	const maxRows = 8
	for i, criterion := range detail {
		if i >= maxRows {
			elements = append(elements, mdBlock(fmt.Sprintf("_…共 %d 条判据,其余到 Console 签署_", len(detail))))
			break
		}
		id, _ := criterion["id"].(string)
		statement, _ := criterion["statement"].(string)
		verdict := verdicts[id]
		icon := "☐"
		switch verdict {
		case "satisfied":
			icon = "✅"
		case "unsatisfied":
			icon = "❌"
		}
		line := fmt.Sprintf("%s **%d. %s**", icon, i+1, clamp(statement, 120))
		if method, _ := criterion["verification_method"].(string); method == "human_judgment" {
			line += "(人工判断)"
		}
		for j, entry := range mapSlice(criterion["evidence"]) {
			if j >= 2 {
				break
			}
			taskTitle, _ := entry["title"].(string)
			conclusion, _ := entry["conclusion"].(string)
			if conclusion == "" {
				status, _ := entry["status"].(string)
				conclusion = "(任务 " + status + ",无结论摘录)"
			}
			line += fmt.Sprintf("\n> %s:%s", clamp(taskTitle, 40), clamp(conclusion, 140))
		}
		elements = append(elements, mdBlock(line))
		if interactive && verdict == "" && demandID != "" && id != "" {
			signValue := func(signVerdict string) map[string]any {
				return map[string]any{
					"action":       "sign_criterion",
					"demand_id":    demandID,
					"project_id":   projectID,
					"decision_id":  decisionID,
					"criterion_id": id,
					"verdict":      signVerdict,
					"statement":    clamp(statement, 80),
				}
			}
			elements = append(elements, map[string]any{"tag": "action", "actions": []map[string]any{
				actionButton(fmt.Sprintf("通过 %d", i+1), "primary", signValue("satisfied")),
				actionButton(fmt.Sprintf("不通过 %d", i+1), "danger", signValue("unsatisfied")),
			}})
		}
	}
	return elements
}

// decisionContextElements 按 HumanTask kind 渲染富上下文区块;未知 kind 静默跳过(薄卡兜底)。
func decisionContextElements(kind string, context map[string]any, payload map[string]any) []map[string]any {
	var sections []map[string]any
	switch kind {
	case "plan_review":
		employeeNames, _ := payload["employee_names"].(map[string]any)
		tasks := mapSlice(context["tasks"])
		lines := make([]string, 0, len(tasks))
		for i, task := range tasks {
			title, _ := task["title"].(string)
			line := fmt.Sprintf("%d. %s", i+1, clamp(title, 60))
			if employeeID, _ := task["selected_employee_id"].(string); employeeID != "" {
				if name, _ := employeeNames[employeeID].(string); name != "" {
					line += fmt.Sprintf("(%s)", name)
				}
			}
			lines = append(lines, line)
		}
		if len(lines) > 0 {
			sections = append(sections, listSection(fmt.Sprintf("计划任务(%d 项)", len(lines)), lines, 8))
		}
		criteria := mapSlice(context["plan_acceptance_criteria"])
		criteriaLines := make([]string, 0, len(criteria))
		for _, criterion := range criteria {
			statement, _ := criterion["statement"].(string)
			if statement == "" {
				continue
			}
			criteriaLines = append(criteriaLines, "• "+clamp(statement, 100))
		}
		if len(criteriaLines) > 0 {
			sections = append(sections, listSection("验收判据", criteriaLines, 8))
		}
		if riskAssessment, ok := context["risk_assessment"].(map[string]any); ok {
			if keys := strSlice(riskAssessment["high_risk_task_keys"]); len(keys) > 0 {
				sections = append(sections, mdBlock("**高风险任务**:"+clamp(strings.Join(keys, "、"), 200)))
			}
		}
		if humanReview, ok := context["human_review"].(map[string]any); ok {
			if reasons := strSlice(humanReview["reasons"]); len(reasons) > 0 {
				sections = append(sections, mdBlock("**需人工确认原因**:"+clamp(strings.Join(reasons, ";"), 300)))
			}
		}
	case "acceptance_sign":
		// 静态渲染(终态卡等场景):同一判据行渲染器,无签署按钮。
		sections = append(sections, acceptanceSignElements(payload, verdictOverlay(payload["criterion_verdicts"]), false, "", "")...)
	case "planning_gap":
		if gap, ok := context["gap"].(map[string]any); ok {
			gapInfo := ""
			if constraintKind, _ := gap["constraint_kind"].(string); constraintKind != "" {
				gapInfo += "**缺口类型**:" + constraintKind + "\n"
			}
			if roles := strSlice(gap["roles"]); len(roles) > 0 {
				gapInfo += "**缺口角色**:" + clamp(strings.Join(roles, "、"), 150) + "\n"
			}
			if capabilities := strSlice(gap["required_capabilities"]); len(capabilities) > 0 {
				gapInfo += "**所需能力**:" + clamp(strings.Join(capabilities, "、"), 200) + "\n"
			}
			if count, ok := gap["active_executor_count"].(float64); ok {
				gapInfo += fmt.Sprintf("**当前可用执行者**:%d 个", int(count))
			}
			if gapInfo != "" {
				sections = append(sections, mdBlock(strings.TrimRight(gapInfo, "\n")))
			}
		}
	case "dispatch_release", "downstream_release":
		if risk, _ := payload["risk_level"].(string); risk != "" {
			sections = append(sections, mdBlock("**风险等级**:"+riskLabel(risk)))
		}
		if summary, _ := payload["summary"].(string); summary != "" {
			sections = append(sections, mdBlock("**动作意图**:"+clamp(summary, 400)))
		}
		if title, _ := context["task_title"].(string); title != "" {
			sections = append(sections, mdBlock("**任务**:"+clamp(title, 120)))
		}
	case "closure_confirm":
		if demands := mapSlice(context["demands"]); len(demands) > 0 {
			lines := make([]string, 0, len(demands))
			for _, demand := range demands {
				title, _ := demand["title"].(string)
				if title == "" {
					continue
				}
				line := "• " + clamp(title, 100)
				// 需求清单含全部终态需求(完成/失败/取消),必须逐条标状态,
				// 否则被取消的需求在卡上与已完成的无法区分。
				label, _ := demand["status_label"].(string)
				if label == "" {
					status, _ := demand["status"].(string)
					label = demandStatusLabel(status)
				}
				if label != "" {
					line += " · " + label
				}
				lines = append(lines, line)
			}
			if len(lines) > 0 {
				sections = append(sections, listSection("需求清单", lines, 8))
			}
		}
	case "planning_failed":
		if reason, _ := context["failure_reason"].(string); reason != "" {
			sections = append(sections, mdBlock("**失败原因**:"+clamp(reason, 400)))
		} else if summary, _ := payload["summary"].(string); summary != "" {
			sections = append(sections, mdBlock("**失败原因**:"+clamp(summary, 400)))
		}
	case "upstream_supplement_review", "project_task_upstream_supplement_review":
		if missing := strSlice(context["missing_inputs"]); len(missing) > 0 {
			lines := make([]string, 0, len(missing))
			for _, item := range missing {
				lines = append(lines, "• "+clamp(item, 100))
			}
			sections = append(sections, listSection("缺失的上游输入", lines, 8))
		}
	case "task_failure_recovery", "project_task_iteration_exhausted":
		if downstream := strSlice(context["downstream_task_ids"]); len(downstream) > 0 {
			sections = append(sections, mdBlock(fmt.Sprintf("**影响范围**:%d 个下游任务已挂起,等待此决策。", len(downstream))))
		}
	}
	return sections
}

// DecisionCard 审批卡:按 HumanTask kind 查交互分级表渲染。payload 来自 CP outbox。
func DecisionCard(payload map[string]any, decisionID, projectID, webOrigin string) string {
	kind := resolvePayloadKind(payload)
	title, _ := payload["title"].(string)
	risk, _ := payload["risk_level"].(string)
	deepLink := webOrigin + "/inbox"

	resolveValue := func(decision string) map[string]any {
		return map[string]any{
			"action":      "resolve_decision",
			"decision_id": decisionID,
			"project_id":  projectID,
			"decision":    decision,
			"title":       title,
		}
	}

	grade, hasGrade := gradeForKind(kind)
	var actions []map[string]any
	switch {
	case hasGrade && grade.Mode == ModePerCriterionSign:
		// 卡内逐条签署;深链只作完整证据血缘兜底,不给一键全过。
		actions = []map[string]any{linkButton("在 Console 查看完整证据", deepLink)}
	case hasGrade && grade.Mode == ModeCardActions:
		for _, entry := range grade.Actions {
			actions = append(actions, actionButton(entry.Label, entry.ButtonType, resolveValue(entry.Decision)))
		}
		actions = append(actions, linkButton("查看详情", deepLink))
	default:
		actions = []map[string]any{linkButton("到 Console 处理", deepLink)}
	}

	var elements []map[string]any
	if hasGrade && grade.Mode == ModePerCriterionSign {
		elements = decisionHeadElements(payload)
		elements = append(elements, acceptanceSignElements(payload, verdictOverlay(payload["criterion_verdicts"]), true, decisionID, projectID)...)
	} else {
		elements = decisionBodyElements(payload)
	}
	elements = append(elements, map[string]any{"tag": "action", "actions": actions})
	return card(header("待你处理:"+clamp(title, 80), riskTemplate(risk)), elements...)
}

// AcceptanceProgressCard 卡内签署一条后的整卡重渲染:签署进度+verdict 覆盖,
// 未签条目保留按钮;需求收敛(completed/failed)后转终态样式无按钮。
func AcceptanceProgressCard(payload map[string]any, verdicts map[string]string, signed, total, remaining int32, demandStatus, decisionID, projectID, webOrigin string) string {
	title, _ := payload["title"].(string)
	headerTitle := "验收签署中:" + clamp(title, 70)
	template := "orange"
	interactive := true
	switch demandStatus {
	case "completed":
		headerTitle = "验收完成:" + clamp(title, 70)
		template = "green"
		interactive = false
	case "failed":
		headerTitle = "验收未通过:" + clamp(title, 70)
		template = "red"
		interactive = false
	}
	elements := []map[string]any{mdBlock(fmt.Sprintf("**签署进度**:%d/%d,剩余 %d 条", signed, total, remaining))}
	elements = append(elements, decisionHeadElements(payload)...)
	elements = append(elements, acceptanceSignElements(payload, verdicts, interactive, decisionID, projectID)...)
	elements = append(elements, map[string]any{"tag": "action", "actions": []map[string]any{
		linkButton("在 Console 查看完整证据", strings.TrimRight(webOrigin, "/")+"/inbox"),
	}})
	return card(header(headerTitle, template), elements...)
}

// DecisionResolvedCard 决策终态卡(即时置换与 card_update 整卡替换共用)。
// 保留原卡全部信息区,只把按钮换成结果与深链——在飞书上就能看清"处理的是什么"。
func DecisionResolvedCard(payload map[string]any, webOrigin string) string {
	title, _ := payload["title"].(string)
	status, _ := payload["resolved_status"].(string)

	result := "**处理结果**:" + statusLabel(status)
	if self, _ := payload["resolved_by_self"].(bool); self {
		result += "(你处理的)"
	} else if name, _ := payload["resolved_by_name"].(string); name != "" {
		result += "\n**处理人**:" + name
	}
	if comment, _ := payload["resolution_comment"].(string); comment != "" {
		result += "\n**说明**:" + clamp(comment, 300)
	}
	if resolvedAt, _ := payload["resolved_at"].(string); resolvedAt != "" {
		if at, err := time.Parse(time.RFC3339, resolvedAt); err == nil {
			result += "\n**处理时间**:" + at.Local().Format("2006-01-02 15:04")
		}
	}

	elements := []map[string]any{mdBlock(result)}
	elements = append(elements, decisionBodyElements(payload)...)
	elements = append(elements, map[string]any{"tag": "action", "actions": []map[string]any{
		linkButton("在 Console 查看", strings.TrimRight(webOrigin, "/")+"/inbox"),
	}})
	return card(header("已处理:"+clamp(title, 80), "grey"), elements...)
}

// ResultNoticeCard 需求终态只读通知:带需求原文摘录与任务完成/失败清单,
// 手机端不回控制台也能看清结果全貌。
func ResultNoticeCard(payload map[string]any, webOrigin string) string {
	title, _ := payload["title"].(string)
	status, _ := payload["status"].(string)
	demandID, _ := payload["demand_id"].(string)
	projectName, _ := payload["project_name"].(string)
	template := "green"
	label := "已完成"
	if status == "failed" {
		template = "red"
		label = "未通过"
	}

	info := ""
	if projectName != "" {
		info = fmt.Sprintf("**项目**:%s\n", projectName)
	}
	if total, ok := payload["task_total"].(float64); ok && total > 0 {
		completed, _ := payload["task_completed"].(float64)
		failed, _ := payload["task_failed"].(float64)
		info += fmt.Sprintf("**任务**:共 %d 项,完成 %d 项", int(total), int(completed))
		if failed > 0 {
			info += fmt.Sprintf(",失败 %d 项", int(failed))
		}
		info += "\n"
	}
	elements := []map[string]any{mdBlock(strings.TrimRight(info, "\n"))}
	// 执行结论置顶:员工交付的收尾结论是结果通知的正文,不是脚注。
	if conclusions := mapSlice(payload["task_conclusions"]); len(conclusions) > 0 {
		if len(conclusions) == 1 {
			conclusion, _ := conclusions[0]["conclusion"].(string)
			elements = append(elements, mdBlock("**执行结论**\n"+clamp(conclusion, 800)))
		} else {
			text := "**执行结论**"
			for _, entry := range conclusions {
				taskTitle, _ := entry["title"].(string)
				conclusion, _ := entry["conclusion"].(string)
				text += fmt.Sprintf("\n**%s**\n%s", clamp(taskTitle, 60), clamp(conclusion, 400))
			}
			elements = append(elements, mdBlock(text))
		}
	}
	if excerpt, _ := payload["content_excerpt"].(string); excerpt != "" {
		elements = append(elements, mdBlock("**需求内容**\n"+clamp(excerpt, 300)))
	}
	if failedTitles := strSlice(payload["failed_task_titles"]); len(failedTitles) > 0 {
		lines := make([]string, 0, len(failedTitles))
		for _, taskTitle := range failedTitles {
			lines = append(lines, "• "+clamp(taskTitle, 80))
		}
		elements = append(elements, listSection("失败任务", lines, 3))
	}
	elements = append(elements, map[string]any{"tag": "action", "actions": []map[string]any{
		linkButton("查看详情", fmt.Sprintf("%s/workflows/%s", strings.TrimRight(webOrigin, "/"), demandID)),
	}})
	return card(header(fmt.Sprintf("需求%s:%s", label, clamp(title, 70)), template), elements...)
}

func riskLabel(risk string) string {
	switch strings.ToLower(risk) {
	case "high":
		return "高"
	case "medium":
		return "中"
	case "low":
		return "低"
	default:
		return risk
	}
}

func statusLabel(status string) string {
	switch status {
	case "":
		return "已处理"
	case "approved":
		return "批准"
	case "rejected":
		return "驳回"
	case "request_changes":
		return "请求修改"
	case "restaffed":
		return "已补员重规划"
	case "exempted":
		return "豁免"
	default:
		return status
	}
}

func demandStatusLabel(status string) string {
	switch status {
	case "completed":
		return "已完成"
	case "failed":
		return "失败"
	case "cancelled":
		return "已取消"
	default:
		return status
	}
}

func riskTemplate(risk string) string {
	switch strings.ToLower(risk) {
	case "high":
		return "red"
	case "medium":
		return "orange"
	default:
		return "blue"
	}
}

func clamp(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + "…"
}
