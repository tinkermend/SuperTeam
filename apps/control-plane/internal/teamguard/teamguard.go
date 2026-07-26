// Package teamguard 承载"数字员工脱离团队"的共享判据：移出团队（回候岗大厅）与
// 换队是同一件事的两个入口，必须用同一套阻断项与同一条错误消息，否则一个拦得住、
// 另一个绕得过。数据来源是共享 sqlc 查询 ListDigitalEmployeeDetachBlockers。
package teamguard

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/superteam/control-plane/internal/apierror"
)

// DetachBlocker 是数字员工脱离当前团队的阻断项。
type DetachBlocker struct {
	// Type: BlockerActiveRun / BlockerActiveProject
	Type   string
	RefID  string
	Name   string
	Status string
}

const (
	// BlockerActiveRun 在役执行：换队/移出会重算 agent 家目录并切换继承，直接打断正在跑的活。
	BlockerActiveRun = "active_run"
	// BlockerActiveProject 仍被非归档项目引用：无团队归属的员工不能参与项目，
	// 静默移出会让项目挂在一个再也派发不出去的成员上。
	BlockerActiveProject = "active_project"
)

// ErrDetachBlocked 是被守卫拦下的原型错误。前端按 code 分支、直接展示 message。
var ErrDetachBlocked = apierror.New(
	"team.digital_employee.detach_blocked",
	http.StatusConflict,
	"该数字员工当前不可脱离团队",
)

// nameSampleLimit 控制 message 里点名的对象数量：够定位，又不至于撑爆一屏。
const nameSampleLimit = 3

// BlockedError 把阻断项汇总成一条可直接展示的中文错误；无阻断项返回 nil。
// action 是动作名（如「移出团队」「换队」），拼进句子的动词位。
func BlockedError(blockers []DetachBlocker, action string) error {
	if len(blockers) == 0 {
		return nil
	}
	var runs, projects []DetachBlocker
	for _, blocker := range blockers {
		switch blocker.Type {
		case BlockerActiveRun:
			runs = append(runs, blocker)
		case BlockerActiveProject:
			projects = append(projects, blocker)
		}
	}
	// 每个 clause 自带谓语（"有…" / "被…引用"），拼接后才读得通；建议句也只提
	// 真正命中的那一类，不给没发生的情况派活。
	var clauses, advice []string
	if len(runs) > 0 {
		clauses = append(clauses, fmt.Sprintf("有 %d 个在役执行（%s）", len(runs), names(runs)))
		advice = append(advice, "等待或取消在役执行")
	}
	if len(projects) > 0 {
		clauses = append(clauses, fmt.Sprintf("被 %d 个进行中项目引用（%s）", len(projects), names(projects)))
		advice = append(advice, "从相关项目中移除该成员")
	}
	if len(clauses) == 0 {
		return nil
	}
	message := fmt.Sprintf(
		"该数字员工%s，无法%s。请先%s后重试。",
		strings.Join(clauses, "，且"),
		action,
		strings.Join(advice, "、"),
	)
	return apierror.New(ErrDetachBlocked.Code, ErrDetachBlocked.Status, message)
}

func names(blockers []DetachBlocker) string {
	sample := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		name := strings.TrimSpace(blocker.Name)
		if name == "" {
			// 名称缺失时退回 id，避免出现空括号；技术兜底，不是常态。
			name = blocker.RefID
		}
		sample = append(sample, name)
		if len(sample) == nameSampleLimit {
			break
		}
	}
	joined := strings.Join(sample, "、")
	if len(blockers) > len(sample) {
		joined = fmt.Sprintf("%s 等 %d 个", joined, len(blockers))
	}
	return joined
}
