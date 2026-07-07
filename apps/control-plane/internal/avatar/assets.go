package avatar

import "strings"

const (
	StylePhotorealistic2D = "photorealistic_2d"
	SourceInternalPack    = "ai_generated_internal_pack"
	LicenseInternalAsset  = "internal_product_asset"
	StatusActive          = "active"
)

type Asset struct {
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

var builtInAssets = []Asset{
	asset("engineer-m-01", "工程师头像 M01", "male", "24"),
	asset("engineer-m-02", "工程师头像 M02", "male", "31"),
	asset("engineer-m-03", "工程师头像 M03", "male", "28"),
	asset("engineer-m-04", "工程师头像 M04", "male", "38"),
	asset("engineer-m-05", "工程师头像 M05", "male", "35"),
	asset("engineer-m-06", "工程师头像 M06", "male", "29"),
	asset("engineer-m-07", "工程师头像 M07", "male", "22"),
	asset("engineer-m-08", "工程师头像 M08", "male", "33"),
	asset("engineer-m-09", "工程师头像 M09", "male", "27"),
	asset("engineer-m-10", "工程师头像 M10", "male", "40"),
	asset("engineer-m-11", "工程师头像 M11", "male", "24"),
	asset("engineer-m-12", "工程师头像 M12", "male", "33"),
	asset("engineer-m-13", "工程师头像 M13", "male", "39"),
	asset("engineer-m-14", "工程师头像 M14", "male", "29"),
	asset("engineer-m-15", "工程师头像 M15", "male", "21"),
	asset("engineer-m-16", "工程师头像 M16", "male", "36"),
	asset("engineer-m-17", "工程师头像 M17", "male", "27"),
	asset("engineer-m-18", "工程师头像 M18", "male", "31"),
	asset("engineer-f-01", "工程师头像 F01", "female", "23"),
	asset("engineer-f-02", "工程师头像 F02", "female", "30"),
	asset("engineer-f-03", "工程师头像 F03", "female", "27"),
	asset("engineer-f-04", "工程师头像 F04", "female", "34"),
	asset("engineer-f-05", "工程师头像 F05", "female", "37"),
	asset("engineer-f-06", "工程师头像 F06", "female", "32"),
	asset("engineer-f-07", "工程师头像 F07", "female", "21"),
	asset("engineer-f-08", "工程师头像 F08", "female", "39"),
	asset("engineer-f-09", "工程师头像 F09", "female", "26"),
	asset("engineer-f-10", "工程师头像 F10", "female", "29"),
	asset("engineer-f-11", "工程师头像 F11", "female", "25"),
	asset("engineer-f-12", "工程师头像 F12", "female", "38"),
	asset("engineer-f-13", "工程师头像 F13", "female", "30"),
	asset("engineer-f-14", "工程师头像 F14", "female", "22"),
}

func ListBuiltInAssets() []Asset {
	assets := make([]Asset, len(builtInAssets))
	copy(assets, builtInAssets)
	return assets
}

func BuiltInAssetByID(id string) (Asset, bool) {
	normalized := strings.ToLower(strings.TrimSpace(id))
	for _, asset := range builtInAssets {
		if asset.ID == normalized && asset.Status == StatusActive {
			return asset, true
		}
	}
	return Asset{}, false
}

func asset(id, label, gender, ageRange string) Asset {
	return Asset{
		ID:           id,
		Label:        label,
		Gender:       gender,
		AgeRange:     ageRange,
		Style:        StylePhotorealistic2D,
		ImageURL:     "/images/digital-employee-avatars/" + id + ".webp",
		ThumbnailURL: "/images/digital-employee-avatars/" + id + "-256.webp",
		Source:       SourceInternalPack,
		License:      LicenseInternalAsset,
		Status:       StatusActive,
	}
}
