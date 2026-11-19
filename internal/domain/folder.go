package domain

import "time"

// Folder represents a logical folder/collection inside a Library.
// Adapters may populate either flat ParentID relationships, nested Children,
// or both; Seq and ChildIDs preserve ordering metadata across formats.
type Folder struct {
	ID          string         `json:"id"`
	ParentID    string         `json:"parentId,omitempty"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Color       string         `json:"color,omitempty"`
	Seq         int64          `json:"seq,omitempty"`
	IsRecycle   bool           `json:"isRecycle,omitempty"`
	IsHidden    bool           `json:"isHidden,omitempty"`
	Icon        string         `json:"icon,omitempty"`
	IconColor   string         `json:"iconColor,omitempty"`
	CreatedAt   time.Time      `json:"createdAt,omitempty"`
	ModifiedAt  time.Time      `json:"modifiedAt,omitempty"`
	Children    []*Folder      `json:"children,omitempty"`
	ChildIDs    []string       `json:"childIds,omitempty"`
	TagNames    []string       `json:"tagNames,omitempty"`
	CoverAssetID string        `json:"coverAssetId,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}
