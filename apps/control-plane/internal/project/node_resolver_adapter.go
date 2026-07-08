package project

import (
	"context"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/employee"
)

// ProjectTaskNodeResolverAdapter implements employee.ProjectTaskNodeResolver by
// delegating to Service.ResolveProjectTaskNode and translating its Paused/Reason
// outcomes into this package's dispatch sentinel errors
// (ErrProjectTaskPinnedNodeOffline / ErrProjectTaskNoEligibleOnlineNode). The
// employee package must not import project, so it only ever sees an opaque
// error it can bubble up; composition-root code that imports both packages
// (internal/app) classifies these with errors.Is.
type ProjectTaskNodeResolverAdapter struct {
	service *Service
}

// NewProjectTaskNodeResolverAdapter wires a project Service into the
// employee.ProjectTaskNodeResolver interface for injection into
// DigitalEmployeeRunService.
func NewProjectTaskNodeResolverAdapter(service *Service) employee.ProjectTaskNodeResolver {
	if service == nil {
		return nil
	}
	return ProjectTaskNodeResolverAdapter{service: service}
}

func (a ProjectTaskNodeResolverAdapter) ResolveProjectTaskNode(ctx context.Context, req employee.ResolveProjectTaskNodeRequest) (uuid.UUID, error) {
	resolution, err := a.service.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeInput{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		ProjectTaskID:     req.ProjectTaskID,
		// RequiredProvider intentionally left empty: the dispatch preflight's
		// provider_healthy check is the downstream safety net for provider
		// mismatch, so the live dispatch path does not thread provider_type
		// into node selection (YAGNI; avoids a new dependency chain).
	})
	if err != nil {
		return uuid.Nil, err
	}
	switch resolution.Reason {
	case NodeResolutionReasonPinnedNodeOffline:
		return uuid.Nil, ErrProjectTaskPinnedNodeOffline
	case NodeResolutionReasonNoEligibleOnlineNode:
		return uuid.Nil, ErrProjectTaskNoEligibleOnlineNode
	case "":
		if resolution.NodeID == uuid.Nil {
			// Defensive: the resolver contract guarantees NodeID != Nil when
			// Reason == "" and Paused == false; treat an unexpected empty
			// result the same as "no eligible node" rather than dispatching
			// to a nil node.
			return uuid.Nil, ErrProjectTaskNoEligibleOnlineNode
		}
		return resolution.NodeID, nil
	default:
		return uuid.Nil, ErrProjectTaskNoEligibleOnlineNode
	}
}
