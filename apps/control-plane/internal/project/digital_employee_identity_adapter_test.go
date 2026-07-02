package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
)

type fakeDigitalEmployeeReader struct {
	employees map[uuid.UUID]*employee.DigitalEmployee
	err       error
}

func (f *fakeDigitalEmployeeReader) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployee, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.employees[employeeID], nil
}

func TestDigitalEmployeeIdentityAdapterReturnsRoleAndAvatar(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	asset := employee.ListDigitalEmployeeAvatarAssets()[0]
	lookup := NewDigitalEmployeeIdentityAdapter(&fakeDigitalEmployeeReader{
		employees: map[uuid.UUID]*employee.DigitalEmployee{
			employeeID: {
				ID:       employeeID,
				TenantID: tenantID,
				Role:     "代码审查员",
				Metadata: map[string]any{"avatar_asset_id": asset.ID},
			},
		},
	})

	identity, err := lookup.GetDigitalEmployeeIdentity(context.Background(), tenantID, employeeID)
	if err != nil {
		t.Fatalf("GetDigitalEmployeeIdentity returned error: %v", err)
	}
	if identity.Role != "代码审查员" {
		t.Fatalf("Role = %q, want %q", identity.Role, "代码审查员")
	}
	if identity.AvatarAsset == nil {
		t.Fatalf("AvatarAsset is nil")
	}
	if identity.AvatarAsset.ID != asset.ID {
		t.Fatalf("AvatarAsset.ID = %q, want %q", identity.AvatarAsset.ID, asset.ID)
	}
	if identity.AvatarAsset.ThumbnailURL != asset.ThumbnailURL {
		t.Fatalf("AvatarAsset.ThumbnailURL = %q, want %q", identity.AvatarAsset.ThumbnailURL, asset.ThumbnailURL)
	}
}

func TestDigitalEmployeeIdentityAdapterPropagatesReaderError(t *testing.T) {
	wantErr := errors.New("boom")
	lookup := NewDigitalEmployeeIdentityAdapter(&fakeDigitalEmployeeReader{err: wantErr})

	_, err := lookup.GetDigitalEmployeeIdentity(context.Background(), uuid.New(), uuid.New())
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

func TestNewDigitalEmployeeIdentityAdapterReturnsNilForNilReader(t *testing.T) {
	if lookup := NewDigitalEmployeeIdentityAdapter(nil); lookup != nil {
		t.Fatalf("NewDigitalEmployeeIdentityAdapter(nil) = %#v, want nil", lookup)
	}
}
