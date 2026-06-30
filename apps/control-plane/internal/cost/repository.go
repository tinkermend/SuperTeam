package cost

import "context"

type Repository interface {
	GetEmployeeSummary(ctx context.Context, p QueryParams) ([]EmployeeSummaryRow, error)
	GetDailyTrend(ctx context.Context, p QueryParams) ([]DailyTrendRow, error)
}
