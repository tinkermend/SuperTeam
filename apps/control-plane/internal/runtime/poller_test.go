package runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/superteam/control-plane/internal/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoller_WaitForTask_Timeout(t *testing.T) {
	poller := NewPoller()
	defer poller.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := poller.WaitForTask(ctx, "node-1")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestPoller_WaitForTask_Notify(t *testing.T) {
	poller := NewPoller()
	defer poller.Close()

	testTask := &task.Task{
		ID:           1,
		Title:        "Test Task",
		ProviderType: "claude-code",
		Status:       task.TaskStatusPending,
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var result *task.Task
	var err error

	// Start waiting in a goroutine
	go func() {
		defer wg.Done()
		ctx := context.Background()
		result, err = poller.WaitForTask(ctx, "node-1")
	}()

	// Give the waiter time to register
	time.Sleep(50 * time.Millisecond)

	// Notify the task
	poller.NotifyTask("node-1", testTask)

	// Wait for completion
	wg.Wait()

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, testTask.ID, result.ID)
	assert.Equal(t, testTask.Title, result.Title)
}

func TestPoller_NotifyTask_NoWaiter(t *testing.T) {
	poller := NewPoller()
	defer poller.Close()

	testTask := &task.Task{
		ID:           1,
		Title:        "Test Task",
		ProviderType: "claude-code",
		Status:       task.TaskStatusPending,
	}

	// Should not panic when no waiter exists
	assert.NotPanics(t, func() {
		poller.NotifyTask("node-1", testTask)
	})
}

func TestPoller_Concurrent(t *testing.T) {
	poller := NewPoller()
	defer poller.Close()

	const numNodes = 10
	var wg sync.WaitGroup
	wg.Add(numNodes)

	results := make([]*task.Task, numNodes)
	errors := make([]error, numNodes)

	// Start multiple waiters
	for i := 0; i < numNodes; i++ {
		go func(idx int) {
			defer wg.Done()
			ctx := context.Background()
			nodeID := "node-" + string(rune('0'+idx))
			results[idx], errors[idx] = poller.WaitForTask(ctx, nodeID)
		}(i)
	}

	// Give waiters time to register
	time.Sleep(50 * time.Millisecond)

	// Verify all waiters are registered
	assert.Equal(t, numNodes, poller.ActiveWaiters())

	// Notify each node
	for i := 0; i < numNodes; i++ {
		nodeID := "node-" + string(rune('0'+i))
		testTask := &task.Task{
			ID:           int64(i + 1),
			Title:        "Task " + string(rune('0'+i)),
			ProviderType: "claude-code",
			Status:       task.TaskStatusPending,
		}
		poller.NotifyTask(nodeID, testTask)
	}

	// Wait for all to complete
	wg.Wait()

	// Verify results
	for i := 0; i < numNodes; i++ {
		require.NoError(t, errors[i], "node %d should not have error", i)
		require.NotNil(t, results[i], "node %d should have result", i)
		assert.Equal(t, int64(i+1), results[i].ID)
	}

	// All waiters should be cleaned up
	assert.Equal(t, 0, poller.ActiveWaiters())
}

func TestPoller_Close(t *testing.T) {
	poller := NewPoller()

	var wg sync.WaitGroup
	wg.Add(1)

	var result *task.Task
	var err error

	// Start waiting
	go func() {
		defer wg.Done()
		ctx := context.Background()
		result, err = poller.WaitForTask(ctx, "node-1")
	}()

	// Give waiter time to register
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, poller.ActiveWaiters())

	// Close the poller
	poller.Close()

	// Wait for completion
	wg.Wait()

	// Should return error when closed
	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPollerClosed)

	// All waiters should be cleaned up
	assert.Equal(t, 0, poller.ActiveWaiters())
}

func TestPoller_WaitAfterClose(t *testing.T) {
	poller := NewPoller()
	poller.Close()

	ctx := context.Background()
	result, err := poller.WaitForTask(ctx, "node-1")

	assert.Nil(t, result)
	assert.ErrorIs(t, err, ErrPollerClosed)
}

func TestPoller_NotifyAfterClose(t *testing.T) {
	poller := NewPoller()
	poller.Close()

	testTask := &task.Task{
		ID:           1,
		Title:        "Test Task",
		ProviderType: "claude-code",
		Status:       task.TaskStatusPending,
	}

	// Should not panic
	assert.NotPanics(t, func() {
		poller.NotifyTask("node-1", testTask)
	})
}

func TestPoller_MultipleNotifications(t *testing.T) {
	poller := NewPoller()
	defer poller.Close()

	testTask1 := &task.Task{
		ID:           1,
		Title:        "Task 1",
		ProviderType: "claude-code",
		Status:       task.TaskStatusPending,
	}

	testTask2 := &task.Task{
		ID:           2,
		Title:        "Task 2",
		ProviderType: "claude-code",
		Status:       task.TaskStatusPending,
	}

	var wg sync.WaitGroup
	wg.Add(1)

	var result *task.Task
	var err error

	// Start waiting
	go func() {
		defer wg.Done()
		ctx := context.Background()
		result, err = poller.WaitForTask(ctx, "node-1")
	}()

	// Give waiter time to register
	time.Sleep(50 * time.Millisecond)

	// Send multiple notifications (only first should be received)
	poller.NotifyTask("node-1", testTask1)
	poller.NotifyTask("node-1", testTask2)

	// Wait for completion
	wg.Wait()

	require.NoError(t, err)
	require.NotNil(t, result)
	// Should receive the first task
	assert.Equal(t, testTask1.ID, result.ID)
}

func TestPoller_ContextCancellation(t *testing.T) {
	poller := NewPoller()
	defer poller.Close()

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)

	var result *task.Task
	var err error

	// Start waiting
	go func() {
		defer wg.Done()
		result, err = poller.WaitForTask(ctx, "node-1")
	}()

	// Give waiter time to register
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, poller.ActiveWaiters())

	// Cancel the context
	cancel()

	// Wait for completion
	wg.Wait()

	assert.Nil(t, result)
	assert.ErrorIs(t, err, context.Canceled)

	// Waiter should be cleaned up
	assert.Equal(t, 0, poller.ActiveWaiters())
}
