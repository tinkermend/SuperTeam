package project

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceGitStatusFromProbeResultNonGitNotClean(t *testing.T) {
	status, err := workspaceGitStatusFromProbeResult(map[string]any{
		"exists":      true,
		"is_dir":      true,
		"is_git_repo": false,
	}, time.Now().UTC(), uuid.New(), "node-a")
	require.NoError(t, err)
	require.False(t, status.Applicable)
	require.NotNil(t, status.IsGitRepo)
	require.False(t, *status.IsGitRepo)
	require.Nil(t, status.IsClean)
}

func TestWorkspaceGitStatusFromProbeResultDirtyEntries(t *testing.T) {
	status, err := workspaceGitStatusFromProbeResult(map[string]any{
		"exists":      true,
		"is_dir":      true,
		"is_git_repo": true,
		"dirty":       true,
		"head_commit": "abc123",
		"repo_state":  "ok",
		"uncommitted_count": 2,
		"uncommitted_entries": []any{
			map[string]any{"path": "a.txt", "category": "modified"},
			map[string]any{"path": "b.txt", "category": "untracked"},
		},
	}, time.Now().UTC(), uuid.New(), "node-a")
	require.NoError(t, err)
	require.True(t, status.Applicable)
	require.NotNil(t, status.IsClean)
	require.False(t, *status.IsClean)
	require.Equal(t, "abc123", status.HeadCommit)
	require.Equal(t, ProjectWorkspaceGitRepoStateOK, status.RepoState)
	require.Equal(t, 2, status.UncommittedCount)
	require.Len(t, status.UncommittedEntries, 2)
}

func TestWorkspaceGitStatusFromProbeResultMissingDirFails(t *testing.T) {
	_, err := workspaceGitStatusFromProbeResult(map[string]any{
		"exists": false,
	}, time.Now().UTC(), uuid.New(), "node-a")
	require.Error(t, err)
}

func TestLeanWorkspaceGitStatusDropsEntries(t *testing.T) {
	status := &ProjectWorkspaceGitStatus{
		Applicable: true,
		UncommittedEntries: []ProjectWorkspaceGitFileEntry{
			{Path: "a.txt", Category: ProjectWorkspaceGitFileModified},
		},
		UncommittedCount: 1,
	}
	lean := leanWorkspaceGitStatus(status)
	require.NotNil(t, lean)
	require.Nil(t, lean.UncommittedEntries)
	require.Equal(t, 1, lean.UncommittedCount)
	require.Len(t, status.UncommittedEntries, 1)
}

func TestProjectTaskStatusEligibleForWorkspaceGitSampleIncludesCancelled(t *testing.T) {
	require.True(t, projectTaskStatusEligibleForWorkspaceGitSample(ProjectTaskStatusCompleted))
	require.True(t, projectTaskStatusEligibleForWorkspaceGitSample(ProjectTaskStatusFailed))
	require.True(t, projectTaskStatusEligibleForWorkspaceGitSample(ProjectTaskStatusCancelled))
	require.False(t, projectTaskStatusEligibleForWorkspaceGitSample(ProjectTaskStatusBlocked))
	require.False(t, projectTaskStatusEligibleForWorkspaceGitSample(ProjectTaskStatusRunning))
}
