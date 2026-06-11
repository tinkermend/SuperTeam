package projectcoordination

import (
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func NewWorker(c client.Client, taskQueue string, activities *Activities) worker.Worker {
	w := worker.New(c, taskQueue, worker.Options{})
	w.RegisterWorkflow(ProjectCoordinatorWorkflow)
	w.RegisterActivity(activities)
	return w
}
