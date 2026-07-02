package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
)

type digitalEmployeeReader interface {
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployee, error)
}

type DigitalEmployeeIdentityServiceAdapter struct {
	reader digitalEmployeeReader
}

func NewDigitalEmployeeIdentityAdapter(reader digitalEmployeeReader) DigitalEmployeeIdentityLookup {
	if reader == nil {
		return nil
	}
	return &DigitalEmployeeIdentityServiceAdapter{reader: reader}
}

func (a *DigitalEmployeeIdentityServiceAdapter) GetDigitalEmployeeIdentity(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeIdentity, error) {
	emp, err := a.reader.GetDigitalEmployee(ctx, tenantID, digitalEmployeeID)
	if err != nil {
		return DigitalEmployeeIdentity{}, err
	}

	identity := DigitalEmployeeIdentity{Role: emp.Role}
	if asset := employee.AvatarAssetFromMetadata(emp.Metadata); asset != nil {
		identity.AvatarAsset = &ProjectTaskGraphEmployeeAvatarAsset{
			ID:           asset.ID,
			Label:        asset.Label,
			ImageURL:     asset.ImageURL,
			ThumbnailURL: asset.ThumbnailURL,
		}
	}
	return identity, nil
}
