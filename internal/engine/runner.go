package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/pkg/debris"
)

var _ debris.Runner = (*Runner)(nil)

// Runner implements debris.Runner using a Registry of format.Adapter instances.
type Runner struct {
	reg *Registry
}

// NewRunner creates a Runner backed by reg. If reg is nil, an empty Registry is used.
func NewRunner(reg *Registry) *Runner {
	if reg == nil {
		reg = NewRegistry()
	}
	return &Runner{reg: reg}
}

// Formats returns all registered library formats and their capabilities.
func (r *Runner) Formats() []debris.FormatSpec {
	if r == nil || r.reg == nil {
		return nil
	}
	return r.reg.List()
}

// Probe detects or verifies the format of the library at req.Path.
func (r *Runner) Probe(ctx context.Context, req debris.ProbeRequest) (debris.ProbeResponse, error) {
	if err := checkContext(ctx); err != nil {
		return debris.ProbeResponse{}, err
	}
	if strings.TrimSpace(req.Path) == "" {
		return debris.ProbeResponse{}, debris.NewError(debris.CodeInvalidRequest, "probe path is required", "")
	}

	if strings.TrimSpace(req.Format) == "" {
		adapter, res, err := r.reg.Detect(ctx, req.Path)
		if err != nil {
			return debris.ProbeResponse{}, err
		}
		if res.Format == "" && adapter != nil {
			res.Format = adapter.ID()
		}
		return toProbeResponse(res, req.Path), nil
	}

	if err := ensurePathExists(req.Path); err != nil {
		return debris.ProbeResponse{}, err
	}
	adapter, ok := r.reg.Get(req.Format)
	if !ok {
		return debris.ProbeResponse{}, debris.NewError(debris.CodeUnsupportedFormat, "unsupported library format", req.Format)
	}
	res, err := adapter.Probe(ctx, req.Path)
	if err != nil {
		return debris.ProbeResponse{}, wrapEngineError(err, debris.CodeProbeFailed, "probe failed")
	}
	if res.Matched && res.Format == "" {
		res.Format = adapter.ID()
	}
	return toProbeResponse(res, req.Path), nil
}

// Inspect opens the source library (auto-detecting if req.Format is empty) and
// returns a lightweight summary without materializing files.
func (r *Runner) Inspect(ctx context.Context, req debris.InspectRequest) (resp debris.InspectResponse, retErr error) {
	if err := checkContext(ctx); err != nil {
		return debris.InspectResponse{}, err
	}
	if strings.TrimSpace(req.Path) == "" {
		return debris.InspectResponse{}, debris.NewError(debris.CodeInvalidRequest, "inspect path is required", "")
	}

	adapter, _, err := r.resolveSourceAdapter(ctx, req.Format, req.Path)
	if err != nil {
		return debris.InspectResponse{}, err
	}

	opts := format.ReadOptions{
		IncludeDeleted: req.IncludeDeleted,
		IncludeHidden:  req.IncludeHidden,
		SkipMediaProbe: true,
	}
	reader, err := adapter.OpenReader(ctx, req.Path, opts)
	if err != nil {
		return debris.InspectResponse{}, wrapEngineError(err, debris.CodeIO, "open source library")
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && retErr == nil {
			retErr = wrapEngineError(closeErr, debris.CodeIO, "close source library")
		}
	}()

	res, err := reader.Inspect(ctx)
	if err != nil {
		return debris.InspectResponse{}, wrapEngineError(err, debris.CodeIO, "inspect source library")
	}
	if res.Format == "" {
		res.Format = adapter.ID()
	}
	if res.RootPath == "" {
		res.RootPath = req.Path
	}

	return debris.InspectResponse{
		Format:        res.Format,
		Version:       res.Version,
		RootPath:      res.RootPath,
		Name:          res.Name,
		FolderCount:   res.FolderCount,
		TagGroupCount: res.TagGroupCount,
		TagCount:      res.TagCount,
		AssetCount:    res.AssetCount,
		DeletedCount:  res.DeletedCount,
		HiddenCount:   res.HiddenCount,
		TotalBytes:    res.TotalBytes,
		Warnings:      append([]string(nil), res.Warnings...),
		Extra:         res.Extra,
	}, nil
}

