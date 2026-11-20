package debris

// CopyMode specifies how asset files are transferred into the target library.
type CopyMode string

const (
	CopyModeCopy     CopyMode = "copy"
	CopyModeHardlink CopyMode = "hardlink"
	CopyModeAuto     CopyMode = "auto"
)

// Capabilities describes the features supported by a library format.
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
	SupportsOriginURL        bool `json:"supportsOriginUrl"`
	SupportsAnnotation       bool `json:"supportsAnnotation"`
}

// FormatSpec describes a supported library format and its capabilities.
type FormatSpec struct {
	ID           string       `json:"id"`
	DisplayName  string       `json:"displayName"`
	Capabilities Capabilities `json:"capabilities"`
}

// ProbeRequest asks Debris to detect whether Path matches a library format.
type ProbeRequest struct {
	Path   string `json:"path"`
	Format string `json:"format,omitempty"`
}

// ProbeResponse reports the detected format and metadata for a library path.
type ProbeResponse struct {
	Matched    bool           `json:"matched"`
	Format     string         `json:"format"`
	Version    string         `json:"version,omitempty"`
	RootPath   string         `json:"rootPath,omitempty"`
	Name       string         `json:"name,omitempty"`
	Confidence int            `json:"confidence,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

// InspectRequest asks Debris to summarize a source library without converting it.
type InspectRequest struct {
	Path           string `json:"path"`
	Format         string `json:"format,omitempty"`
	IncludeDeleted bool   `json:"includeDeleted,omitempty"`
	IncludeHidden  bool   `json:"includeHidden,omitempty"`
}

// InspectResponse summarizes the contents of a source library.
type InspectResponse struct {
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

// PlanRequest previews a conversion and potential capability/metadata losses.
type PlanRequest struct {
	From           string   `json:"from,omitempty"`
	To             string   `json:"to"`
	InputPath      string   `json:"inputPath"`
	OutputPath     string   `json:"outputPath"`
	CopyMode       CopyMode `json:"copyMode,omitempty"`
	IncludeDeleted bool     `json:"includeDeleted,omitempty"`
	IncludeHidden  bool     `json:"includeHidden,omitempty"`
	Overwrite      bool     `json:"overwrite,omitempty"`
	LibraryName    string   `json:"libraryName,omitempty"`
}

// PlanResponse describes the projected conversion counts and compatibility notes.
type PlanResponse struct {
	SourceFormat  string   `json:"sourceFormat"`
	TargetFormat  string   `json:"targetFormat"`
	InputPath     string   `json:"inputPath"`
	OutputPath    string   `json:"outputPath"`
	CopyMode      CopyMode `json:"copyMode"`
	FolderCount   int      `json:"folderCount"`
	TagGroupCount int      `json:"tagGroupCount"`
	TagCount      int      `json:"tagCount"`
	AssetCount    int      `json:"assetCount"`
	TotalBytes    int64    `json:"totalBytes"`
	Warnings      []string `json:"warnings,omitempty"`
	Losses        []string `json:"losses,omitempty"`
}

// ConvertRequest executes a conversion between two formats.
type ConvertRequest struct {
	From           string   `json:"from,omitempty"`
	To             string   `json:"to"`
	InputPath      string   `json:"inputPath"`
	OutputPath     string   `json:"outputPath"`
	CopyMode       CopyMode `json:"copyMode,omitempty"`
	IncludeDeleted bool     `json:"includeDeleted,omitempty"`
	IncludeHidden  bool     `json:"includeHidden,omitempty"`
	Overwrite      bool     `json:"overwrite,omitempty"`
	LibraryName    string   `json:"libraryName,omitempty"`
}

// ConvertResponse summarizes the completed conversion run.
type ConvertResponse struct {
	SourceFormat     string   `json:"sourceFormat"`
	TargetFormat     string   `json:"targetFormat"`
	InputPath        string   `json:"inputPath"`
	OutputPath       string   `json:"outputPath"`
	CopyMode         CopyMode `json:"copyMode"`
	FolderCount      int      `json:"folderCount"`
	TagGroupCount    int      `json:"tagGroupCount"`
	TagCount         int      `json:"tagCount"`
	AssetCount       int      `json:"assetCount"`
	CopiedAssets     int      `json:"copiedAssets"`
	HardlinkedAssets int      `json:"hardlinkedAssets"`
	SkippedAssets    int      `json:"skippedAssets"`
	TotalBytes       int64    `json:"totalBytes"`
	DurationMs       int64    `json:"durationMs"`
	Warnings         []string `json:"warnings,omitempty"`
}

// Event represents a streaming progress, stage, or warning notification.
type Event struct {
	Kind     string `json:"kind"` // "stage", "progress", "warning"
	Stage    string `json:"stage,omitempty"`
	Current  int64  `json:"current,omitempty"`
	Total    int64  `json:"total,omitempty"`
	Bytes    int64  `json:"bytes,omitempty"`
	AssetID  string `json:"assetId,omitempty"`
	CopyMode string `json:"copyMode,omitempty"`
	Message  string `json:"message,omitempty"`
}

// EventCallback receives asynchronous conversion events.
type EventCallback func(Event)
