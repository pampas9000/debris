package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"debris/internal/format"
	"debris/pkg/debris"
)

type capabilityRule struct {
	has  func(format.Capabilities) bool
	loss string
}

var capabilityLossRules = []capabilityRule{
	{
		has:  func(c format.Capabilities) bool { return c.SupportsFolders },
		loss: "folders are not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsNestedFolders },
		loss: "nested folder hierarchy is not supported by target format and may be flattened",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsMultiFolderAsset },
		loss: "multi-folder asset associations are not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsTagGroups },
		loss: "tag groups are not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsNestedTags },
		loss: "nested tag hierarchy is not supported by target format and may be flattened",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsColors },
		loss: "asset color palettes are not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsRecycleBin },
		loss: "recycle bin / deleted item state is not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsThumbnails },
		loss: "custom thumbnails are not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsRating },
		loss: "asset ratings are not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsOriginURL },
		loss: "asset origin URLs are not supported by target format",
	},
	{
		has:  func(c format.Capabilities) bool { return c.SupportsAnnotation },
		loss: "asset annotations are not supported by target format",
	},
}

// BuildPlan validates conversion paths and compares source/target format
// capabilities against inspectRes to produce a projected PlanResponse.
func BuildPlan(
	ctx context.Context,
	srcAdapter format.Adapter,
	dstAdapter format.Adapter,
	inspectRes format.InspectResult,
	req debris.PlanRequest,
) (debris.PlanResponse, error) {
	if err := checkContext(ctx); err != nil {
		return debris.PlanResponse{}, err
	}
	if srcAdapter == nil || dstAdapter == nil {
		return debris.PlanResponse{}, debris.NewError(debris.CodeUnsupportedFormat, "source and target adapters are required", "")
	}
	if strings.TrimSpace(req.InputPath) == "" {
		return debris.PlanResponse{}, debris.NewError(debris.CodeInvalidRequest, "inputPath is required", "")
	}
	if strings.TrimSpace(req.OutputPath) == "" {
		return debris.PlanResponse{}, debris.NewError(debris.CodeInvalidRequest, "outputPath is required", "")
	}
	if samePath(req.InputPath, req.OutputPath) {
		return debris.PlanResponse{}, debris.NewError(debris.CodeInvalidRequest, "inputPath and outputPath must be different", req.OutputPath)
	}

	if err := validateTargetPath(req.OutputPath, req.Overwrite); err != nil {
		return debris.PlanResponse{}, err
	}

	parsedMode, err := format.ParseCopyMode(string(req.CopyMode))
	if err != nil {
		return debris.PlanResponse{}, debris.NewError(debris.CodeInvalidRequest, "invalid copyMode", err.Error())
	}
	copyMode := debris.CopyMode(parsedMode)

	srcCaps := srcAdapter.Capabilities()
	dstCaps := dstAdapter.Capabilities()

	var losses []string
	for _, rule := range capabilityLossRules {
		if rule.has(srcCaps) && !rule.has(dstCaps) {
			losses = append(losses, rule.loss)
		}
	}

	warnings := append([]string(nil), inspectRes.Warnings...)
	if !dstCaps.SupportsFolders && inspectRes.FolderCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d folder(s) will not be preserved in %s", inspectRes.FolderCount, dstAdapter.DisplayName()))
	}
	if !dstCaps.SupportsTagGroups && inspectRes.TagGroupCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d tag group(s) will not be preserved in %s", inspectRes.TagGroupCount, dstAdapter.DisplayName()))
	}
	if !dstCaps.SupportsRecycleBin && req.IncludeDeleted && inspectRes.DeletedCount > 0 {
		warnings = append(warnings, fmt.Sprintf("%d deleted item(s) cannot remain in a recycle bin in %s", inspectRes.DeletedCount, dstAdapter.DisplayName()))
	}
	if strings.EqualFold(srcAdapter.ID(), dstAdapter.ID()) {
		warnings = append(warnings, fmt.Sprintf("source and target formats are both %s", srcAdapter.ID()))
	}

	switch copyMode {
	case debris.CopyModeHardlink:
		warnings = append(warnings, fmt.Sprintf(
			"hardlink mode: estimated additional media disk usage is ~0 bytes (vs %d bytes in copy mode); input and output must reside on the same filesystem",
			inspectRes.TotalBytes,
		))
	case debris.CopyModeAuto:
		warnings = append(warnings, fmt.Sprintf(
			"auto copy mode: hardlinks avoid duplicating %d bytes on the same filesystem, or falls back to copy across filesystems",
			inspectRes.TotalBytes,
		))
	}

	sourceFormat := inspectRes.Format
	if sourceFormat == "" {
		sourceFormat = srcAdapter.ID()
	}

	return debris.PlanResponse{
		SourceFormat:  sourceFormat,
		TargetFormat:  dstAdapter.ID(),
		InputPath:     req.InputPath,
		OutputPath:    req.OutputPath,
		CopyMode:      copyMode,
		FolderCount:   inspectRes.FolderCount,
		TagGroupCount: inspectRes.TagGroupCount,
		TagCount:      inspectRes.TagCount,
		AssetCount:    inspectRes.AssetCount,
		TotalBytes:    inspectRes.TotalBytes,
		Warnings:      warnings,
		Losses:        losses,
	}, nil
}

func validateTargetPath(outputPath string, overwrite bool) error {
	info, err := os.Stat(outputPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return wrapEngineError(err, debris.CodeIO, "failed to inspect output path")
	}
	if !overwrite {
		return debris.NewError(debris.CodeInvalidRequest, "target output path already exists (set overwrite=true to replace)", outputPath)
	}
	if !info.IsDir() {
		return debris.NewError(debris.CodeInvalidRequest, "target output path exists and is not a directory", outputPath)
	}
	return nil
}

func samePath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	normA := canonicalPath(a)
	normB := canonicalPath(b)
	if normA == normB {
		return true
	}
	infoA, errA := os.Stat(a)
	infoB, errB := os.Stat(b)
	if errA == nil && errB == nil && os.SameFile(infoA, infoB) {
		return true
	}
	return false
}

func canonicalPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved)
	}
	parent := filepath.Dir(abs)
	if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
		return filepath.Clean(filepath.Join(resolvedParent, filepath.Base(abs)))
	}
	return filepath.Clean(abs)
}
