package employee

import (
	"net/http"

	"github.com/superteam/control-plane/internal/apierror"
)

// 数字员工创建/管理的结构化错误：code 稳定、message 为权威中文提示（zh-first
// 单一源），经 apierror.Write 输出 {code, message}，前端直接展示 message、按 code
// 分支，不再靠英文错误文本关键词匹配。
var (
	ErrEmployeeNameConflict = apierror.New(
		"employee.name_conflict", http.StatusConflict,
		"该名称已被使用，请更换名称后重试。")
	ErrEmployeeAvatarInUse = apierror.New(
		"employee.avatar_in_use", http.StatusConflict,
		"该头像已被其他数字员工使用，请重新选择头像。")
	ErrEmployeeTeamCapacityExceeded = apierror.New(
		"employee.team_capacity_exceeded", http.StatusConflict,
		"团队数字员工数已达上限，请在系统配置调大上限或更换团队。")
)
