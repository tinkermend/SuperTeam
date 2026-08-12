package project

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestListProjectDemandsForConsoleOrdersByUpdatedAtThenNonTerminal(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	repo := &memoryRepository{demands: []ProjectDemand{
		{
			ID:        uuid.MustParse("00000000-0000-0000-0000-000000000003"),
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "仍在执行但更早",
			Status:    ProjectDemandStatusExecuting,
			UpdatedAt: now.Add(-2 * time.Hour),
			CreatedAt: now.Add(-3 * time.Hour),
		},
		{
			ID:        uuid.MustParse("00000000-0000-0000-0000-000000000002"),
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "刚关闭",
			Status:    ProjectDemandStatusCompleted,
			UpdatedAt: now.Add(-12 * time.Minute),
			CreatedAt: now.Add(-4 * time.Hour),
		},
		{
			ID:        uuid.MustParse("00000000-0000-0000-0000-000000000001"),
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "正在执行",
			Status:    ProjectDemandStatusExecuting,
			UpdatedAt: now.Add(-3 * time.Minute),
			CreatedAt: now.Add(-1 * time.Hour),
		},
	}}
	service := &Service{repository: repo}

	got, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Title != "正在执行" {
		t.Fatalf("want 最近更新的执行中 first, got %q", got[0].Title)
	}
	if got[1].Title != "刚关闭" {
		t.Fatalf("want 更近的已完成 before 更早的执行中, got %q", got[1].Title)
	}
	if got[2].Title != "仍在执行但更早" {
		t.Fatalf("got %q", got[2].Title)
	}
}
