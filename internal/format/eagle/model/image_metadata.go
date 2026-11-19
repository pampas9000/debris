package model

// Palette represents a dominant color entry in an Eagle item's palettes array.
type Palette struct {
	Color [3]uint8 `json:"color"`
	Ratio float64  `json:"ratio"`
}

// ImageMetadata represents images/<id>.info/metadata.json in an Eagle 4.0.0 library.
type ImageMetadata struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Size             int64     `json:"size"`
	Btime            int64     `json:"btime"`
	Mtime            int64     `json:"mtime"`
	Ext              string    `json:"ext"`
	Tags             []string  `json:"tags"`
	Folders          []string  `json:"folders"`
	IsDeleted        bool      `json:"isDeleted"`
	URL              string    `json:"url"`
	Annotation       string    `json:"annotation"`
	ModificationTime int64     `json:"modificationTime"`
	Width            int       `json:"width,omitempty"`
	Height           int       `json:"height,omitempty"`
	ResolutionWidth  int       `json:"resolutionWidth,omitempty"`
	ResolutionHeight int       `json:"resolutionHeight,omitempty"`
	Duration         float64   `json:"duration,omitempty"`
	NoThumbnail      bool      `json:"noThumbnail,omitempty"`
	LastModified     int64     `json:"lastModified"`
	Star             int       `json:"star,omitempty"`
	Palettes         []Palette `json:"palettes,omitempty"`
}
