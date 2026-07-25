package automation

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// RegisterWith adds the automation workflow and activities to an existing worker.
//
// Automation shares the coordination task queue, so it must share the
// coordination worker too. Two workers polling one queue with disjoint
// registrations means the server hands each task to whichever worker polled
// first, so roughly half of them land on a worker that does not know the type
// ("unable to find workflow type: ProjectCoordinatorWorkflow" / "unable to find
// activityType=AppendProjectEvent"). Those tasks fail and retry, inflating
// workflow history — which in turn drags the coordinator toward continue-as-new
// sooner — and once an activity exhausts its retry budget it fails real
// coordination work.
func RegisterWith(r worker.Registry, activities *Activities) {
	r.RegisterWorkflow(AutomationFireWorkflow)
	r.RegisterActivity(activities)
}

// NewWorker builds a standalone automation worker. Only use it for a task queue
// no other worker polls; when automation shares the coordination queue, register
// onto that worker with RegisterWith instead.
func NewWorker(c client.Client, taskQueue string, activities *Activities) worker.Worker {
	w := worker.New(c, taskQueue, worker.Options{})
	RegisterWith(w, activities)
	return w
}
