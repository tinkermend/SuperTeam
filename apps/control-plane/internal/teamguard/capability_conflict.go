package teamguard

import (
	"fmt"
	"net/http"

	"github.com/superteam/control-plane/internal/apierror"
)

// ErrCapabilityProvidedByTeam 是"该能力已由团队提供，不必也不允许再绑一份个人的"这条
// 冲突的原型错误。规则见 spec §5.2.1：同一 MCP/技能在团队与员工两个维度只留一份，
// 团队胜出。此前 MCP 侧是写进库再由读路径静默屏蔽，产生了"个人绑定列表看得见、
// 生效列表看不见"的幽灵行；改为写时明确拒绝。
var ErrCapabilityProvidedByTeam = apierror.New(
	"team.capability.provided_by_team",
	http.StatusConflict,
	"该能力已由所属团队提供，无需重复绑定",
)

// CapabilityProvidedByTeamError 生成带能力名称的冲突错误。kind 用于组句（如 "MCP"、"技能"）。
func CapabilityProvidedByTeamError(kind, name string) error {
	return apierror.New(
		ErrCapabilityProvidedByTeam.Code,
		ErrCapabilityProvidedByTeam.Status,
		fmt.Sprintf("%s「%s」已由所属团队提供，无需重复绑定；如需按员工差异化配置，请先让团队解绑。", kind, name),
	)
}
