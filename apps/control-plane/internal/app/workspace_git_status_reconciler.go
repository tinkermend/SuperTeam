package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/superteam/control-plane/internal/platform"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/systemconfig"
)

const (
	workspaceGitReconcileInterval = time.Minute
	workspaceGitSampleBatchLimit  = 50
)

func startWorkspaceGitStatusReconciler(ctx context.Context, projectService *project.Service, config systemconfig.Reader) {
	run := func() {
		now := time.Now().UTC()
		interval := time.Duration(0)
		if config != nil {
			interval = config.Duration(ctx, platform.DefaultTenantID, systemconfig.KeyProjectWorkspaceGitSampleIntervalSeconds)
		}
		if interval <= 0 {
			interval = systemconfig.DefaultDurationFor(systemconfig.KeyProjectWorkspaceGitSampleIntervalSeconds)
		}
		_ = interval
		if _, err := projectService.SweepWorkspaceGitSamples(ctx, now, workspaceGitSampleBatchLimit); err != nil && ctx.Err() == nil {
			slog.Warn("workspace git status reconciler: sample sweep failed", "error", err)
		}
	}
	run()
	ticker := time.NewTicker(workspaceGitReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
