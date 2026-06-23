package handlers

import (
	"context"
	"errors"

	"github.com/superteam/control-plane/internal/employee"
)

type RuntimeCommandWritebackRouter struct {
	primary  RuntimeCommandWritebackService
	fallback RuntimeCommandWritebackService
}

func NewRuntimeCommandWritebackRouter(primary, fallback RuntimeCommandWritebackService) RuntimeCommandWritebackService {
	if fallback == nil {
		return primary
	}
	if primary == nil {
		return fallback
	}
	return &RuntimeCommandWritebackRouter{primary: primary, fallback: fallback}
}

func (r *RuntimeCommandWritebackRouter) RecordEvent(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, event employee.RuntimeCommandEventWriteback) error {
	return r.call(func(service RuntimeCommandWritebackService) error {
		return service.RecordEvent(ctx, identity, commandID, event)
	})
}

func (r *RuntimeCommandWritebackRouter) Complete(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	return r.call(func(service RuntimeCommandWritebackService) error {
		return service.Complete(ctx, identity, commandID, terminal)
	})
}

func (r *RuntimeCommandWritebackRouter) Fail(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	return r.call(func(service RuntimeCommandWritebackService) error {
		return service.Fail(ctx, identity, commandID, terminal)
	})
}

func (r *RuntimeCommandWritebackRouter) Cancel(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	return r.call(func(service RuntimeCommandWritebackService) error {
		return service.Cancel(ctx, identity, commandID, terminal)
	})
}

func (r *RuntimeCommandWritebackRouter) TimedOut(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	return r.call(func(service RuntimeCommandWritebackService) error {
		return service.TimedOut(ctx, identity, commandID, terminal)
	})
}

func (r *RuntimeCommandWritebackRouter) call(call func(RuntimeCommandWritebackService) error) error {
	if r == nil {
		return employee.ErrInvalidInput
	}
	if r.primary != nil {
		err := call(r.primary)
		if err == nil || !errors.Is(err, employee.ErrNotFound) || r.fallback == nil {
			return err
		}
	}
	if r.fallback == nil {
		return employee.ErrNotFound
	}
	return call(r.fallback)
}