// Plan inspects the source library and builds a conversion plan against the target format.
func (r *Runner) Plan(ctx context.Context, req debris.PlanRequest) (resp debris.PlanResponse, retErr error) {
	if err := checkContext(ctx); err != nil {
		return debris.PlanResponse{}, err
	}
	if strings.TrimSpace(req.To) == "" {
		return debris.PlanResponse{}, debris.NewError(debris.CodeInvalidRequest, "target format (to) is required", "")
	}
	if strings.TrimSpace(req.InputPath) == "" {
		return debris.PlanResponse{}, debris.NewError(debris.CodeInvalidRequest, "inputPath is required", "")
	}
	if strings.TrimSpace(req.OutputPath) == "" {
		return debris.PlanResponse{}, debris.NewError(debris.CodeInvalidRequest, "outputPath is required", "")
	}

	srcAdapter, _, err := r.resolveSourceAdapter(ctx, req.From, req.InputPath)
	if err != nil {
		return debris.PlanResponse{}, err
	}
	dstAdapter, ok := r.reg.Get(req.To)
	if !ok {
		return debris.PlanResponse{}, debris.NewError(debris.CodeUnsupportedFormat, "unsupported target format", req.To)
	}

	opts := format.ReadOptions{
		IncludeDeleted: req.IncludeDeleted,
		IncludeHidden:  req.IncludeHidden,
		SkipMediaProbe: true,
	}
	reader, err := srcAdapter.OpenReader(ctx, req.InputPath, opts)
	if err != nil {
		return debris.PlanResponse{}, wrapEngineError(err, debris.CodeIO, "open source library")
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && retErr == nil {
			retErr = wrapEngineError(closeErr, debris.CodeIO, "close source library")
		}
	}()

	inspectRes, err := reader.Inspect(ctx)
	if err != nil {
		return debris.PlanResponse{}, wrapEngineError(err, debris.CodeIO, "inspect source library")
	}
	if inspectRes.Format == "" {
		inspectRes.Format = srcAdapter.ID()
	}
	if inspectRes.RootPath == "" {
		inspectRes.RootPath = req.InputPath
	}

	return BuildPlan(ctx, srcAdapter, dstAdapter, inspectRes, req)
}

