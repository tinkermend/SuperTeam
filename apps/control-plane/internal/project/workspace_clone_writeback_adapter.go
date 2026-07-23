package project

import (
	"context"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/employee"
)

// ProjectWorkspaceCloneWritebackAdapter bridges clone command writeback into
// project readiness transitions.
type ProjectWorkspaceCloneWritebackAdapter struct {
	service *Service
}

func NewProjectWorkspaceCloneWritebackAdapter(service *Service) employee.ProjectWorkspaceCommandHook {
	if service == nil {
		return nil
	}
	return ProjectWorkspaceCloneWritebackAdapter{service: service}
}

func (a ProjectWorkspaceCloneWritebackAdapter) OnProjectWorkspaceCommandTerminal(ctx context.Context, receipt employee.RuntimeCommandReceipt, success bool) error {
	errMsg := ""
	if receipt.ErrorMessage != nil {
		errMsg = *receipt.ErrorMessage
	}
	return a.service.HandleCloneCommandTerminal(ctx, CloneCommandTerminal{
		TenantID:      receipt.TenantID,
		ProjectID:     receipt.ResourceID,
		RuntimeNodeID: receipt.RuntimeNodeID,
		CommandID:     receipt.CommandID,
		Success:       success,
		ErrorMessage:  errMsg,
		Payload:       receipt.Payload,
	})
}

type employeeProjectWorkspaceReceiptStore interface {
	ListCommandReceiptsByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID, commandType string, limit int32) ([]employee.RuntimeCommandReceipt, error)
}

func NewProjectWorkspaceReceiptListerAdapter(store employeeProjectWorkspaceReceiptStore) ProjectWorkspaceReceiptLister {
	if store == nil {
		return nil
	}
	return projectWorkspaceReceiptListerAdapter{store: store}
}

type projectWorkspaceReceiptListerAdapter struct {
	store employeeProjectWorkspaceReceiptStore
}

func (a projectWorkspaceReceiptListerAdapter) ListProjectWorkspaceReceipts(ctx context.Context, tenantID, projectID uuid.UUID, commandType string, limit int32) ([]WorkspaceCommandReceiptSummary, error) {
	receipts, err := a.store.ListCommandReceiptsByResource(ctx, tenantID, projectWorkspaceResourceType, projectID, commandType, limit)
	if err != nil {
		return nil, err
	}
	out := make([]WorkspaceCommandReceiptSummary, 0, len(receipts))
	for _, receipt := range receipts {
		out = append(out, WorkspaceCommandReceiptSummary{
			CommandID:     receipt.CommandID,
			CommandType:   receipt.CommandType,
			RuntimeNodeID: receipt.RuntimeNodeID,
			Status:        receipt.Status,
			Payload:       receipt.Payload,
			ErrorMessage:  receipt.ErrorMessage,
		})
	}
	return out, nil
}
