package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/superteam/control-plane/internal/workflow/projectcoordination"
)

const (
	autoRecheckRecoveryInterval   = 30 * time.Second
	autoRecheckRecoveryBatchLimit = 50
)

func startAutoRecheckRecoveryReconciler(ctx context.Context, store *projectcoordination.ProjectStore) {
	run := func() {
		n, err := store.SweepAutoRecheckRecoveryGates(ctx, autoRecheckRecoveryBatchLimit)
		if err != nil && ctx.Err() == nil {
			slog.Warn("auto_recheck recovery reconciler failed", "error", err)
			return
		}
		if n > 0 {
			slog.Info("auto_recheck recovery reconciler: released gates", "count", n)
		}
	}
	run()
	ticker := time.NewTicker(autoRecheckRecoveryInterval)
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
