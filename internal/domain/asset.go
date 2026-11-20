package domain

import (
	"strings"
	"time"
)

// AssetKind classifies the high-level media type of an Asset.
type AssetKind string

const (
	KindImage   AssetKind = "image"
	KindVideo   AssetKind = "video"
	KindAudio   AssetKind = "audio"
	KindFont    AssetKind = "font"
	KindProject AssetKind = "project"
	KindOther   AssetKind = "other"
)

// PaletteColor represents one dominant color entry in an asset's color palette.
type PaletteColor struct {
	R     uint8   `json:"r"`
	G     uint8   `json:"g"`
	B     uint8   `json:"b"`
	Ratio float64 `json:"ratio"`
}

// MediaMeta holds format-specific or extended media metadata that does not
// belong on the core Asset fields.
type MediaMeta struct {
	Rotation      int            `json:"rotation,omitempty"`
	HFlip         bool           `json:"hflip,omitempty"`
	VFlip         bool           `json:"vflip,omitempty"`
	Bitrate       int64          `json:"bitrate,omitempty"`
	BPM           float64        `json:"bpm,omitempty"`
	Format        string         `json:"format,omitempty"`
	EXIF          map[string]any `json:"exif,omitempty"`
	ThumbnailPath string         `json:"thumbnailPath,omitempty"`
	NoThumbnail   bool           `json:"noThumbnail,omitempty"`
	IsAnimated    bool           `json:"isAnimated,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}

// Asset represents a single design asset/file in the library IR.
type Asset struct {
	ID          string         `json:"id"`
	FileName    string         `json:"fileName"`
	Name        string         `json:"name"`
	Ext         string         `json:"ext"`
	SourcePath  string         `json:"sourcePath"`
	Size        int64          `json:"size"`
	Hash        string         `json:"hash,omitempty"`
	Kind        AssetKind      `json:"kind"`
	Width       int            `json:"width,omitempty"`
	Height      int            `json:"height,omitempty"`
	DurationSec float64        `json:"durationSec,omitempty"`
	Rating      int            `json:"rating,omitempty"`
	Annotation  string         `json:"annotation,omitempty"`
	OriginURL   string         `json:"originUrl,omitempty"`
	Colors      []PaletteColor `json:"colors,omitempty"`
	FolderIDs   []string       `json:"folderIds,omitempty"`
	TagIDs      []string       `json:"tagIds,omitempty"`
	IsHidden    bool           `json:"isHidden,omitempty"`
	IsDeleted   bool           `json:"isDeleted,omitempty"`
	CreatedAt   time.Time      `json:"createdAt,omitzero"`
	ModifiedAt  time.Time      `json:"modifiedAt,omitzero"`
	MediaMeta   *MediaMeta     `json:"mediaMeta,omitempty"`
}

// EffectiveFileName returns FileName if set, otherwise Name plus normalized Ext.
func (a *Asset) EffectiveFileName() string {
	if a.FileName != "" {
		return a.FileName
	}
	ext := strings.TrimPrefix(strings.ToLower(a.Ext), ".")
	if ext == "" {
		return a.Name
	}
	return a.Name + "." + ext
}
