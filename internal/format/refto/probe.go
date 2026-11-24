package refto

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"debris/internal/format"
	"debris/internal/format/refto/model"
	"debris/internal/kit"
)

const (
	// FormatID is the canonical adapter identifier for Refto libraries.
	FormatID = "refto"
	// DisplayName is the human-readable name for the Refto format.
	DisplayName = "Refto"
)

// Adapter implements format.Adapter for Refto (.reflib, schema v25) libraries.
type Adapter struct{}

// New returns a stateless Refto format.Adapter.
func New() format.Adapter {
	return &Adapter{}
}

// ID returns "refto".
func (a *Adapter) ID() string {
	return FormatID
}

// DisplayName returns "Refto".
func (a *Adapter) DisplayName() string {
	return DisplayName
}

// Capabilities reports the structural and metadata features supported by Refto.
func (a *Adapter) Capabilities() format.Capabilities {
	return format.Capabilities{
		SupportsFolders:          true,
		SupportsNestedFolders:    true,
		SupportsMultiFolderAsset: true,
		SupportsTagGroups:        true,
		SupportsNestedTags:       false,
		SupportsColors:           true,
		SupportsRecycleBin:       true,
		SupportsThumbnails:       true,
		SupportsRating:           true,
		SupportsOriginURL:        true,
		SupportsAnnotation:       true,
	}
}

// Probe inspects path to determine whether it is a valid Refto (.reflib) library directory.
func (a *Adapter) Probe(ctx context.Context, path string) (format.ProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return format.ProbeResult{}, err
	}

	res := format.ProbeResult{Format: FormatID}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		res.Reason = "empty path"
		return res, nil
	}

	rootPath := resolvePath(trimmed)
	info, err := os.Stat(rootPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			res.Reason = "path does not exist"
			return res, nil
		}
		return res, fmt.Errorf("refto: stat %q: %w", rootPath, err)
	}
	if !info.IsDir() {
		res.Reason = "path is not a directory"
		return res, nil
	}

	dbPath := mainDBPath(rootPath)
	dbInfo, err := os.Stat(dbPath)
	if err != nil || dbInfo.IsDir() {
		res.Reason = "missing act.db"
		return res, nil
	}

	db, closer, err := kit.OpenReadOnly(dbPath)
	if err != nil {
		res.Reason = fmt.Sprintf("open act.db: %v", err)
		return res, nil
	}
	defer closer.Close()

	var itemCount int64
	if err := db.WithContext(ctx).Model(&model.Item{}).Limit(1).Count(&itemCount).Error; err != nil {
		if ctx.Err() != nil {
			return format.ProbeResult{}, ctx.Err()
		}
		res.Reason = "invalid refto items table"
		return res, nil
	}

	var userVersion int
	_ = db.WithContext(ctx).Raw("PRAGMA user_version").Row().Scan(&userVersion)

	var manifest model.LibraryManifest
	hasManifest := false
	manifestPath := filepath.Join(rootPath, "act-library.json")
	if raw, err := os.ReadFile(manifestPath); err == nil {
		if uErr := json.Unmarshal(raw, &manifest); uErr == nil {
			hasManifest = true
		}
	}

	confidence := 90
	if hasManifest {
		confidence = 100
	}

	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = libraryNameFromPath(rootPath)
	}

	version := strings.TrimSpace(manifest.AppVersion)
	if version == "" && userVersion > 0 {
		version = strconv.Itoa(userVersion)
	}

	details := map[string]any{
		"dbPath":      dbPath,
		"userVersion": userVersion,
		"hasManifest": hasManifest,
	}
	if manifest.Schema > 0 {
		details["manifestSchema"] = manifest.Schema
	}
	if manifest.Storage != "" {
		details["storage"] = manifest.Storage
	}

	res.Matched = true
	res.Version = version
	res.RootPath = rootPath
	res.Name = name
	res.Confidence = confidence
	res.Reason = "detected Refto act.db"
	res.Details = details
	return res, nil
}

// OpenReader opens a Refto library at path in strict read-only mode.
func (a *Adapter) OpenReader(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
	return newReader(ctx, path, opts)
}

// OpenWriter opens a Refto library destination path for writing.
func (a *Adapter) OpenWriter(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
	return newWriter(ctx, path, opts)
}

func mainDBPath(rootPath string) string {
	return filepath.Join(rootPath, "act.db")
}

func resolvePath(path string) string {
	cleaned := filepath.Clean(path)
	if abs, err := filepath.Abs(cleaned); err == nil {
		return abs
	}
	return cleaned
}

func libraryNameFromPath(rootPath string) string {
	base := filepath.Base(rootPath)
	for _, ext := range []string{".reflib", ".refto", ".REFLIB", ".REFTO"} {
		if strings.HasSuffix(base, ext) && len(base) > len(ext) {
			return strings.TrimSuffix(base, ext)
		}
	}
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}
