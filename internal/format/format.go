package format

import (
	"context"
	"fmt"
	"strings"

	"debris/internal/domain"
)

// CopyMode specifies how asset files are transferred into the target library.
type CopyMode string

const (
	CopyModeCopy     CopyMode = "copy"
	CopyModeHardlink CopyMode = "hardlink"
	CopyModeAuto     CopyMode = "auto"
)

// ParseCopyMode normalizes and validates a copy mode string. Empty defaults to CopyModeAuto.
func ParseCopyMode(raw string) (CopyMode, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if normalized == "" {
		return CopyModeAuto, nil
	}
	mode := CopyMode(normalized)
	if !mode.IsValid() {
		return "", fmt.Errorf("invalid copy mode %q: expected copy, hardlink, or auto", raw)
	}
	return mode, nil
}

// IsValid reports whether m is a recognized CopyMode value.
func (m CopyMode) IsValid() bool {
	switch m {
	case CopyModeCopy, CopyModeHardlink, CopyModeAuto:
		return true
	default:
		return false
	}
}

// Capabilities describes the structural and metadata features supported by a format.
type Capabilities struct {
	SupportsFolders          bool `json:"supportsFolders"`
	SupportsNestedFolders    bool `json:"supportsNestedFolders"`
	SupportsMultiFolderAsset bool `json:"supportsMultiFolderAsset"`
	SupportsTagGroups        bool `json:"supportsTagGroups"`
	SupportsNestedTags       bool `json:"supportsNestedTags"`
	SupportsColors           bool `json:"supportsColors"`
	SupportsRecycleBin       bool `json:"supportsRecycleBin"`
	SupportsThumbnails       bool `json:"supportsThumbnails"`
	SupportsRating           bool `json:"supportsRating"`
	SupportsOriginURL        bool `json:"supportsOriginURL"`
	SupportsAnnotation       bool `json:"supportsAnnotation"`
}

// ProbeResult describes whether a path matches a format adapter and its detected metadata.
type ProbeResult struct {
	Matched    bool           `json:"matched"`
	Format     string         `json:"format"`
	Version    string         `json:"version,omitempty"`
	RootPath   string         `json:"rootPath,omitempty"`
	Name       string         `json:"name,omitempty"`
	Confidence int            `json:"confidence,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

// InspectResult summarizes a library's contents without reading every asset payload.
type InspectResult struct {
	Format        string         `json:"format"`
	Version       string         `json:"version,omitempty"`
	RootPath      string         `json:"rootPath"`
	Name          string         `json:"name,omitempty"`
	FolderCount   int            `json:"folderCount"`
	TagGroupCount int            `json:"tagGroupCount"`
	TagCount      int            `json:"tagCount"`
	AssetCount    int            `json:"assetCount"`
	DeletedCount  int            `json:"deletedCount,omitempty"`
	HiddenCount   int            `json:"hiddenCount,omitempty"`
	TotalBytes    int64          `json:"totalBytes"`
	Warnings      []string       `json:"warnings,omitempty"`
	Extra         map[string]any `json:"extra,omitempty"`
}

// ReadOptions configures how a Reader inspects or loads a library.
type ReadOptions struct {
	IncludeDeleted bool `json:"includeDeleted,omitempty"`
	IncludeHidden  bool `json:"includeHidden,omitempty"`
	SkipMediaProbe bool `json:"skipMediaProbe,omitempty"`
}

// WriteOptions configures how a Writer materializes a library on disk.
type WriteOptions struct {
	CopyMode       CopyMode `json:"copyMode,omitempty"`
	Overwrite      bool     `json:"overwrite,omitempty"`
	IncludeDeleted bool     `json:"includeDeleted,omitempty"`
	IncludeHidden  bool     `json:"includeHidden,omitempty"`
	LibraryName    string   `json:"libraryName,omitempty"`
}

// ProgressEvent reports progress, stage transitions, or non-fatal warnings from a Writer.
type ProgressEvent struct {
	Kind     string `json:"kind"` // "stage", "progress", "warning"
	Stage    string `json:"stage,omitempty"`
	Current  int64  `json:"current,omitempty"`
	Total    int64  `json:"total,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
	AssetID  string `json:"assetId,omitempty"`
	CopyMode string `json:"copyMode,omitempty"`
	Message  string `json:"message,omitempty"`
}

// ProgressCallback receives asynchronous progress and warning events during Write.
type ProgressCallback func(ProgressEvent)

// Adapter is the stateless entry point for a library format (e.g. billfish, eagle, pixcall).
type Adapter interface {
	ID() string
	DisplayName() string
	Capabilities() Capabilities
	Probe(ctx context.Context, path string) (ProbeResult, error)
	OpenReader(ctx context.Context, path string, opts ReadOptions) (Reader, error)
	OpenWriter(ctx context.Context, path string, opts WriteOptions) (Writer, error)
}

// Reader reads a source library into the domain IR.
type Reader interface {
	Inspect(ctx context.Context) (InspectResult, error)
	Read(ctx context.Context) (*domain.Library, error)
	Close() error
}

// Writer materializes a domain IR library into a target format on disk.
type Writer interface {
	Write(ctx context.Context, lib *domain.Library, emit ProgressCallback) error
	Close() error
}
