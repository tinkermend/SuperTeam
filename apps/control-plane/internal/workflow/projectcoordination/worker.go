package projectcoordination

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

// RegisterWith adds the coordinator workflow and its activities to an existing
// worker. Anything else sharing this task queue must register onto the same
// worker rather than starting its own — see automation.RegisterWith.
func RegisterWith(r worker.Registry, activities *Activities) {
	r.RegisterWorkflow(ProjectCoordinatorWorkflow)
	r.RegisterActivity(activities)
}

func NewWorker(c client.Client, taskQueue string, activities *Activities) worker.Worker {
	w := worker.New(c, taskQueue, worker.Options{})
	RegisterWith(w, activities)
	return w
}
