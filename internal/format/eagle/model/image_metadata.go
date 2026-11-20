package model

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
)

// Palette represents a dominant color entry in an Eagle item's palettes array.
type Palette struct {
	Color [3]uint8 `json:"color"`
	Ratio float64  `json:"ratio"`
}

// MarshalJSONTo encodes Palette with Color as a 3-element JSON number array
// because encoding/json/v2 encodes [3]uint8 as a base64 string by default.
func (p Palette) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, struct {
		Color [3]uint16 `json:"color"`
		Ratio float64   `json:"ratio"`
	}{
		Color: [3]uint16{uint16(p.Color[0]), uint16(p.Color[1]), uint16(p.Color[2])},
		Ratio: p.Ratio,
	})
}

// UnmarshalJSONFrom decodes Palette with Color from a 3-element JSON number array.
func (p *Palette) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	var raw struct {
		Color [3]uint16 `json:"color"`
		Ratio float64   `json:"ratio"`
	}
	if err := json.UnmarshalDecode(dec, &raw); err != nil {
		return err
	}
	for i, c := range raw.Color {
		if c > 255 {
			return fmt.Errorf("palette color channel %d out of uint8 range: %d", i, c)
		}
		p.Color[i] = uint8(c)
	}
	p.Ratio = raw.Ratio
	return nil
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
