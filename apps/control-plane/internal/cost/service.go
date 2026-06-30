package cost

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidPeriod = errors.New("invalid period")

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// periodDates 将 period 字符串解析为 [start, end)
func periodDates(period string) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	end := now.Truncate(24 * time.Hour).Add(24 * time.Hour)
	var days int
	switch period {
	case "7d":
		days = 7
	case "30d", "":
		days = 30
	case "90d":
		days = 90
	default:
		return time.Time{}, time.Time{}, ErrInvalidPeriod
	}
	start := end.Add(-time.Duration(days) * 24 * time.Hour)
	return start, end, nil
}

func (s *Service) GetTenantCostSummary(ctx context.Context, tenantID uuid.UUID, period string) (TenantCostSummary, error) {
	start, end, err := periodDates(period)
	if err != nil {
		return TenantCostSummary{}, err
	}
	p := QueryParams{TenantID: tenantID, StartDate: start, EndDate: end}

	employees, err := s.repo.GetEmployeeSummary(ctx, p)
	if err != nil {
		return TenantCostSummary{}, err
	}
	trend, err := s.repo.GetDailyTrend(ctx, p)
	if err != nil {
		return TenantCostSummary{}, err
	}

	var totalTokens int64
	var totalRuns int32
	byProvider := make(map[string]int64)
	for _, e := range employees {
		totalTokens += e.TotalTokens
		totalRuns += e.RunCount
		byProvider[e.ProviderType] += e.TotalTokens
	}

	return TenantCostSummary{
		PeriodStart: start,
		PeriodEnd:   end,
		TotalTokens: totalTokens,
		TotalRuns:   totalRuns,
		ByEmployee:  employees,
		ByProvider:  byProvider,
		DailyTrend:  trend,
	}, nil
}

func (s *Service) GetEmployeeCostList(ctx context.Context, tenantID uuid.UUID, period string) (EmployeeCostList, error) {
	start, end, err := periodDates(period)
	if err != nil {
		return EmployeeCostList{}, err
	}
	p := QueryParams{TenantID: tenantID, StartDate: start, EndDate: end}

	rows, err := s.repo.GetEmployeeSummary(ctx, p)
	if err != nil {
		return EmployeeCostList{}, err
	}
	return EmployeeCostList{
		PeriodStart: start,
		PeriodEnd:   end,
		Items:       rows,
	}, nil
}
