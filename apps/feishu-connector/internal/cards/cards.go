// Package cards 渲染飞书交互卡片(纯函数,输出卡片 JSON 字符串)。
// 分级规则(spec §8.2):plan_review / planning_gap 卡内可操作;clarification 与
// demand_acceptance(判据签署)只给富信息+深链 Console——签署控件必须紧邻证据,
// 防橡皮图章;result_notice 只读。
package cards

import (
	"encoding/json"
	"fmt"
	"strings"

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

// DecisionCard 审批卡:按决策类型分级渲染。payload 来自控制平面 outbox 快照。
func DecisionCard(payload map[string]any, decisionID, projectID, webOrigin string) string {
	decisionType, _ := payload["decision_type"].(string)
	title, _ := payload["title"].(string)
	summary, _ := payload["summary"].(string)
	risk, _ := payload["risk_level"].(string)

	info := fmt.Sprintf("**类型**:%s", decisionTypeLabel(decisionType))
	if risk != "" {
		info += fmt.Sprintf("\n**风险**:%s", risk)
	}
	if summary != "" {
		info += "\n\n" + clamp(summary, 1500)
	}
	deepLink := webOrigin + "/inbox"

	resolveValue := func(decision string) map[string]any {
		return map[string]any{
			"action":      "resolve_decision",
			"decision_id": decisionID,
			"project_id":  projectID,
			"decision":    decision,
		}
	}

	var actions []map[string]any
	switch decisionType {
	case "plan_review":
		actions = []map[string]any{
			actionButton("批准", "primary", resolveValue("approved")),
			actionButton("请求修改", "danger", resolveValue("request_changes")),
			linkButton("查看计划详情", deepLink),
		}
	case "planning_gap":
		actions = []map[string]any{
			actionButton("已补员,重新规划", "primary", resolveValue("restaffed")),
			actionButton("豁免约束", "default", resolveValue("exempted")),
			actionButton("关闭需求", "danger", resolveValue("rejected")),
			linkButton("查看详情", deepLink),
		}
	case "demand_acceptance":
		// 判据签署必须紧邻证据——卡片只给信息与深链,不给一键签署。
		actions = []map[string]any{linkButton("到 Console 逐条签署判据", deepLink)}
	default:
		actions = []map[string]any{linkButton("到 Console 处理", deepLink)}
	}

	return card(
		header("待你处理:"+clamp(title, 80), riskTemplate(risk)),
		mdBlock(info),
		map[string]any{"tag": "action", "actions": actions},
	)
}

// DecisionResolvedCard 决策终态卡(card_update 整卡替换)。
func DecisionResolvedCard(payload map[string]any) string {
	title, _ := payload["title"].(string)
	status, _ := payload["resolved_status"].(string)
	return card(
		header("已处理:"+clamp(title, 80), "grey"),
		mdBlock(fmt.Sprintf("该决策已处理,结论:**%s**。卡片按钮已失效;详情见 Console。", statusLabel(status))),
	)
}

// ResultNoticeCard 需求终态只读通知。
func ResultNoticeCard(payload map[string]any, webOrigin string) string {
	title, _ := payload["title"].(string)
	status, _ := payload["status"].(string)
	demandID, _ := payload["demand_id"].(string)
	template := "green"
	label := "已完成"
	if status == "failed" {
		template = "red"
		label = "未通过"
	}
	return card(
		header(fmt.Sprintf("需求%s:%s", label, clamp(title, 70)), template),
		mdBlock(fmt.Sprintf("**状态**:%s\n需求 ID:`%s`", status, demandID)),
		map[string]any{"tag": "action", "actions": []map[string]any{
			linkButton("查看详情", fmt.Sprintf("%s/workflows/%s", strings.TrimRight(webOrigin, "/"), demandID)),
		}},
	)
}

func decisionTypeLabel(decisionType string) string {
	switch decisionType {
	case "plan_review":
		return "计划评审"
	case "planning_gap":
		return "规划缺口"
	case "demand_acceptance":
		return "需求验收(判据签署)"
	case "clarification":
		return "需求澄清"
	default:
		if decisionType == "" {
			return "人类决策"
		}
		return decisionType
	}
}

func statusLabel(status string) string {
	switch status {
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
