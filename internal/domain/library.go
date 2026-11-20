package domain

import "time"

// Library is the format-neutral intermediate representation (IR) of a design
// asset library (Billfish, Eagle, or Pixcall).
type Library struct {
	Format     string      `json:"format"`
	Version    string      `json:"version,omitempty"`
	RootPath   string      `json:"rootPath"`
	Name       string      `json:"name,omitempty"`
	CreatedAt  time.Time   `json:"createdAt,omitzero"`
	ModifiedAt time.Time   `json:"modifiedAt,omitzero"`
	Folders    []*Folder   `json:"folders,omitempty"`
	TagGroups  []*TagGroup `json:"tagGroups,omitempty"`
	Tags       []*Tag      `json:"tags,omitempty"`
	Assets     []*Asset    `json:"assets,omitempty"`
}
