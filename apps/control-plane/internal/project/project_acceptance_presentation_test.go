package project

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

func TestBuildProjectAcceptancePresentationNeverNarratesCancelledDemandAsCompleted(t *testing.T) {
	projectID := uuid.New()
	cancelled := ProjectAcceptanceDemandInput{
		ID:        uuid.New(),
		Title:     "also_close E2E：输出一行中文问候",
		Status:    "cancelled",
		UpdatedAt: time.Now(),
	}
	completed := ProjectAcceptanceDemandInput{
		ID:         uuid.New(),
		Title:      "分析 CPU 使用率",
		Status:     "completed",
		UpdatedAt:  time.Now().Add(-time.Hour),
		TaskTitles: []string{"分析 CPU 使用率及高占用进程"},
	}
	got := BuildProjectAcceptancePresentation("混合项目", projectID, []ProjectAcceptanceDemandInput{cancelled, completed})

	// 已完成主句只覆盖 completed;取消项单列且带状态。
	require.Contains(t, got.Summary, "「分析 CPU 使用率」（含任务：分析 CPU 使用率及高占用进程）已完成")
	require.Contains(t, got.Summary, "「also_close E2E：输出一行中文问候」已取消")
	require.Contains(t, got.Summary, "未计入本次结项")
	require.NotContains(t, got.Summary, "「also_close E2E：输出一行中文问候」已完成")
	// primary 必须指向真正完成的需求,而非最新的取消需求。
	require.Equal(t, completed.ID, got.PrimaryDemandID)

	demands, ok := got.Context["demands"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, demands, 2)
	labels := map[string]string{}
	for _, entry := range demands {
		labels[entry["title"].(string)] = entry["status_label"].(string)
	}
	require.Equal(t, "已取消", labels["also_close E2E：输出一行中文问候"])
	require.Equal(t, "已完成", labels["分析 CPU 使用率"])
}

func TestBuildProjectAcceptancePresentationKeepsUnfinishedClauseUnderTruncation(t *testing.T) {
	inputs := make([]ProjectAcceptanceDemandInput, 0, 11)
	for i := 0; i < 10; i++ {
		inputs = append(inputs, ProjectAcceptanceDemandInput{
			ID:         uuid.New(),
			Title:      fmt.Sprintf("需求 %d：分析 CPU 使用率情况", i),
			Status:     "completed",
			UpdatedAt:  time.Now().Add(-time.Duration(i) * time.Minute),
			TaskTitles: []string{"分析 CPU 使用率及高占用进程", "输出结论报告"},
		})
	}
	cancelled := ProjectAcceptanceDemandInput{
		ID:        uuid.New(),
		Title:     "also_close E2E：输出一行中文问候",
		Status:    "cancelled",
		UpdatedAt: time.Now().Add(-time.Hour),
	}
	got := BuildProjectAcceptancePresentation("大项目", uuid.New(), append(inputs, cancelled))

	// 摘要超长时截断只能吃已完成枚举;取消项披露若被截掉,摘要又退回"全部已完成"。
	require.LessOrEqual(t, utf8.RuneCountInString(got.Summary), projectAcceptanceSummaryMaxRunes)
	require.Contains(t, got.Summary, "「also_close E2E：输出一行中文问候」已取消，未计入本次结项")
	require.True(t, strings.HasSuffix(got.Summary, "，请确认结项并归档"))
}

func TestBuildProjectAcceptancePresentationAllDemandsUnfinished(t *testing.T) {
	got := BuildProjectAcceptancePresentation("全取消项目", uuid.New(), []ProjectAcceptanceDemandInput{
		{ID: uuid.New(), Title: "失败需求", Status: "failed", UpdatedAt: time.Now()},
	})
	require.Contains(t, got.Summary, "无已完成需求")
	require.Contains(t, got.Summary, "「失败需求」失败")
	require.NotContains(t, got.Summary, "已完成，")
}

func TestBuildProjectAcceptancePresentationEmptyDemandsFallsBackToProject(t *testing.T) {
	got := BuildProjectAcceptancePresentation("空项目", uuid.New(), nil)
	require.Equal(t, "结项确认 · 空项目", got.Title)
	require.Contains(t, got.Summary, "空项目")
	require.NotContains(t, got.Title, "验收项目交付")
}
