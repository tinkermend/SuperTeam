package project

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBuildProjectAcceptancePresentationSingleDemand(t *testing.T) {
	projectID := uuid.New()
	demandID := uuid.New()
	got := BuildProjectAcceptancePresentation("测试项目", projectID, []ProjectAcceptanceDemandInput{
		{
			ID:         demandID,
			Title:      "帮我分析一下当前服务器中的 claude code 配置是否合理",
			Status:     "completed",
			UpdatedAt:  time.Now(),
			TaskTitles: []string{"分析服务器中的Claude Code配置合理性"},
		},
	})

	require.Equal(t, "结项确认 · 测试项目", got.Title)
	require.NotContains(t, got.Title, "验收项目交付")
	require.NotContains(t, got.Title, "帮我分析")
	require.Contains(t, got.Summary, "测试项目")
	require.Contains(t, got.Summary, "帮我分析一下当前服务器中的 claude code 配置是否合理")
	require.Contains(t, got.Summary, "分析服务器中的Claude Code配置合理性")
	require.Equal(t, demandID, got.PrimaryDemandID)
	require.Equal(t, demandID.String(), got.Context["primary_demand_id"])
	demands, ok := got.Context["demands"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, demands, 1)
	require.Equal(t, []string{"分析服务器中的Claude Code配置合理性"}, demands[0]["task_titles"])
}

func TestBuildProjectAcceptancePresentationMultipleDemandsLeadsWithNewest(t *testing.T) {
	projectID := uuid.New()
	older := ProjectAcceptanceDemandInput{
		ID:        uuid.New(),
		Title:     "旧需求",
		Status:    "completed",
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	newer := ProjectAcceptanceDemandInput{
		ID:        uuid.New(),
		Title:     "新完成的需求",
		Status:    "completed",
		UpdatedAt: time.Now(),
	}
	got := BuildProjectAcceptancePresentation("多需求项目", projectID, []ProjectAcceptanceDemandInput{older, newer})

	require.Equal(t, "结项确认 · 多需求项目", got.Title)
	require.Equal(t, newer.ID, got.PrimaryDemandID)
	require.Contains(t, got.Summary, "旧需求")
	require.Contains(t, got.Summary, "新完成的需求")
	require.NotContains(t, got.Title, "验收项目交付")
}

func TestBuildProjectAcceptancePresentationEmptyDemandsFallsBackToProject(t *testing.T) {
	got := BuildProjectAcceptancePresentation("空项目", uuid.New(), nil)
	require.Equal(t, "结项确认 · 空项目", got.Title)
	require.Contains(t, got.Summary, "空项目")
	require.NotContains(t, got.Title, "验收项目交付")
}
