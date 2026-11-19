package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/pkg/debris"
)

type mockAdapter struct {
	id           string
	displayName  string
	capabilities format.Capabilities
	probeFn      func(ctx context.Context, path string) (format.ProbeResult, error)
	openReaderFn func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error)
	openWriterFn func(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error)
}

func (m *mockAdapter) ID() string                        { return m.id }
func (m *mockAdapter) DisplayName() string               { return m.displayName }
func (m *mockAdapter) Capabilities() format.Capabilities { return m.capabilities }
func (m *mockAdapter) Probe(ctx context.Context, path string) (format.ProbeResult, error) {
	if m.probeFn != nil {
		return m.probeFn(ctx, path)
	}
	return format.ProbeResult{}, nil
}
func (m *mockAdapter) OpenReader(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
	if m.openReaderFn != nil {
		return m.openReaderFn(ctx, path, opts)
	}
	return nil, errors.New("not implemented")
}
func (m *mockAdapter) OpenWriter(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
	if m.openWriterFn != nil {
		return m.openWriterFn(ctx, path, opts)
	}
	return nil, errors.New("not implemented")
}

type mockReader struct {
	inspectFn func(ctx context.Context) (format.InspectResult, error)
	readFn    func(ctx context.Context) (*domain.Library, error)
	closeFn   func() error
	closed    bool
}

func (r *mockReader) Inspect(ctx context.Context) (format.InspectResult, error) {
	if r.inspectFn != nil {
		return r.inspectFn(ctx)
	}
	return format.InspectResult{}, nil
}

func (r *mockReader) Read(ctx context.Context) (*domain.Library, error) {
	if r.readFn != nil {
		return r.readFn(ctx)
	}
	return &domain.Library{}, nil
}

func (r *mockReader) Close() error {
	r.closed = true
	if r.closeFn != nil {
		return r.closeFn()
	}
	return nil
}

