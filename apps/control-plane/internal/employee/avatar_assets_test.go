package employee

import "testing"

func TestAvatarAssetFromMetadataReadsTopLevelAvatarAssetID(t *testing.T) {
	asset := AvatarAssetFromMetadata(map[string]any{
		"avatar_asset_id": "engineer-m-01",
	})

	if asset == nil {
		t.Fatal("expected avatar asset")
	}
	if asset.ID != "engineer-m-01" {
		t.Fatalf("expected engineer-m-01, got %q", asset.ID)
	}
}

func TestAvatarAssetFromMetadataReadsNestedAvatarID(t *testing.T) {
	asset := AvatarAssetFromMetadata(map[string]any{
		"avatar": map[string]any{
			"id": "engineer-f-01",
		},
	})

	if asset == nil {
		t.Fatal("expected avatar asset")
	}
	if asset.ID != "engineer-f-01" {
		t.Fatalf("expected engineer-f-01, got %q", asset.ID)
	}
}

func TestAvatarAssetFromMetadataReturnsNilForNilOrEmptyMetadata(t *testing.T) {
	if asset := AvatarAssetFromMetadata(nil); asset != nil {
		t.Fatalf("expected nil for nil metadata, got %#v", asset)
	}
	if asset := AvatarAssetFromMetadata(map[string]any{}); asset != nil {
		t.Fatalf("expected nil for empty metadata, got %#v", asset)
	}
}
