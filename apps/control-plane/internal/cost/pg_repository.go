package cost

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const tokenExtractSQL = `
	CASE
		WHEN COALESCE(
			tr.result #>> '{usage,total_tokens}',
			tr.result ->> 'total_tokens',
			''
		) ~ '^[0-9]+$'
		THEN COALESCE(
			tr.result #>> '{usage,total_tokens}',
			tr.result ->> 'total_tokens'
		)::bigint
		ELSE 0
	END`

const employeeSummarySQL = `
SELECT
	tr.digital_employee_id,
	COALESCE(de.name, '') AS employee_name,
	COALESCE(tr.provider_type, 'unknown') AS provider_type,
	COUNT(*)::int AS run_count,
	COALESCE(SUM(` + tokenExtractSQL + `), 0)::bigint AS total_tokens
FROM task_runs tr
LEFT JOIN digital_employees de
	ON de.id = tr.digital_employee_id AND de.tenant_id = tr.tenant_id
WHERE tr.tenant_id = $1
  AND tr.finished_at >= $2
  AND tr.finished_at < $3
  AND tr.status IN ('completed', 'finished')
  AND tr.digital_employee_id IS NOT NULL
GROUP BY tr.digital_employee_id, de.name, tr.provider_type
ORDER BY total_tokens DESC`

const dailyTrendSQL = `
SELECT
	DATE_TRUNC('day', tr.finished_at AT TIME ZONE 'Asia/Shanghai') AS day,
	COALESCE(tr.provider_type, 'unknown') AS provider_type,
	COUNT(*)::int AS run_count,
	COALESCE(SUM(` + tokenExtractSQL + `), 0)::bigint AS total_tokens
FROM task_runs tr
WHERE tr.tenant_id = $1
  AND tr.finished_at >= $2
  AND tr.finished_at < $3
  AND tr.status IN ('completed', 'finished')
GROUP BY day, tr.provider_type
ORDER BY day ASC`

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) Repository {
	return &PgRepository{db: db}
}

func (r *PgRepository) GetEmployeeSummary(ctx context.Context, p QueryParams) ([]EmployeeSummaryRow, error) {
	rows, err := r.db.Query(ctx, employeeSummarySQL, p.TenantID, p.StartDate, p.EndDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EmployeeSummaryRow
	for rows.Next() {
		var (
			id       uuid.UUID
			name     string
			provider string
			runCount int32
			tokens   int64
		)
		if err := rows.Scan(&id, &name, &provider, &runCount, &tokens); err != nil {
			return nil, err
		}
		result = append(result, EmployeeSummaryRow{
			DigitalEmployeeID: id,
			EmployeeName:      name,
			ProviderType:      provider,
			RunCount:          runCount,
			TotalTokens:       tokens,
		})
	}
	return result, rows.Err()
}

func (r *PgRepository) GetDailyTrend(ctx context.Context, p QueryParams) ([]DailyTrendRow, error) {
	rows, err := r.db.Query(ctx, dailyTrendSQL, p.TenantID, p.StartDate, p.EndDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DailyTrendRow
	for rows.Next() {
		var (
			day      time.Time
			provider string
			runCount int32
			tokens   int64
		)
		if err := rows.Scan(&day, &provider, &runCount, &tokens); err != nil {
			return nil, err
		}
		result = append(result, DailyTrendRow{
			Day:          day,
			ProviderType: provider,
			RunCount:     runCount,
			TotalTokens:  tokens,
		})
	}
	return result, rows.Err()
}
