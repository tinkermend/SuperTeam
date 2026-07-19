package employee

import (
	"strings"

	"github.com/superteam/control-plane/internal/avatar"
)

type DigitalEmployeeAvatarAsset struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Gender       string `json:"gender"`
	AgeRange     string `json:"age_range"`
	Style        string `json:"style"`
	ImageURL     string `json:"image_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	Source       string `json:"source"`
	License      string `json:"license"`
	Status       string `json:"status"`
	InUse        bool   `json:"in_use"`
}

func ListDigitalEmployeeAvatarAssets() []DigitalEmployeeAvatarAsset {
	sharedAssets := avatar.ListBuiltInAssets()
	assets := make([]DigitalEmployeeAvatarAsset, 0, len(sharedAssets))
	for _, asset := range sharedAssets {
		assets = append(assets, digitalEmployeeAvatarAssetFromShared(asset))
	}
	return assets
}

func DigitalEmployeeAvatarAssetByID(id string) (DigitalEmployeeAvatarAsset, bool) {
	asset, ok := avatar.BuiltInAssetByID(id)
	if !ok {
		return DigitalEmployeeAvatarAsset{}, false
	}
	return digitalEmployeeAvatarAssetFromShared(asset), true
}

func AvatarAssetFromMetadata(metadata map[string]any) *DigitalEmployeeAvatarAsset {
	if metadata == nil {
		return nil
	}
	if assetID, ok := metadata["avatar_asset_id"].(string); ok {
		if asset, found := DigitalEmployeeAvatarAssetByID(assetID); found {
			return &asset
		}
	}
	avatar, ok := metadata["avatar"].(map[string]any)
	if !ok {
		return nil
	}
	assetID, _ := avatar["id"].(string)
	if asset, found := DigitalEmployeeAvatarAssetByID(assetID); found {
		return &asset
	}
	return nil
}

func metadataWithAvatarAsset(metadata map[string]any, asset DigitalEmployeeAvatarAsset) map[string]any {
	next := cloneMap(metadata)
	next["avatar_asset_id"] = asset.ID
	next["avatar"] = map[string]any{
		"id":            asset.ID,
		"label":         asset.Label,
		"gender":        asset.Gender,
		"age_range":     asset.AgeRange,
		"style":         asset.Style,
		"provider":      "superteam-generated",
		"image_url":     asset.ImageURL,
		"thumbnail_url": asset.ThumbnailURL,
		"source":        asset.Source,
		"license":       asset.License,
		"status":        asset.Status,
	}
	return next
}

func digitalEmployeeAvatarAssetFromShared(asset avatar.Asset) DigitalEmployeeAvatarAsset {
	return DigitalEmployeeAvatarAsset{
		ID:           asset.ID,
		Label:        asset.Label,
		Gender:       asset.Gender,
		AgeRange:     asset.AgeRange,
		Style:        asset.Style,
		ImageURL:     asset.ImageURL,
		ThumbnailURL: asset.ThumbnailURL,
		Source:       asset.Source,
		License:      asset.License,
		Status:       asset.Status,
	}
}

func normalizeAvatarAssetID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
