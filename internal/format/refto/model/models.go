package model

import (
	"fmt"
	"math"
	"time"
)

// LibraryManifest represents act-library.json at the root of a .reflib library.
type LibraryManifest struct {
	Schema     int       `json:"schema"`
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"createdAt"`
	AppVersion string    `json:"appVersion"`
	Storage    string    `json:"storage"`
}

// TagsSidecar represents tags.json at the root of a .reflib library.
type TagsSidecar struct {
	Version int               `json:"version"`
	Groups  []TagGroupSidecar `json:"groups"`
	Tags    []TagSidecar      `json:"tags"`
}

// TagGroupSidecar represents one group entry inside tags.json.
type TagGroupSidecar struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	Position  int       `json:"position,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// TagSidecar represents one tag entry inside tags.json.
type TagSidecar struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Pinyin    string    `json:"pinyin,omitempty"`
	Color     string    `json:"color,omitempty"`
	Starred   bool      `json:"starred,omitempty"`
	GroupID   string    `json:"groupId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ItemSidecar represents items/<ULID>/item.json in a .reflib library.
type ItemSidecar struct {
	Version int              `json:"version"`
	Item    ItemSidecarBody  `json:"item"`
	Folders []string         `json:"folders,omitempty"`
	Tags    []ItemTagSidecar `json:"tags,omitempty"`
}

// ItemSidecarBody stores the item payload inside items/<ULID>/item.json.
type ItemSidecarBody struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Ext       string     `json:"ext"`
	Mime      string     `json:"mime,omitempty"`
	Name      string     `json:"name"`
	FileName  string     `json:"fileName"`
	Storage   string     `json:"storage"`
	RelPath   string     `json:"relPath,omitempty"`
	AbsPath   string     `json:"absPath,omitempty"`
	Size      int64      `json:"size"`
	Hash      string     `json:"hash,omitempty"`
	Width     int        `json:"width,omitempty"`
	Height    int        `json:"height,omitempty"`
	Duration  float64    `json:"duration,omitempty"`
	Rating    int        `json:"rating,omitempty"`
	Note      string     `json:"note,omitempty"`
	SourceURL string     `json:"sourceUrl,omitempty"`
	FileCTime *time.Time `json:"fileCTime,omitempty"`
	FileMTime *time.Time `json:"fileMTime,omitempty"`
	AddedAt   time.Time  `json:"addedAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
}

// ItemTagSidecar stores one tag assignment inside items/<ULID>/item.json.
type ItemTagSidecar struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Source     string    `json:"source"`
	State      string    `json:"state"`
	Confidence float64   `json:"confidence"`
	AddedAt    time.Time `json:"addedAt"`
}

// Item mirrors the `items` table in act.db.
type Item struct {
	RID             int64   `gorm:"column:rid;primaryKey"`
	ID              string  `gorm:"column:id"`
	Kind            string  `gorm:"column:kind"`
	Ext             string  `gorm:"column:ext"`
	Mime            string  `gorm:"column:mime"`
	Name            string  `gorm:"column:name"`
	FileName        string  `gorm:"column:file_name"`
	Storage         string  `gorm:"column:storage"`
	RelPath         string  `gorm:"column:rel_path"`
	AbsPath         string  `gorm:"column:abs_path"`
	Size            int64   `gorm:"column:size"`
	Hash            string  `gorm:"column:hash"`
	Fingerprint     string  `gorm:"column:fingerprint"`
	Width           int     `gorm:"column:width"`
	Height          int     `gorm:"column:height"`
	Duration        float64 `gorm:"column:duration"`
	Pages           int     `gorm:"column:pages"`
	Meta            string  `gorm:"column:meta"`
	Rating          int     `gorm:"column:rating"`
	Note            string  `gorm:"column:note"`
	SourceURL       string  `gorm:"column:source_url"`
	SourceApp       string  `gorm:"column:source_app"`
	FileCTime       *int64  `gorm:"column:file_ctime"`
	FileMTime       *int64  `gorm:"column:file_mtime"`
	AddedAt         int64   `gorm:"column:added_at"`
	UpdatedAt       int64   `gorm:"column:updated_at;autoUpdateTime:false"`
	DeletedAt       *int64  `gorm:"column:deleted_at"`
	PrimaryColor    string  `gorm:"column:primary_color"`
	ThumbState      string  `gorm:"column:thumb_state"`
	ThumbExt        string  `gorm:"column:thumb_ext"`
	ThumbW          int     `gorm:"column:thumb_w"`
	ThumbH          int     `gorm:"column:thumb_h"`
	AIState         string  `gorm:"column:ai_state"`
	Missing         int     `gorm:"column:missing"`
	TagsText        string  `gorm:"column:tags_text"`
	PinnedAt        int64   `gorm:"column:pinned_at"`
	Cover           string  `gorm:"column:cover"`
	Live            string  `gorm:"column:live"`
	LiveID          string  `gorm:"column:live_id"`
	LiveDuration    float64 `gorm:"column:live_duration"`
	OriginalAddedAt *int64  `gorm:"column:original_added_at"`
	ArchivedAt      int64   `gorm:"column:archived_at"`
	StackID         string  `gorm:"column:stack_id"`
	StackPos        int     `gorm:"column:stack_pos"`
}