type mockWriter struct {
	writeFn func(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error
	closeFn func() error
	closed  bool
}

func (w *mockWriter) Write(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
	if w.writeFn != nil {
		return w.writeFn(ctx, lib, emit)
	}
	return nil
}

func (w *mockWriter) Close() error {
	w.closed = true
	if w.closeFn != nil {
		return w.closeFn()
	}
	return nil
}

func TestRegistryAndFormats(t *testing.T) {
	eagleCaps := format.Capabilities{
		SupportsFolders:       true,
		SupportsNestedFolders: true,
		SupportsRating:        true,
	}
	billfishCaps := format.Capabilities{
		SupportsFolders:   true,
		SupportsTagGroups: true,
	}

	reg := NewRegistry(
		nil,
		&mockAdapter{id: "eagle", displayName: "Eagle", capabilities: eagleCaps},
		&mockAdapter{id: "billfish", displayName: "Billfish", capabilities: billfishCaps},
	)
	runner := NewRunner(reg)

	specs := runner.Formats()
	if len(specs) != 2 {
		t.Fatalf("Formats() len = %d, want 2", len(specs))
	}
	if specs[0].ID != "eagle" || specs[0].DisplayName != "Eagle" || !specs[0].Capabilities.SupportsNestedFolders {
		t.Fatalf("unexpected first format spec: %#v", specs[0])
	}
	if specs[1].ID != "billfish" || !specs[1].Capabilities.SupportsTagGroups {
		t.Fatalf("unexpected second format spec: %#v", specs[1])
	}
	if _, ok := reg.Get("EAGLE"); !ok {
		t.Fatalf("expected case-insensitive Get(\"EAGLE\") to succeed")
	}
}

func TestProbe_ExplicitAutoDetectAndUnmatched(t *testing.T) {
	dir := t.TempDir()

	lowConf := &mockAdapter{
		id:          "billfish",
		displayName: "Billfish",
		probeFn: func(ctx context.Context, path string) (format.ProbeResult, error) {
			return format.ProbeResult{Matched: true, Format: "billfish", Confidence: 40, Reason: "fallback dir match"}, nil
		},
	}
	highConf := &mockAdapter{
		id:          "eagle",
		displayName: "Eagle",
		probeFn: func(ctx context.Context, path string) (format.ProbeResult, error) {
			return format.ProbeResult{Matched: true, Format: "eagle", Confidence: 95, Name: "DesignLib", Reason: "metadata.json found"}, nil
		},
	}
	noMatch := &mockAdapter{
		id:          "pixcall",
		displayName: "Pixcall",
		probeFn: func(ctx context.Context, path string) (format.ProbeResult, error) {
			return format.ProbeResult{Matched: false}, nil
		},
	}

	runner := NewRunner(NewRegistry(lowConf, highConf, noMatch))

	// Auto-detect picks highest confidence.
	res, err := runner.Probe(context.Background(), debris.ProbeRequest{Path: dir})
	if err != nil {
		t.Fatalf("auto-detect Probe failed: %v", err)
	}
	if !res.Matched || res.Format != "eagle" || res.Confidence != 95 || res.Name != "DesignLib" {
		t.Fatalf("unexpected auto-detect probe result: %#v", res)
	}

	// Explicit format uses requested adapter.
	res, err = runner.Probe(context.Background(), debris.ProbeRequest{Path: dir, Format: "pixcall"})
	if err != nil {
		t.Fatalf("explicit Probe failed: %v", err)
	}
	if res.Matched {
		t.Fatalf("expected Matched=false for explicit pixcall probe, got %#v", res)
	}

	// Unsupported explicit format returns CodeUnsupportedFormat.
	if _, err := runner.Probe(context.Background(), debris.ProbeRequest{Path: dir, Format: "unknown"}); debris.AsError(err).Code != debris.CodeUnsupportedFormat {
		t.Fatalf("expected CodeUnsupportedFormat, got %v", err)
	}

	// Missing path returns CodeNotFound.
	missing := filepath.Join(dir, "does-not-exist")
	if _, err := runner.Probe(context.Background(), debris.ProbeRequest{Path: missing}); debris.AsError(err).Code != debris.CodeNotFound {
		t.Fatalf("expected CodeNotFound, got %v", err)
	}

	// Unmatched auto-detect returns CodeProbeFailed.
	unmatchedRunner := NewRunner(NewRegistry(noMatch))
	if _, err := unmatchedRunner.Probe(context.Background(), debris.ProbeRequest{Path: dir}); debris.AsError(err).Code != debris.CodeProbeFailed {
		t.Fatalf("expected CodeProbeFailed when no adapter matches, got %v", err)
	}
}

func TestInspect_ClosesReaderAndPassesOptions(t *testing.T) {
	dir := t.TempDir()
	var gotOpts format.ReadOptions
	reader := &mockReader{
		inspectFn: func(ctx context.Context) (format.InspectResult, error) {
			return format.InspectResult{
				Name:          "Sample",
				FolderCount:   3,
				TagGroupCount: 2,
				TagCount:      10,
				AssetCount:    42,
				DeletedCount:  1,
				HiddenCount:   2,
				TotalBytes:    102400,
				Warnings:      []string{"minor metadata gap"},
			}, nil
		},
	}

	adapter := &mockAdapter{
		id:          "eagle",
		displayName: "Eagle",
		probeFn: func(ctx context.Context, path string) (format.ProbeResult, error) {
			return format.ProbeResult{Matched: true, Confidence: 90}, nil
		},
		openReaderFn: func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
			gotOpts = opts
			return reader, nil
		},
	}

	runner := NewRunner(NewRegistry(adapter))
	resp, err := runner.Inspect(context.Background(), debris.InspectRequest{
		Path:           dir,
		IncludeDeleted: true,
		IncludeHidden:  false,
	})
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if !reader.closed {
		t.Fatalf("expected reader.Close() to be called")
	}
	if !gotOpts.IncludeDeleted || gotOpts.IncludeHidden || !gotOpts.SkipMediaProbe {
		t.Fatalf("unexpected ReadOptions passed to OpenReader: %#v", gotOpts)
	}
	if resp.Format != "eagle" || resp.RootPath != dir || resp.AssetCount != 42 || resp.TotalBytes != 102400 {
		t.Fatalf("unexpected InspectResponse: %#v", resp)
	}

	// Verify reader is also closed on Inspect error.
	errReader := &mockReader{
		inspectFn: func(ctx context.Context) (format.InspectResult, error) {
			return format.InspectResult{}, errors.New("corrupt index")
		},
	}
	adapter.openReaderFn = func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
		return errReader, nil
	}
	if _, err := runner.Inspect(context.Background(), debris.InspectRequest{Path: dir, Format: "eagle"}); err == nil {
		t.Fatalf("expected Inspect error")
	}
	if !errReader.closed {
		t.Fatalf("expected reader.Close() to be called even when Inspect fails")
	}
}

