package projectcoordination

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
)

func TestIsProjectTaskDispatchRetryLater(t *testing.T) {
	require.False(t, isProjectTaskDispatchRetryLater(nil))
	require.True(t, isProjectTaskDispatchRetryLater(ErrProjectTaskDispatchRetryLater))
	require.True(t, isProjectTaskDispatchRetryLater(errors.Join(ErrProjectTaskDispatchRetryLater, errors.New("wrap"))))
	require.False(t, isProjectTaskDispatchRetryLater(errors.New("other")))
	// Temporal application error carrying the sentinel message
	app := temporal.NewApplicationError(ErrProjectTaskDispatchRetryLater.Error(), "SomeType", ErrProjectTaskDispatchRetryLater)
	require.True(t, isProjectTaskDispatchRetryLater(app))
}