func (Item) TableName() string { return "items" }

// Folder mirrors the `folders` table in act.db.
type Folder struct {
	ID          string  `gorm:"column:id;primaryKey"`
	ParentID    *string `gorm:"column:parent_id"`
	Position    int     `gorm:"column:position"`
	Name        string  `gorm:"column:name"`
	Pinyin      string  `gorm:"column:pinyin"`
	Description string  `gorm:"column:description"`
	Icon        string  `gorm:"column:icon"`
	Color       string  `gorm:"column:color"`
	CoverItemID *string `gorm:"column:cover_item_id"`
	AutoTags    string  `gorm:"column:auto_tags"`
	Sort        string  `gorm:"column:sort"`
	CreatedAt   int64   `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt   int64   `gorm:"column:updated_at;autoUpdateTime:false"`
}

func (Folder) TableName() string { return "folders" }

// ItemFolder mirrors the `item_folders` join table in act.db.
type ItemFolder struct {
	ItemID   string  `gorm:"column:item_id;primaryKey"`
	FolderID string  `gorm:"column:folder_id;primaryKey"`
	AddedAt  int64   `gorm:"column:added_at"`
	Position float64 `gorm:"column:position"`
}

func (ItemFolder) TableName() string { return "item_folders" }

// TagGroup mirrors the `tag_groups` table in act.db.
type TagGroup struct {
	ID        string `gorm:"column:id;primaryKey"`
	Name      string `gorm:"column:name"`
	Color     string `gorm:"column:color"`
	Position  int    `gorm:"column:position"`
	CreatedAt int64  `gorm:"column:created_at;autoCreateTime:false"`
}

func (TagGroup) TableName() string { return "tag_groups" }

// Tag mirrors the `tags` table in act.db.
type Tag struct {
	ID        string  `gorm:"column:id;primaryKey"`
	Name      string  `gorm:"column:name"`
	Pinyin    string  `gorm:"column:pinyin"`
	GlossEN   string  `gorm:"column:gloss_en"`
	Color     string  `gorm:"column:color"`
	Starred   int     `gorm:"column:starred"`
	GroupID   *string `gorm:"column:group_id"`
	CreatedAt int64   `gorm:"column:created_at;autoCreateTime:false"`
	UpdatedAt int64   `gorm:"column:updated_at;autoUpdateTime:false"`
}

func (Tag) TableName() string { return "tags" }

// ItemTag mirrors the `item_tags` join table in act.db.
type ItemTag struct {
	ItemID     string  `gorm:"column:item_id;primaryKey"`
	TagID      string  `gorm:"column:tag_id;primaryKey"`
	Source     string  `gorm:"column:source"`
	State      string  `gorm:"column:state"`
	Confidence float64 `gorm:"column:confidence"`
	CreatedAt  int64   `gorm:"column:created_at;autoCreateTime:false"`
}

func (ItemTag) TableName() string { return "item_tags" }

// Palette mirrors the `palettes` table in act.db.
type Palette struct {
	ItemID string  `gorm:"column:item_id;primaryKey"`
	Rank   int     `gorm:"column:rank;primaryKey"`
	RGB    int64   `gorm:"column:rgb"`
	L      float64 `gorm:"column:l"`
	A      float64 `gorm:"column:a"`
	B      float64 `gorm:"column:b"`
	Ratio  float64 `gorm:"column:ratio"`
	Accent int     `gorm:"column:accent"`
}

func (Palette) TableName() string { return "palettes" }

// PackRGB encodes an 8-bit RGB triple into Refto's packed 24-bit integer.
func PackRGB(r, g, b uint8) int64 {
	return (int64(r) << 16) | (int64(g) << 8) | int64(b)
}

// UnpackRGB decodes Refto's packed 24-bit integer into (R, G, B).
func UnpackRGB(rgb int64) (r, g, b uint8) {
	return uint8((rgb >> 16) & 0xFF), uint8((rgb >> 8) & 0xFF), uint8(rgb & 0xFF)
}

// RGBToHex formats (r, g, b) as lowercase "#rrggbb" matching items.primary_color.
func RGBToHex(r, g, b uint8) string {
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// SRGBToOKLab converts 8-bit sRGB channels into perceptual OKLab (L, a, b).
func SRGBToOKLab(r, g, b uint8) (l, a, okb float64) {
	lr := srgbToLinear(float64(r) / 255.0)
	lg := srgbToLinear(float64(g) / 255.0)
	lb := srgbToLinear(float64(b) / 255.0)

	lmsL := 0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb
	lmsM := 0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb
	lmsS := 0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb

	l_ := math.Cbrt(lmsL)
	m_ := math.Cbrt(lmsM)
	s_ := math.Cbrt(lmsS)

	l = 0.2104542553*l_ + 0.7936177850*m_ - 0.0040720468*s_
	a = 1.9779984951*l_ - 2.4285922050*m_ + 0.4505937099*s_
	okb = 0.0259040371*l_ + 0.7827717662*m_ - 0.8086757660*s_
	return l, a, okb
}

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
