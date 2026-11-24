package pixcall

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"debris/internal/format"
	"debris/internal/format/pixcall/model"
	"debris/internal/kit"
)

const (
	formatID          = "pixcall"
	formatDisplayName = "Pixcall"

	rootEntryID  int64 = 1
	trashEntryID int64 = 2
)

// Adapter implements format.Adapter for Pixcall asset libraries (Pixcall 0.9.7+build5, internal schema_version 22).
type Adapter struct{}

// New returns a stateless format.Adapter for Pixcall libraries.
func New() format.Adapter {
	return &Adapter{}
}

// ID returns the canonical format identifier ("pixcall").
func (a *Adapter) ID() string {
	return formatID
}

// DisplayName returns the human-readable format name ("Pixcall").
func (a *Adapter) DisplayName() string {
	return formatDisplayName
}

// Capabilities reports the structural and metadata features supported by Pixcall.
func (a *Adapter) Capabilities() format.Capabilities {
	return format.Capabilities{
		SupportsFolders:          true,
		SupportsNestedFolders:    true,
		SupportsMultiFolderAsset: false,
		SupportsTagGroups:        true,
		SupportsNestedTags:       false,
		SupportsColors:           true,
		SupportsRecycleBin:       true,
		SupportsThumbnails:       false,
		SupportsRating:           true,
		SupportsOriginURL:        true,
		SupportsAnnotation:       true,
	}
}

// Probe inspects path to determine whether it is a valid Pixcall library directory.
func (a *Adapter) Probe(ctx context.Context, path string) (format.ProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return format.ProbeResult{}, err
	}

	if strings.TrimSpace(path) == "" {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  "empty library path",
		}, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  err.Error(),
		}, nil
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  err.Error(),
		}, nil
	}
	if !info.IsDir() {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  "path is not a directory",
		}, nil
	}

	dbPath := mainDBPath(absPath)
	dbInfo, err := os.Stat(dbPath)
	if err != nil || dbInfo.IsDir() {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  "missing .pixcall/database/main.db",
		}, nil
	}

	db, closer, err := kit.OpenReadOnly(dbPath)
	if err != nil {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  err.Error(),
		}, nil
	}
	defer closer.Close()

	var entryCount int64
	if err := db.WithContext(ctx).Model(&model.Entry{}).Limit(1).Count(&entryCount).Error; err != nil {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  "invalid pixcall entries table",
		}, nil
	}

	var kvs []model.Kvs
	if err := db.WithContext(ctx).Where("k = ?", "schema_version").Limit(1).Find(&kvs).Error; err != nil {
		return format.ProbeResult{
			Matched: false,
			Format:  formatID,
			Reason:  "invalid pixcall kvs table",
		}, nil
	}

	schemaVersion := ""
	if len(kvs) > 0 {
		schemaVersion = strings.TrimSpace(kvs[0].V)
	}

	confidence := 90
	if schemaVersion == model.SchemaVersion {
		confidence = 100
	}

	_, cfgErr := os.Stat(filepath.Join(absPath, ".pixcall", "config.json"))
	_, keystoreErr := os.Stat(filepath.Join(absPath, ".pixcall", "keystore"))
	details := map[string]any{
		"dbPath":      dbPath,
		"hasConfig":   cfgErr == nil,
		"hasKeystore": keystoreErr == nil,
	}
	if schemaVersion != "" {
		details["schemaVersion"] = schemaVersion
	}

	return format.ProbeResult{
		Matched:    true,
		Format:     formatID,
		Version:    model.AppVersionForSchema(schemaVersion),
		RootPath:   absPath,
		Name:       deriveLibraryName(absPath),
		Confidence: confidence,
		Details:    details,
	}, nil
}

func mainDBPath(rootPath string) string {
	return filepath.Join(rootPath, ".pixcall", "database", "main.db")
}

func thumbsDBPath(rootPath string) string {
	return filepath.Join(rootPath, ".pixcall", "database", "thumbs.db")
}