// Convert reads the source library, filters items according to req, and
// writes the target library while streaming progress events.
func (r *Runner) Convert(ctx context.Context, req debris.ConvertRequest, emit debris.EventCallback) (resp debris.ConvertResponse, retErr error) {
	start := time.Now()

	if err := checkContext(ctx); err != nil {
		return debris.ConvertResponse{}, err
	}
	if strings.TrimSpace(req.To) == "" {
		return debris.ConvertResponse{}, debris.NewError(debris.CodeInvalidRequest, "target format (to) is required", "")
	}
	if strings.TrimSpace(req.InputPath) == "" {
		return debris.ConvertResponse{}, debris.NewError(debris.CodeInvalidRequest, "inputPath is required", "")
	}
	if strings.TrimSpace(req.OutputPath) == "" {
		return debris.ConvertResponse{}, debris.NewError(debris.CodeInvalidRequest, "outputPath is required", "")
	}
	if samePath(req.InputPath, req.OutputPath) {
		return debris.ConvertResponse{}, debris.NewError(debris.CodeInvalidRequest, "inputPath and outputPath must be different", req.OutputPath)
	}

	parsedMode, err := format.ParseCopyMode(string(req.CopyMode))
	if err != nil {
		return debris.ConvertResponse{}, debris.NewError(debris.CodeInvalidRequest, "invalid copyMode", err.Error())
	}
	copyMode := debris.CopyMode(parsedMode)

	srcAdapter, _, err := r.resolveSourceAdapter(ctx, req.From, req.InputPath)
	if err != nil {
		return debris.ConvertResponse{}, err
	}
	dstAdapter, ok := r.reg.Get(req.To)
	if !ok {
		return debris.ConvertResponse{}, debris.NewError(debris.CodeUnsupportedFormat, "unsupported target format", req.To)
	}

	if err := validateTargetPath(req.OutputPath, req.Overwrite); err != nil {
		return debris.ConvertResponse{}, err
	}

	if emit != nil {
		emit(debris.Event{
			Kind:    "stage",
			Stage:   "read",
			Message: "reading source library",
		})
	}

	lib, err := r.readSourceLibrary(ctx, srcAdapter, req)
	if err != nil {
		return debris.ConvertResponse{}, err
	}

	filtered := filterLibrary(lib, req.IncludeDeleted, req.IncludeHidden, req.LibraryName)
	folderCount := countFolders(filtered.Folders)
	tagGroupCount := len(filtered.TagGroups)
	tagCount := len(filtered.Tags)
	assetCount := len(filtered.Assets)
	totalBytes := sumAssetBytes(filtered.Assets)

	resp = debris.ConvertResponse{
		SourceFormat:  srcAdapter.ID(),
		TargetFormat:  dstAdapter.ID(),
		InputPath:     req.InputPath,
		OutputPath:    req.OutputPath,
		CopyMode:      copyMode,
		FolderCount:   folderCount,
		TagGroupCount: tagGroupCount,
		TagCount:      tagCount,
		AssetCount:    assetCount,
		TotalBytes:    totalBytes,
	}

	if err := os.MkdirAll(req.OutputPath, 0o755); err != nil {
		return debris.ConvertResponse{}, wrapEngineError(err, debris.CodeIO, "prepare target output directory")
	}

	if emit != nil {
		emit(debris.Event{
			Kind:    "stage",
			Stage:   "write",
			Total:   int64(assetCount),
			Bytes:   totalBytes,
			Message: "writing target library",
		})
	}

	writeOpts := format.WriteOptions{
		CopyMode:       parsedMode,
		Overwrite:      req.Overwrite,
		IncludeDeleted: req.IncludeDeleted,
		IncludeHidden:  req.IncludeHidden,
		LibraryName:    req.LibraryName,
	}

	tracker := newEventTracker(emit)
	if err := r.writeTargetLibrary(ctx, dstAdapter, req.OutputPath, writeOpts, filtered, tracker.handle); err != nil {
		return debris.ConvertResponse{}, err
	}

	copied, hardlinked, skipped, warnings := tracker.summary()
	resp.CopiedAssets = copied
	resp.HardlinkedAssets = hardlinked
	resp.SkippedAssets = skipped
	resp.Warnings = warnings
	resp.DurationMs = time.Since(start).Milliseconds()
	return resp, nil
}

func (r *Runner) resolveSourceAdapter(ctx context.Context, explicitFormat, path string) (format.Adapter, format.ProbeResult, error) {
	if strings.TrimSpace(explicitFormat) != "" {
		if err := ensurePathExists(path); err != nil {
			return nil, format.ProbeResult{}, err
		}
		adapter, ok := r.reg.Get(explicitFormat)
		if !ok {
			return nil, format.ProbeResult{}, debris.NewError(debris.CodeUnsupportedFormat, "unsupported source format", explicitFormat)
		}
		return adapter, format.ProbeResult{Matched: true, Format: adapter.ID(), RootPath: path}, nil
	}
	return r.reg.Detect(ctx, path)
}

func (r *Runner) readSourceLibrary(ctx context.Context, srcAdapter format.Adapter, req debris.ConvertRequest) (lib *domain.Library, retErr error) {
	readOpts := format.ReadOptions{
		IncludeDeleted: req.IncludeDeleted,
		IncludeHidden:  req.IncludeHidden,
	}
	reader, err := srcAdapter.OpenReader(ctx, req.InputPath, readOpts)
	if err != nil {
		return nil, wrapEngineError(err, debris.CodeIO, "open source library")
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && retErr == nil {
			retErr = wrapEngineError(closeErr, debris.CodeIO, "close source library")
		}
	}()

	lib, err = reader.Read(ctx)
	if err != nil {
		return nil, wrapEngineError(err, debris.CodeIO, "read source library")
	}
	if lib == nil {
		lib = &domain.Library{
			Format:   srcAdapter.ID(),
			RootPath: req.InputPath,
		}
	}
	return lib, nil
}

