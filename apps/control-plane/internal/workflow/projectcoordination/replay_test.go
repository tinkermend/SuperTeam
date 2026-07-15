package projectcoordination

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/worker"
)

// TestReplayRealCoordinatorHistory replays the real exported history of
// project-coordinator:9bb61e95 (temporal workflow show --output json, dev env,
// 2026-07-15). This single history spans every terminal-planning era the
// coordinator has shipped, so it pins replay compatibility for all of them:
//
//   - event 23: UNTYPED planner failure (pre-a9a4b8a9 "wrapError") followed by
//     the survive-gate signal_failed path (marker at 27). Current code must keep
//     taking the old path here — guaranteed by noSuitableEmployeeDiagnosis's
//     error-type check, NOT by a version fence.
//   - events 53/135: TYPED NoSuitableEmployee failures followed by
//     RejectDemandPlanning (57/139) recorded by the UNFENCED a9a4b8a9/a310259a
//     code, with no GetVersion marker. Current code must unconditionally take
//     the reject branch here; a retroactive GetVersion fence returns
//     DefaultVersion at these positions (no marker in history) and diverges —
//     that exact divergence killed project-coordinator:b4226c24
//     (WorkflowExecutionFailed surfacing a stale planning error) on 07-15.
//
// If this test fails after a workflow change, the change breaks replay of
// in-flight coordinators and must be redesigned (fences only work when they
// ship in the same deploy as the behavior change — never after).
func TestReplayRealCoordinatorHistory(t *testing.T) {
	replayer := worker.NewWorkflowReplayer()
	replayer.RegisterWorkflow(ProjectCoordinatorWorkflow)
	err := replayer.ReplayWorkflowHistoryFromJSONFile(nil, "testdata/history-project-coordinator-9bb61e95.json")
	require.NoError(t, err)
}
