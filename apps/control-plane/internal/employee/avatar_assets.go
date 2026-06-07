package employee

import "strings"

const (
	avatarStylePhotorealistic2D = "photorealistic_2d"
	avatarSourceInternalPack    = "ai_generated_internal_pack"
	avatarLicenseInternalAsset  = "internal_product_asset"
	avatarStatusActive          = "active"
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
}

var builtInDigitalEmployeeAvatarAssets = []DigitalEmployeeAvatarAsset{
	avatarAsset("engineer-m-01", "工程师头像 M01", "male", "24"),
	avatarAsset("engineer-m-02", "工程师头像 M02", "male", "31"),
	avatarAsset("engineer-m-03", "工程师头像 M03", "male", "28"),
	avatarAsset("engineer-m-04", "工程师头像 M04", "male", "38"),
	avatarAsset("engineer-m-05", "工程师头像 M05", "male", "35"),
	avatarAsset("engineer-m-06", "工程师头像 M06", "male", "29"),
	avatarAsset("engineer-m-07", "工程师头像 M07", "male", "22"),
	avatarAsset("engineer-m-08", "工程师头像 M08", "male", "33"),
	avatarAsset("engineer-m-09", "工程师头像 M09", "male", "27"),
	avatarAsset("engineer-m-10", "工程师头像 M10", "male", "40"),
	avatarAsset("engineer-f-01", "工程师头像 F01", "female", "23"),
	avatarAsset("engineer-f-02", "工程师头像 F02", "female", "30"),
	avatarAsset("engineer-f-03", "工程师头像 F03", "female", "27"),
	avatarAsset("engineer-f-04", "工程师头像 F04", "female", "34"),
	avatarAsset("engineer-f-05", "工程师头像 F05", "female", "37"),
	avatarAsset("engineer-f-06", "工程师头像 F06", "female", "32"),
	avatarAsset("engineer-f-07", "工程师头像 F07", "female", "21"),
	avatarAsset("engineer-f-08", "工程师头像 F08", "female", "39"),
	avatarAsset("engineer-f-09", "工程师头像 F09", "female", "26"),
	avatarAsset("engineer-f-10", "工程师头像 F10", "female", "29"),
}

func ListDigitalEmployeeAvatarAssets() []DigitalEmployeeAvatarAsset {
	assets := make([]DigitalEmployeeAvatarAsset, len(builtInDigitalEmployeeAvatarAssets))
	copy(assets, builtInDigitalEmployeeAvatarAssets)
	return assets
}

func DigitalEmployeeAvatarAssetByID(id string) (DigitalEmployeeAvatarAsset, bool) {
	normalized := normalizeAvatarAssetID(id)
	for _, asset := range builtInDigitalEmployeeAvatarAssets {
		if asset.ID == normalized && asset.Status == avatarStatusActive {
			return asset, true
		}
	}
	return DigitalEmployeeAvatarAsset{}, false
}

func avatarAssetFromEmployeeMetadata(metadata map[string]any) *DigitalEmployeeAvatarAsset {
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

func normalizeAvatarAssetID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}

func avatarAsset(id, label, gender, ageRange string) DigitalEmployeeAvatarAsset {
	return DigitalEmployeeAvatarAsset{
		ID:           id,
		Label:        label,
		Gender:       gender,
		AgeRange:     ageRange,
		Style:        avatarStylePhotorealistic2D,
		ImageURL:     "/images/digital-employee-avatars/" + id + ".webp",
		ThumbnailURL: "/images/digital-employee-avatars/" + id + "-256.webp",
		Source:       avatarSourceInternalPack,
		License:      avatarLicenseInternalAsset,
		Status:       avatarStatusActive,
	}
}