func (r *Runner) writeTargetLibrary(
	ctx context.Context,
	dstAdapter format.Adapter,
	outputPath string,
	writeOpts format.WriteOptions,
	lib *domain.Library,
	emit format.ProgressCallback,
) (retErr error) {
	writer, err := dstAdapter.OpenWriter(ctx, outputPath, writeOpts)
	if err != nil {
		return wrapEngineError(err, debris.CodeIO, "open target library writer")
	}
	defer func() {
		if closeErr := writer.Close(); closeErr != nil && retErr == nil {
			retErr = wrapEngineError(closeErr, debris.CodeIO, "close target library writer")
		}
	}()

	if err := writer.Write(ctx, lib, emit); err != nil {
		return wrapEngineError(err, debris.CodeIO, "write target library")
	}
	return nil
}

func filterLibrary(lib *domain.Library, includeDeleted, includeHidden bool, libraryName string) *domain.Library {
	if lib == nil {
		out := &domain.Library{}
		if libraryName != "" {
			out.Name = libraryName
		}
		return out
	}

	out := *lib
	if libraryName != "" {
		out.Name = libraryName
	}

	out.Folders = filterFolders(lib.Folders, includeDeleted, includeHidden)

	if len(lib.Assets) > 0 {
		assets := make([]*domain.Asset, 0, len(lib.Assets))
		for _, a := range lib.Assets {
			if a == nil {
				continue
			}
			if !includeDeleted && a.IsDeleted {
				continue
			}
			if !includeHidden && a.IsHidden {
				continue
			}
			assets = append(assets, a)
		}
		out.Assets = assets
	}

	return &out
}

func filterFolders(folders []*domain.Folder, includeDeleted, includeHidden bool) []*domain.Folder {
	if len(folders) == 0 {
		return nil
	}
	kept := make([]*domain.Folder, 0, len(folders))

	for _, f := range folders {
		if f == nil {
			continue
		}
		if !includeDeleted && f.IsRecycle {
			continue
		}
		if !includeHidden && f.IsHidden {
			continue
		}
		cp := *f
		cp.Children = filterFolders(f.Children, includeDeleted, includeHidden)
		if len(f.ChildIDs) > 0 && len(cp.Children) > 0 {
			childIDSet := make(map[string]struct{}, len(cp.Children))
			for _, c := range cp.Children {
				if c.ID != "" {
					childIDSet[c.ID] = struct{}{}
				}
			}
			filteredChildIDs := make([]string, 0, len(f.ChildIDs))
			for _, cid := range f.ChildIDs {
				if _, ok := childIDSet[cid]; ok {
					filteredChildIDs = append(filteredChildIDs, cid)
				}
			}
			cp.ChildIDs = filteredChildIDs
		} else if len(f.ChildIDs) > 0 && len(cp.Children) == 0 && len(f.Children) > 0 {
			cp.ChildIDs = nil
		}
		kept = append(kept, &cp)
	}
	return kept
}

func countFolders(folders []*domain.Folder) int {
	if len(folders) == 0 {
		return 0
	}
	seenIDs := make(map[string]struct{})
	seenPtrs := make(map[*domain.Folder]struct{})
	var walk func([]*domain.Folder) int
	walk = func(list []*domain.Folder) int {
		total := 0
		for _, f := range list {
			if f == nil {
				continue
			}
			if f.ID != "" {
				if _, ok := seenIDs[f.ID]; ok {
					continue
				}
				seenIDs[f.ID] = struct{}{}
			} else {
				if _, ok := seenPtrs[f]; ok {
					continue
				}
				seenPtrs[f] = struct{}{}
			}
			total++
			if len(f.Children) > 0 {
				total += walk(f.Children)
			}
		}
		return total
	}
	return walk(folders)
}

func sumAssetBytes(assets []*domain.Asset) int64 {
	var total int64
	for _, a := range assets {
		if a != nil && a.Size > 0 {
			total += a.Size
		}
	}
	return total
}

