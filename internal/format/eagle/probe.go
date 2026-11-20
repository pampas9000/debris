package eagle

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"debris/internal/format"
	"debris/internal/format/eagle/model"
)

const (
	// FormatID is the adapter identifier for Eagle libraries.
	FormatID = "eagle"
	// DefaultAppVersion is the Eagle metadata version written by this adapter.
	DefaultAppVersion = "4.0.0"
)

// Adapter implements format.Adapter for Eagle 4.x libraries.
type Adapter struct{}

// New returns a stateless Eagle format adapter.
func New() format.Adapter {
	return &Adapter{}
}

// ID returns the unique identifier for the Eagle format.
func (a *Adapter) ID() string {
	return FormatID
}

// DisplayName returns the human-readable name for the Eagle format.
func (a *Adapter) DisplayName() string {
	return "Eagle"
}

// Capabilities reports the metadata and structural features supported by Eagle.
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

// Probe inspects path to determine whether it is an Eagle library directory.
func (a *Adapter) Probe(ctx context.Context, path string) (format.ProbeResult, error) {
	if err := ctx.Err(); err != nil {
		return format.ProbeResult{}, err
	}

	res := format.ProbeResult{
		Format: FormatID,
	}

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
		return res, fmt.Errorf("eagle: stat %q: %w", rootPath, err)
	}
	if !info.IsDir() {
		res.Reason = "path is not a directory"
		return res, nil
	}

	metaPath := filepath.Join(rootPath, "metadata.json")
	metaInfo, err := os.Stat(metaPath)
	if err != nil || metaInfo.IsDir() {
		res.Reason = "missing metadata.json"
		return res, nil
	}

	imagesDir := filepath.Join(rootPath, "images")
	imagesInfo, err := os.Stat(imagesDir)
	if err != nil || !imagesInfo.IsDir() {
		res.Reason = "missing images directory"
		return res, nil
	}

	raw, err := os.ReadFile(metaPath)
	if err != nil {
		return res, fmt.Errorf("eagle: read metadata.json: %w", err)
	}

	var libMeta model.LibraryMetadata
	if err := json.Unmarshal(raw, &libMeta); err != nil {
		res.Reason = "invalid metadata.json"
		return res, nil
	}

	confidence := 90
	if libMeta.ApplicationVersion != "" {
		confidence = 100
	}

	res.Matched = true
	res.Version = libMeta.ApplicationVersion
	res.RootPath = rootPath
	res.Name = libraryNameFromPath(rootPath)
	res.Confidence = confidence
	res.Details = map[string]any{
		"applicationVersion": libMeta.ApplicationVersion,
		"folderCount":        countModelFolders(libMeta.Folders),
		"tagGroupCount":      len(libMeta.TagsGroups),
	}
	return res, nil
}

// OpenReader opens an Eagle library at path for inspection and reading.
func (a *Adapter) OpenReader(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("eagle: reader path is required")
	}

	rootPath := resolvePath(trimmed)
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("eagle: open reader %q: %w", rootPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("eagle: %q is not a directory", rootPath)
	}

	metaPath := filepath.Join(rootPath, "metadata.json")
	if metaInfo, err := os.Stat(metaPath); err != nil || metaInfo.IsDir() {
		return nil, fmt.Errorf("eagle: %q missing metadata.json", rootPath)
	}
	imagesPath := filepath.Join(rootPath, "images")
	if imgInfo, err := os.Stat(imagesPath); err != nil || !imgInfo.IsDir() {
		return nil, fmt.Errorf("eagle: %q missing images directory", rootPath)
	}

	return &reader{
		rootPath: rootPath,
		opts:     opts,
	}, nil
}

// OpenWriter opens an Eagle library destination path for writing.
func (a *Adapter) OpenWriter(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("eagle: writer path is required")
	}

	mode, err := format.ParseCopyMode(string(opts.CopyMode))
	if err != nil {
		return nil, fmt.Errorf("eagle: %w", err)
	}
	opts.CopyMode = mode

	return &writer{
		rootPath: resolvePath(trimmed),
		opts:     opts,
	}, nil
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
	trimmed := strings.TrimSuffix(base, ".library")
	if trimmed == "" {
		return base
	}
	return trimmed
}

func countModelFolders(folders []model.Folder) int {
	total := 0
	for _, f := range folders {
		total++
		total += countModelFolders(f.Children)
	}
	return total
}
