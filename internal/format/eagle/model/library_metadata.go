package model

// LibraryMetadata represents the root metadata.json file in an Eagle library.
type LibraryMetadata struct {
	Folders            []Folder   `json:"folders"`
	SmartFolders       []any      `json:"smartFolders"`
	QuickAccess        []any      `json:"quickAccess"`
	TagsGroups         []TagGroup `json:"tagsGroups"`
	ModificationTime   int64      `json:"modificationTime"`
	ApplicationVersion string     `json:"applicationVersion"`
}

// Folder represents a strongly-typed recursive folder node in Eagle metadata.json.
type Folder struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Children         []Folder `json:"children"`
	ModificationTime int64    `json:"modificationTime"`
	Tags             []string `json:"tags"`
	Password         string   `json:"password"`
	PasswordTips     string   `json:"passwordTips"`
	Icon             string   `json:"icon,omitempty"`
	IconColor        string   `json:"iconColor,omitempty"`
}

// TagGroup represents a tag group entry in Eagle's root metadata.json.
type TagGroup struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Tags     []string `json:"tags"`
	Color    string   `json:"color,omitempty"`
	ParentID string   `json:"parentId,omitempty"`
}

// TagsFile represents the root tags.json file in an Eagle library.
type TagsFile struct {
	HistoryTags []string `json:"historyTags"`
	StarredTags []string `json:"starredTags"`
}