func TestPlan_CapabilityLossesAndTargetChecks(t *testing.T) {
	inDir := t.TempDir()
	existingOut := t.TempDir()
	freshOut := filepath.Join(t.TempDir(), "target.library")

	srcAdapter := &mockAdapter{
		id:          "eagle",
		displayName: "Eagle",
		capabilities: format.Capabilities{
			SupportsFolders:          true,
			SupportsNestedFolders:    true,
			SupportsMultiFolderAsset: true,
			SupportsTagGroups:        true,
			SupportsNestedTags:       true,
			SupportsColors:           true,
			SupportsRecycleBin:       true,
			SupportsThumbnails:       true,
			SupportsRating:           true,
			SupportsOriginURL:        true,
			SupportsAnnotation:       true,
		},
		probeFn: func(ctx context.Context, path string) (format.ProbeResult, error) {
			return format.ProbeResult{Matched: true, Confidence: 90}, nil
		},
		openReaderFn: func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
			return &mockReader{
				inspectFn: func(ctx context.Context) (format.InspectResult, error) {
					return format.InspectResult{
						Format:        "eagle",
						RootPath:      path,
						FolderCount:   4,
						TagGroupCount: 2,
						TagCount:      7,
						AssetCount:    20,
						DeletedCount:  3,
						TotalBytes:    4096,
					}, nil
				},
			}, nil
		},
	}

	dstAdapter := &mockAdapter{
		id:          "minimal",
		displayName: "Minimal",
		capabilities: format.Capabilities{
			SupportsFolders: true,
			SupportsRating:  true,
		},
	}

	runner := NewRunner(NewRegistry(srcAdapter, dstAdapter))

	// Target exists without Overwrite -> error.
	_, err := runner.Plan(context.Background(), debris.PlanRequest{
		To:         "minimal",
		InputPath:  inDir,
		OutputPath: existingOut,
	})
	if err == nil || debris.AsError(err).Code != debris.CodeInvalidRequest {
		t.Fatalf("expected CodeInvalidRequest when output exists without overwrite, got %v", err)
	}

	// InputPath == OutputPath -> error.
	_, err = runner.Plan(context.Background(), debris.PlanRequest{
		To:         "minimal",
		InputPath:  inDir,
		OutputPath: inDir,
		Overwrite:  true,
	})
	if err == nil || debris.AsError(err).Code != debris.CodeInvalidRequest {
		t.Fatalf("expected CodeInvalidRequest when inputPath == outputPath, got %v", err)
	}

	// Valid plan with hardlink mode and Overwrite=true on existing dir.
	plan, err := runner.Plan(context.Background(), debris.PlanRequest{
		To:             "minimal",
		InputPath:      inDir,
		OutputPath:     existingOut,
		CopyMode:       debris.CopyModeHardlink,
		IncludeDeleted: true,
		Overwrite:      true,
	})
	if err != nil {
		t.Fatalf("Plan with overwrite failed: %v", err)
	}
	if plan.SourceFormat != "eagle" || plan.TargetFormat != "minimal" {
		t.Fatalf("unexpected formats in plan: %#v", plan)
	}
	if plan.FolderCount != 4 || plan.TagGroupCount != 2 || plan.TagCount != 7 || plan.AssetCount != 20 || plan.TotalBytes != 4096 {
		t.Fatalf("unexpected counts in plan: %#v", plan)
	}
	// Eagle has 11 capabilities, Minimal has 2 -> 9 losses.
	if len(plan.Losses) != 9 {
		t.Fatalf("expected 9 capability losses, got %d: %#v", len(plan.Losses), plan.Losses)
	}
	joinedWarnings := strings.Join(plan.Warnings, "\n")
	if !strings.Contains(joinedWarnings, "hardlink mode") {
		t.Fatalf("expected hardlink disk estimate warning, got %#v", plan.Warnings)
	}
	if !strings.Contains(joinedWarnings, "tag group") {
		t.Fatalf("expected tag group warning, got %#v", plan.Warnings)
	}

	// Valid plan with fresh output path.
	freshPlan, err := runner.Plan(context.Background(), debris.PlanRequest{
		To:         "minimal",
		InputPath:  inDir,
		OutputPath: freshOut,
		CopyMode:   debris.CopyModeCopy,
	})
	if err != nil {
		t.Fatalf("Plan with fresh output failed: %v", err)
	}
	if freshPlan.CopyMode != debris.CopyModeCopy {
		t.Fatalf("expected CopyModeCopy, got %q", freshPlan.CopyMode)
	}
}