type eventTracker struct {
	emit debris.EventCallback

	mu          sync.Mutex
	assetModes  map[string]string
	unkeyedCopy int
	unkeyedLink int
	unkeyedSkip int
	warnings    []string
}

func newEventTracker(emit debris.EventCallback) *eventTracker {
	return &eventTracker{
		emit:       emit,
		assetModes: make(map[string]string),
	}
}

func (t *eventTracker) handle(ev format.ProgressEvent) {
	t.mu.Lock()
	mode := strings.ToLower(strings.TrimSpace(ev.CopyMode))
	switch ev.Kind {
	case "progress":
		switch mode {
		case string(debris.CopyModeCopy), string(debris.CopyModeHardlink), "skipped", "skip":
			if mode == "skip" {
				mode = "skipped"
			}
			if ev.AssetID != "" {
				t.assetModes[ev.AssetID] = mode
			} else {
				switch mode {
				case string(debris.CopyModeCopy):
					t.unkeyedCopy++
				case string(debris.CopyModeHardlink):
					t.unkeyedLink++
				case "skipped":
					t.unkeyedSkip++
				}
			}
		}
	case "warning":
		if ev.Message != "" {
			t.warnings = append(t.warnings, ev.Message)
		}
		if mode == "skipped" || mode == "skip" || strings.EqualFold(ev.Stage, "skip") || strings.EqualFold(ev.Stage, "skipped") {
			if ev.AssetID != "" {
				t.assetModes[ev.AssetID] = "skipped"
			} else {
				t.unkeyedSkip++
			}
		}
	}
	t.mu.Unlock()

	if t.emit != nil {
		t.emit(debris.Event{
			Kind:     ev.Kind,
			Stage:    ev.Stage,
			Current:  ev.Current,
			Total:    ev.Total,
			Bytes:    ev.Bytes,
			AssetID:  ev.AssetID,
			CopyMode: ev.CopyMode,
			Message:  ev.Message,
		})
	}
}

func (t *eventTracker) summary() (copied, hardlinked, skipped int, warnings []string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	copied = t.unkeyedCopy
	hardlinked = t.unkeyedLink
	skipped = t.unkeyedSkip
	for _, mode := range t.assetModes {
		switch mode {
		case string(debris.CopyModeCopy):
			copied++
		case string(debris.CopyModeHardlink):
			hardlinked++
		case "skipped":
			skipped++
		}
	}
	if len(t.warnings) > 0 {
		warnings = append([]string(nil), t.warnings...)
	}
	return copied, hardlinked, skipped, warnings
}

func toProbeResponse(res format.ProbeResult, fallbackPath string) debris.ProbeResponse {
	rootPath := res.RootPath
	if rootPath == "" {
		rootPath = fallbackPath
	}
	return debris.ProbeResponse{
		Matched:    res.Matched,
		Format:     res.Format,
		Version:    res.Version,
		RootPath:   rootPath,
		Name:       res.Name,
		Confidence: res.Confidence,
		Reason:     res.Reason,
		Details:    res.Details,
	}
}

func ensurePathExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return debris.WrapError(debris.CodeNotFound, "library path does not exist", err)
		}
		return debris.WrapError(debris.CodeIO, "failed to access library path", err)
	}
	return nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return debris.NewError(debris.CodeInvalidRequest, "context must not be nil", "")
	}
	if err := ctx.Err(); err != nil {
		return debris.WrapError(debris.CodeCanceled, "operation canceled", err)
	}
	return nil
}

func wrapEngineError(err error, defaultCode, message string) error {
	if err == nil {
		return nil
	}
	if structured, ok := errors.AsType[*debris.Error](err); ok {
		return structured
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return debris.WrapError(debris.CodeCanceled, message, err)
	}
	if errors.Is(err, os.ErrNotExist) {
		return debris.WrapError(debris.CodeNotFound, message, err)
	}
	var pathErr *os.PathError
	var linkErr *os.LinkError
	if errors.Is(err, os.ErrPermission) || errors.As(err, &pathErr) || errors.As(err, &linkErr) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return debris.WrapError(debris.CodeIO, message, err)
	}
	return debris.WrapError(defaultCode, message, err)
}
