package pixcall

import (
	"math"
	"mime"
	"path/filepath"
	"strconv"
	"strings"

	"debris/internal/domain"
)

type entryMetadataJSON struct {
	ColorPalette string  `json:"color_palette,omitempty"`
	Creation     string  `json:"creation,omitempty"`
	Modification string  `json:"modification,omitempty"`
	CustomThumb  *bool   `json:"custom_thumb,omitempty"`
	HasThumb     *bool   `json:"has_thumb,omitempty"`
	ImageWidth   int     `json:"image_width,omitempty"`
	ImageHeight  int     `json:"image_height,omitempty"`
	Duration     float64 `json:"duration,omitempty"`
	Orientation  int     `json:"orientation,omitempty"`
	IsAnimated   *bool   `json:"is_animated,omitempty"`
	ThumbHash    string  `json:"thumb_hash,omitempty"`
	ThumbWidth   int     `json:"thumb_width,omitempty"`
	ThumbHeight  int     `json:"thumb_height,omitempty"`
}

type mediaMetadataJSON struct {
	Duration float64        `json:"duration,omitempty"`
	BitRate  int64          `json:"bit_rate,omitempty"`
	Format   string         `json:"format,omitempty"`
	Width    int            `json:"width,omitempty"`
	Height   int            `json:"height,omitempty"`
	Rotation float64        `json:"rotation,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
	Streams  []any          `json:"streams,omitempty"`
}

// parseColorPalette decodes Pixcall's comma-separated uint32 RGBA palette
// (0xRRGGBBWW where WW is the weight out of 255).
func parseColorPalette(raw string) []domain.PaletteColor {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	colors := make([]domain.PaletteColor, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		v, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			continue
		}
		weight := uint8(v & 0xff)
		ratio := math.Round((float64(weight)/255.0)*10000) / 100
		colors = append(colors, domain.PaletteColor{
			R:     uint8((v >> 24) & 0xff),
			G:     uint8((v >> 16) & 0xff),
			B:     uint8((v >> 8) & 0xff),
			Ratio: ratio,
		})
	}
	if len(colors) == 0 {
		return nil
	}
	return colors
}

// encodeColorPalette packs domain.PaletteColor values into Pixcall's
// comma-separated uint32 string (0xRRGGBBWW).
func encodeColorPalette(colors []domain.PaletteColor) string {
	if len(colors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(colors))
	for _, c := range colors {
		ratio := c.Ratio
		if ratio < 0 {
			ratio = 0
		}
		if ratio > 100 {
			ratio = 100
		}
		weight := min(uint32(math.Round((ratio/100.0)*255.0)), 255)
		packed := (uint32(c.R) << 24) | (uint32(c.G) << 16) | (uint32(c.B) << 8) | weight
		parts = append(parts, strconv.FormatUint(uint64(packed), 10))
	}
	return strings.Join(parts, ",")
}

func splitPipeIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func deriveLibraryName(absPath string) string {
	base := filepath.Base(absPath)
	trimmed := strings.TrimSuffix(base, ".library")
	if trimmed != "" {
		return trimmed
	}
	return base
}

var extMIMEOverrides = map[string]string{
	"heic":     "image/heic",
	"heif":     "image/heif",
	"avif":     "image/avif",
	"jxl":      "image/jxl",
	"webp":     "image/webp",
	"jpg":      "image/jpeg",
	"jpeg":     "image/jpeg",
	"png":      "image/png",
	"gif":      "image/gif",
	"bmp":      "image/bmp",
	"svg":      "image/svg+xml",
	"tif":      "image/tiff",
	"tiff":     "image/tiff",
	"ico":      "image/x-icon",
	"raw":      "image/x-raw",
	"dng":      "image/x-adobe-dng",
	"cr2":      "image/x-canon-cr2",
	"nef":      "image/x-nikon-nef",
	"arw":      "image/x-sony-arw",
	"mp4":      "video/mp4",
	"m4v":      "video/mp4",
	"mov":      "video/quicktime",
	"webm":     "video/webm",
	"mkv":      "video/x-matroska",
	"avi":      "video/x-msvideo",
	"wmv":      "video/x-ms-wmv",
	"flv":      "video/x-flv",
	"mp3":      "audio/mpeg",
	"wav":      "audio/wav",
	"flac":     "audio/flac",
	"aac":      "audio/aac",
	"ogg":      "audio/ogg",
	"m4a":      "audio/mp4",
	"ttf":      "font/ttf",
	"otf":      "font/otf",
	"woff":     "font/woff",
	"woff2":    "font/woff2",
	"ttc":      "font/collection",
	"psd":      "image/vnd.adobe.photoshop",
	"psb":      "image/vnd.adobe.photoshop",
	"ai":       "application/postscript",
	"eps":      "application/postscript",
	"pdf":      "application/pdf",
	"sketch":   "application/octet-stream",
	"fig":      "application/octet-stream",
	"xd":       "application/octet-stream",
	"afdesign": "application/octet-stream",
	"afphoto":  "application/octet-stream",
}

func mimeTypeFromExt(ext string, kind domain.AssetKind) string {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext != "" {
		if m, ok := extMIMEOverrides[ext]; ok {
			return m
		}
		if m := mime.TypeByExtension("." + ext); m != "" {
			if idx := strings.IndexByte(m, ';'); idx >= 0 {
				m = strings.TrimSpace(m[:idx])
			}
			if m != "" {
				return m
			}
		}
	}
	switch kind {
	case domain.KindImage:
		return "image/unknown"
	case domain.KindVideo:
		return "video/unknown"
	case domain.KindAudio:
		return "audio/unknown"
	case domain.KindFont:
		return "font/unknown"
	default:
		return "application/octet-stream"
	}
}

func classifyAssetKind(contentType, ext string) domain.AssetKind {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")

	switch ext {
	case "psd", "psb", "ai", "sketch", "fig", "xd", "c4d", "blend", "aep", "prproj", "indd", "cdr", "afdesign", "afphoto":
		return domain.KindProject
	case "ttf", "otf", "woff", "woff2", "ttc":
		return domain.KindFont
	}

	switch {
	case strings.HasPrefix(ct, "image/"):
		return domain.KindImage
	case strings.HasPrefix(ct, "video/"):
		return domain.KindVideo
	case strings.HasPrefix(ct, "audio/"):
		return domain.KindAudio
	case strings.HasPrefix(ct, "font/"):
		return domain.KindFont
	}

	switch ext {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "tif", "tiff", "heic", "heif", "avif", "jxl", "ico", "raw", "dng", "cr2", "nef", "arw":
		return domain.KindImage
	case "mp4", "m4v", "mov", "webm", "mkv", "avi", "wmv", "flv":
		return domain.KindVideo
	case "mp3", "wav", "flac", "aac", "ogg", "m4a", "opus":
		return domain.KindAudio
	default:
		return domain.KindOther
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