func TestConvert_CopyHardlinkDryRunOverwriteFilterCancelAndEvents(t *testing.T) {
	inDir := t.TempDir()

	buildLib := func() *domain.Library {
		return &domain.Library{
			Format:   "eagle",
			RootPath: inDir,
			Name:     "SourceLib",
			Folders: []*domain.Folder{
				{
					ID:   "f1",
					Name: "Active",
					Children: []*domain.Folder{
						{ID: "f1-hidden", Name: "Secret", IsHidden: true},
						{ID: "f1-sub", Name: "Sub"},
					},
					ChildIDs: []string{"f1-hidden", "f1-sub"},
				},
				{ID: "f-recycle", Name: "Trash", IsRecycle: true},
			},
			TagGroups: []*domain.TagGroup{{ID: "tg1", Name: "Group"}},
			Tags:      []*domain.Tag{{ID: "t1", Name: "Tag1"}},
			Assets: []*domain.Asset{
				{ID: "a1", Name: "keep1", Size: 100},
				{ID: "a2", Name: "keep2", Size: 200},
				{ID: "a3", Name: "deleted", Size: 300, IsDeleted: true},
				{ID: "a4", Name: "hidden", Size: 400, IsHidden: true},
			},
		}
	}

	t.Run("dryRun skips target creation and writer", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "dry-out")
		writerCalled := false
		src := &mockAdapter{
			id: "eagle",
			probeFn: func(ctx context.Context, path string) (format.ProbeResult, error) {
				return format.ProbeResult{Matched: true, Confidence: 100}, nil
			},
			openReaderFn: func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
				return &mockReader{readFn: func(ctx context.Context) (*domain.Library, error) { return buildLib(), nil }}, nil
			},
		}
		dst := &mockAdapter{
			id: "pixcall",
			openWriterFn: func(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
				writerCalled = true
				return &mockWriter{}, nil
			},
		}

		eng := debris.NewEngine(debris.WithRunner(NewRunner(NewRegistry(src, dst))))
		var events []debris.Event
		resp, err := eng.Convert(context.Background(), debris.ConvertRequest{
			To:         "pixcall",
			InputPath:  inDir,
			OutputPath: outDir,
			DryRun:     true,
		}, func(ev debris.Event) {
			events = append(events, ev)
		})
		if err != nil {
			t.Fatalf("dry-run Convert failed: %v", err)
		}
		if writerCalled {
			t.Fatalf("expected writer not to be called during dry-run")
		}
		if _, statErr := os.Stat(outDir); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("expected output directory not to be created during dry-run")
		}
		if !resp.DryRun || resp.AssetCount != 2 || resp.TotalBytes != 300 || resp.FolderCount != 2 {
			t.Fatalf("unexpected dry-run response: %#v", resp)
		}
		if len(events) < 2 {
			t.Fatalf("expected stage events during dry-run, got %#v", events)
		}
	})

	t.Run("copy mode, filtering, overwrite, and event tallying", func(t *testing.T) {
		existingOut := t.TempDir()
		var writtenLib *domain.Library
		var writerOpts format.WriteOptions
		reader := &mockReader{
			readFn: func(ctx context.Context) (*domain.Library, error) {
				return buildLib(), nil
			},
		}
		writer := &mockWriter{
			writeFn: func(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
				writtenLib = lib
				// Simulate streaming progress events including repeated AssetID progress, hardlink, skip, and warning.
				emit(format.ProgressEvent{Kind: "progress", Stage: "assets", AssetID: "a1", CopyMode: "copy", Current: 1, Total: 3, Bytes: 50})
				emit(format.ProgressEvent{Kind: "progress", Stage: "assets", AssetID: "a1", CopyMode: "copy", Current: 1, Total: 3, Bytes: 100})
				emit(format.ProgressEvent{Kind: "progress", Stage: "assets", AssetID: "a2", CopyMode: "hardlink", Current: 2, Total: 3, Bytes: 200})
				emit(format.ProgressEvent{Kind: "warning", Stage: "assets", AssetID: "a3", CopyMode: "skipped", Message: "missing source file for a3"})
				return nil
			},
		}
		src := &mockAdapter{
			id: "eagle",
			openReaderFn: func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
				return reader, nil
			},
		}
		dst := &mockAdapter{
			id: "billfish",
			openWriterFn: func(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
				writerOpts = opts
				return writer, nil
			},
		}

		runner := NewRunner(NewRegistry(src, dst))

		// Without Overwrite, existing output fails fast before opening reader.
		if _, err := runner.Convert(context.Background(), debris.ConvertRequest{
			From:       "eagle",
			To:         "billfish",
			InputPath:  inDir,
			OutputPath: existingOut,
			CopyMode:   debris.CopyModeCopy,
		}, nil); debris.AsError(err).Code != debris.CodeInvalidRequest {
			t.Fatalf("expected CodeInvalidRequest without Overwrite, got %v", err)
		}

		// With Overwrite and IncludeDeleted=true, IncludeHidden=false.
		var events []debris.Event
		resp, err := runner.Convert(context.Background(), debris.ConvertRequest{
			From:           "eagle",
			To:             "billfish",
			InputPath:      inDir,
			OutputPath:     existingOut,
			CopyMode:       debris.CopyModeCopy,
			Overwrite:      true,
			IncludeDeleted: true,
			IncludeHidden:  false,
			LibraryName:    "RenamedLib",
		}, func(ev debris.Event) {
			events = append(events, ev)
		})
		if err != nil {
			t.Fatalf("Convert failed: %v", err)
		}
		if !reader.closed || !writer.closed {
			t.Fatalf("expected reader and writer to be closed (reader=%v writer=%v)", reader.closed, writer.closed)
		}
		if writerOpts.CopyMode != format.CopyModeCopy || !writerOpts.Overwrite || !writerOpts.IncludeDeleted || writerOpts.IncludeHidden {
			t.Fatalf("unexpected write opts: %#v", writerOpts)
		}
		if writtenLib.Name != "RenamedLib" {
			t.Fatalf("expected LibraryName override, got %q", writtenLib.Name)
		}
		// Kept assets: a1, a2, a3 (deleted included, hidden excluded).
		if len(writtenLib.Assets) != 3 || resp.AssetCount != 3 || resp.TotalBytes != 600 {
			t.Fatalf("unexpected filtered assets: len=%d resp=%#v", len(writtenLib.Assets), resp)
		}
		// Kept folders: f1 -> f1-sub, and f-recycle (hidden f1-hidden excluded).
		if resp.FolderCount != 3 || len(writtenLib.Folders) != 2 || len(writtenLib.Folders[0].Children) != 1 {
			t.Fatalf("unexpected filtered folders: resp=%#v folders=%#v", resp, writtenLib.Folders)
		}
		if resp.CopiedAssets != 1 || resp.HardlinkedAssets != 1 || resp.SkippedAssets != 1 {
			t.Fatalf("unexpected asset tallies: copied=%d hardlinked=%d skipped=%d", resp.CopiedAssets, resp.HardlinkedAssets, resp.SkippedAssets)
		}
		if len(resp.Warnings) != 1 || resp.Warnings[0] != "missing source file for a3" {
			t.Fatalf("unexpected warnings: %#v", resp.Warnings)
		}
		if resp.DurationMs < 0 {
			t.Fatalf("unexpected negative DurationMs: %d", resp.DurationMs)
		}
		if len(events) < 6 {
			t.Fatalf("expected translated events + stage events, got %d", len(events))
		}
	})

	t.Run("hardlink mode forwards CopyModeHardlink", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "hardlink-out")
		var gotMode format.CopyMode
		src := &mockAdapter{
			id: "eagle",
			openReaderFn: func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
				return &mockReader{readFn: func(ctx context.Context) (*domain.Library, error) { return buildLib(), nil }}, nil
			},
		}
		dst := &mockAdapter{
			id: "pixcall",
			openWriterFn: func(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
				gotMode = opts.CopyMode
				return &mockWriter{
					writeFn: func(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
						emit(format.ProgressEvent{Kind: "progress", AssetID: "a1", CopyMode: "hardlink"})
						emit(format.ProgressEvent{Kind: "progress", AssetID: "a2", CopyMode: "hardlink"})
						return nil
					},
				}, nil
			},
		}

		runner := NewRunner(NewRegistry(src, dst))
		resp, err := runner.Convert(context.Background(), debris.ConvertRequest{
			From:       "eagle",
			To:         "pixcall",
			InputPath:  inDir,
			OutputPath: outDir,
			CopyMode:   debris.CopyModeHardlink,
		}, nil)
		if err != nil {
			t.Fatalf("hardlink Convert failed: %v", err)
		}
		if gotMode != format.CopyModeHardlink || resp.CopyMode != debris.CopyModeHardlink || resp.HardlinkedAssets != 2 {
			t.Fatalf("unexpected hardlink result: gotMode=%q resp=%#v", gotMode, resp)
		}
	})

	t.Run("context cancellation during write closes writer and returns CodeCanceled", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "cancel-out")
		ctx, cancel := context.WithCancel(context.Background())
		reader := &mockReader{
			readFn: func(ctx context.Context) (*domain.Library, error) {
				return buildLib(), nil
			},
		}
		writer := &mockWriter{
			writeFn: func(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
				cancel()
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Second):
					return errors.New("timeout waiting for cancel")
				}
			},
		}
		src := &mockAdapter{
			id: "eagle",
			openReaderFn: func(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
				return reader, nil
			},
		}
		dst := &mockAdapter{
			id: "pixcall",
			openWriterFn: func(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
				return writer, nil
			},
		}

		runner := NewRunner(NewRegistry(src, dst))
		_, err := runner.Convert(ctx, debris.ConvertRequest{
			From:       "eagle",
			To:         "pixcall",
			InputPath:  inDir,
			OutputPath: outDir,
		}, nil)
		if err == nil || debris.AsError(err).Code != debris.CodeCanceled {
			t.Fatalf("expected CodeCanceled, got %v", err)
		}
		if !reader.closed || !writer.closed {
			t.Fatalf("expected reader and writer to be closed on cancel (reader=%v writer=%v)", reader.closed, writer.closed)
		}
	})
}
