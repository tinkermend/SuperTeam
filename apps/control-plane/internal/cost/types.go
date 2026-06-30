package cost

import (
	"time"

	"github.com/google/uuid"
)

// EmployeeSummaryRow 员工维度 token 汇总（跨天合并）
type EmployeeSummaryRow struct {
	DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
	EmployeeName      string    `json:"employee_name"`
	ProviderType      string    `json:"provider_type"`
	RunCount          int32     `json:"run_count"`
	TotalTokens       int64     `json:"total_tokens"`
}

// DailyTrendRow 每日趋势行
type DailyTrendRow struct {
	Day          time.Time `json:"day"`
	ProviderType string    `json:"provider_type"`
	RunCount     int32     `json:"run_count"`
	TotalTokens  int64     `json:"total_tokens"`
}

// TenantCostSummary 租户级成本汇总
type TenantCostSummary struct {
	PeriodStart time.Time            `json:"period_start"`
	PeriodEnd   time.Time            `json:"period_end"`
	TotalTokens int64                `json:"total_tokens"`
	TotalRuns   int32                `json:"total_runs"`
	ByEmployee  []EmployeeSummaryRow `json:"by_employee"`
	ByProvider  map[string]int64     `json:"by_provider"`
	DailyTrend  []DailyTrendRow      `json:"daily_trend"`
}

// EmployeeCostList 员工成本列表
type EmployeeCostList struct {
	PeriodStart time.Time            `json:"period_start"`
	PeriodEnd   time.Time            `json:"period_end"`
	Items       []EmployeeSummaryRow `json:"items"`
}

// QueryParams 查询参数
type QueryParams struct {
	TenantID  uuid.UUID
	StartDate time.Time
	EndDate   time.Time
}
