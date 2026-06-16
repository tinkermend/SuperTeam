package avatar

import "testing"

func TestBuiltInAvatarAssetLookupNormalizesID(t *testing.T) {
	asset, ok := BuiltInAssetByID(" ENGINEER-F-01 ")
	if !ok {
		t.Fatal("expected engineer-f-01 to be found")
	}
	if asset.ID != "engineer-f-01" || asset.ThumbnailURL == "" {
		t.Fatalf("unexpected asset: %#v", asset)
	}
}

func TestBuiltInAvatarAssetsAreReturnedAsCopy(t *testing.T) {
	assets := ListBuiltInAssets()
	if len(assets) != 20 {
		t.Fatalf("expected 20 built-in assets, got %d", len(assets))
	}
	assets[0].ID = "mutated"
	fresh := ListBuiltInAssets()
	if fresh[0].ID == "mutated" {
		t.Fatal("ListBuiltInAssets must return a copy")
	}
}
