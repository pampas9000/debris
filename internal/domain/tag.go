package domain

import "time"

// TagGroup represents a named group of tags (e.g. Billfish tag groups,
// Eagle tagGroups, Pixcall tag_groups).
type TagGroup struct {
	ID       string `json:"id"`
	ParentID string `json:"parentId,omitempty"`
	Name     string `json:"name"`
	Color    string `json:"color,omitempty"`
	Pinyin   string `json:"pinyin,omitempty"`
	Ranking  int64  `json:"ranking,omitempty"`
}

// Tag represents an individual tag in the library IR.
type Tag struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parentId,omitempty"`
	GroupID   string    `json:"groupId,omitempty"`
	Name      string    `json:"name"`
	Pinyin    string    `json:"pinyin,omitempty"`
	Category  string    `json:"category,omitempty"`
	Color     string    `json:"color,omitempty"`
	Icon      string    `json:"icon,omitempty"`
	Seq       int64     `json:"seq,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
}
