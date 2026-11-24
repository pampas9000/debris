package billfish

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"debris/internal/format"
	bfmodel "debris/internal/format/billfish/model"
	"debris/internal/kit"
)

const (
	// FormatID is the canonical adapter identifier for Billfish libraries.
	FormatID = "billfish"
	// FormatDisplayName is the human-readable name of the Billfish format.
	FormatDisplayName = "Billfish"
	// LibraryVersion is the internal Billfish SQLite library.version targeted by Writer,
	// corresponding to Billfish 3.0.32 (DefaultAppVersion).
	LibraryVersion = 41
	// DefaultAppVersion is the user-facing Billfish application version mapped from LibraryVersion.
	DefaultAppVersion = "3.0.32"

	// seqScale maps Billfish fractional REAL seq values (e.g. 0.9000025) into
	// int64 domain Seq fields without losing relative order or precision.
	seqScale = 1_000_000_000.0
)

// schemaAppVersions maps Billfish internal library.version integers to their
// corresponding user-facing Billfish application versions.
var schemaAppVersions = map[int]string{
	LibraryVersion: DefaultAppVersion, // library.version 41 -> Billfish 3.0.32
}

// AppVersionForSchema maps a Billfish internal library.version to its corresponding
// user-facing application version, returning "" if unmapped.
func AppVersionForSchema(libraryVersion int) string {
	return schemaAppVersions[libraryVersion]
}

// Adapter implements format.Adapter for Billfish asset libraries.
type Adapter struct{}

// New returns a stateless Billfish format.Adapter.
func New() format.Adapter {
	return &Adapter{}
}

// ID returns "billfish".
func (a *Adapter) ID() string {
	return FormatID
}

// DisplayName returns "Billfish".
func (a *Adapter) DisplayName() string {
	return FormatDisplayName
}

// Capabilities reports the metadata and structural features supported by Billfish.
func (a *Adapter) Capabilities() format.Capabilities {
	return format.Capabilities{
		SupportsFolders:          true,
		SupportsNestedFolders:    true,
		SupportsMultiFolderAsset: false,
		SupportsTagGroups:        true,
		SupportsNestedTags:       true,
		SupportsColors:           true,
		SupportsRecycleBin:       true,
		SupportsThumbnails:       true,
		SupportsRating:           true,
		SupportsOriginURL:        true,
		SupportsAnnotation:       true,
	}
}

// Probe checks whether path points to a valid Billfish library directory by
// verifying .bf/billfish.db and inspecting its library metadata row read-only.
func (a *Adapter) Probe(ctx context.Context, path string) (format.ProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return format.ProbeResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return format.ProbeResult{
			Matched: false,
			Format:  FormatID,
			Reason:  "empty library path",
		}, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return format.ProbeResult{}, fmt.Errorf("billfish: resolve path %q: %w", path, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return format.ProbeResult{
			Matched:  false,
			Format:   FormatID,
			RootPath: absPath,
			Reason:   fmt.Sprintf("stat path: %v", err),
		}, nil
	}
	if !info.IsDir() {
		return format.ProbeResult{
			Matched:  false,
			Format:   FormatID,
			RootPath: absPath,
			Reason:   "library path is not a directory",
		}, nil
	}

	dbPath := billfishDBPath(absPath)
	dbInfo, err := os.Stat(dbPath)
	if err != nil || dbInfo.IsDir() {
		return format.ProbeResult{
			Matched:  false,
			Format:   FormatID,
			RootPath: absPath,
			Reason:   "missing .bf/billfish.db",
		}, nil
	}

	db, closer, err := kit.OpenReadOnly(dbPath)
	if err != nil {
		return format.ProbeResult{
			Matched:  false,
			Format:   FormatID,
			RootPath: absPath,
			Reason:   fmt.Sprintf("open .bf/billfish.db: %v", err),
		}, nil
	}
	defer closer.Close()

	var libRow bfmodel.Library
	if err := db.WithContext(ctx).Order("id ASC").Limit(1).Find(&libRow).Error; err != nil {
		if ctx.Err() != nil {
			return format.ProbeResult{}, ctx.Err()
		}
		return format.ProbeResult{
			Matched:  false,
			Format:   FormatID,
			RootPath: absPath,
			Reason:   fmt.Sprintf("query library table: %v", err),
		}, nil
	}
	if libRow.ID == 0 && libRow.Version == 0 {
		return format.ProbeResult{
			Matched:  false,
			Format:   FormatID,
			RootPath: absPath,
			Reason:   "library metadata row not found",
		}, nil
	}

	name := readLibraryName(absPath)
	details := map[string]any{
		"dbPath": dbPath,
	}
	if libRow.Version > 0 {
		details["schemaVersion"] = libRow.Version
	}
	if libRow.Platform != "" {
		details["platform"] = libRow.Platform
	}

	return format.ProbeResult{
		Matched:    true,
		Format:     FormatID,
		Version:    AppVersionForSchema(libRow.Version),
		RootPath:   absPath,
		Name:       name,
		Confidence: 100,
		Reason:     "detected .bf/billfish.db",
		Details:    details,
	}, nil
}

// OpenReader opens a Billfish library for read-only inspection and IR extraction.
func (a *Adapter) OpenReader(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
	return newReader(ctx, path, opts)
}

// OpenWriter opens a Billfish library target for writing from domain IR.
func (a *Adapter) OpenWriter(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
	return newWriter(ctx, path, opts)
}

func billfishDBPath(root string) string {
	return filepath.Join(root, ".bf", "billfish.db")
}

type libInfoFile struct {
	AudioVolume              int    `json:"audioVolume"`
	CloseLibraryType         string `json:"closeLibrarType"`
	CloseLibraryID           string `json:"closeLibraryId"`
	CloudLocalFolderID       string `json:"cloudLocalFolderId"`
	CoverPath                string `json:"coverPath"`
	CreatedDefaultCloudLocal bool   `json:"createdDefaultCloudLocalFolder"`
	CreatedDefaultDownload   bool   `json:"createdDefaultDownloadFolder"`
	DownloadFolderID         string `json:"downloadFolderId"`
	DownloadFolderName       string `json:"downloadFolderName"`
	Folders                  []any  `json:"folders"`
	ImportType               int    `json:"importType"`
	LibraryName              string `json:"libraryName"`
	OpenedColorOptimize      bool   `json:"openedColorOptimize"`
	PreviewType              int    `json:"previewType"`
	TagIDs                   []any  `json:"tag_ids"`
	VideoVolume              int    `json:"videoVolume"`
}

func readLibraryName(root string) string {
	infoPath := filepath.Join(root, ".bf", ".ui_config", "lib_info.json")
	data, err := os.ReadFile(infoPath)
	if err == nil {
		var info libInfoFile
		if jsonErr := json.Unmarshal(data, &info); jsonErr == nil {
			if trimmed := strings.TrimSpace(info.LibraryName); trimmed != "" {
				return trimmed
			}
		}
	}
	base := filepath.Base(root)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}
