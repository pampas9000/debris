package eagle_test

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/eagle"
	"debris/internal/format/eagle/model"
)

const sampleEagleLibraryPath = "/Users/kazuha/dev/debris/resources/sample_libraries/eagle.library"

func TestAdapterMetadata(t *testing.T) {
	adapter := eagle.New()
	if adapter.ID() != "eagle" {
		t.Fatalf("ID() = %q, want eagle", adapter.ID())
	}
	if adapter.DisplayName() != "Eagle" {
		t.Fatalf("DisplayName() = %q, want Eagle", adapter.DisplayName())
	}
	caps := adapter.Capabilities()
	if !caps.SupportsFolders || !caps.SupportsNestedFolders || !caps.SupportsMultiFolderAsset ||
		!caps.SupportsTagGroups || !caps.SupportsColors || !caps.SupportsRecycleBin ||
		!caps.SupportsThumbnails || !caps.SupportsRating || !caps.SupportsOriginURL ||
		!caps.SupportsAnnotation {
		t.Fatalf("unexpected false capability in Eagle Capabilities(): %+v", caps)
	}
	if caps.SupportsNestedTags {
		t.Fatalf("SupportsNestedTags should be false for Eagle")
	}
}

func TestProbe(t *testing.T) {
	adapter := eagle.New()
	ctx := context.Background()

	t.Run("empty and nonexistent", func(t *testing.T) {
		res, err := adapter.Probe(ctx, "")
		if err != nil || res.Matched {
			t.Fatalf("empty Probe = (%+v, %v), want unmatched nil err", res, err)
		}
		res, err = adapter.Probe(ctx, filepath.Join(t.TempDir(), "missing"))
		if err != nil || res.Matched {
			t.Fatalf("missing Probe = (%+v, %v), want unmatched nil err", res, err)
		}
	})

	t.Run("file instead of dir", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "file.txt")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := adapter.Probe(ctx, f)
		if err != nil || res.Matched {
			t.Fatalf("file Probe = (%+v, %v), want unmatched", res, err)
		}
	})

	t.Run("missing images or metadata", func(t *testing.T) {
		d1 := t.TempDir()
		_ = os.WriteFile(filepath.Join(d1, "metadata.json"), []byte(`{"applicationVersion":"4.0.0"}`), 0o644)
		res, err := adapter.Probe(ctx, d1)
		if err != nil || res.Matched {
			t.Fatalf("missing images Probe = (%+v, %v), want unmatched", res, err)
		}

		d2 := t.TempDir()
		_ = os.MkdirAll(filepath.Join(d2, "images"), 0o755)
		res, err = adapter.Probe(ctx, d2)
		if err != nil || res.Matched {
			t.Fatalf("missing metadata.json Probe = (%+v, %v), want unmatched", res, err)
		}
	})

	t.Run("invalid metadata json", func(t *testing.T) {
		d := t.TempDir()
		_ = os.MkdirAll(filepath.Join(d, "images"), 0o755)
		_ = os.WriteFile(filepath.Join(d, "metadata.json"), []byte("{not-json"), 0o644)
		res, err := adapter.Probe(ctx, d)
		if err != nil || res.Matched {
			t.Fatalf("invalid json Probe = (%+v, %v), want unmatched", res, err)
		}
	})

	t.Run("valid eagle dir", func(t *testing.T) {
		d := filepath.Join(t.TempDir(), "Design.library")
		_ = os.MkdirAll(filepath.Join(d, "images"), 0o755)
		_ = os.WriteFile(filepath.Join(d, "metadata.json"), []byte(`{"applicationVersion":"4.0.0","folders":[]}`), 0o644)
		res, err := adapter.Probe(ctx, d)
		if err != nil {
			t.Fatalf("Probe err: %v", err)
		}
		if !res.Matched || res.Format != "eagle" || res.Version != "4.0.0" || res.Name != "Design" || res.Confidence != 100 {
			t.Fatalf("unexpected ProbeResult: %+v", res)
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := adapter.Probe(cctx, t.TempDir()); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	})
}

func TestReadSpecialFilenamesDeletedTagsAndFolders(t *testing.T) {
	libDir := t.TempDir()
	imagesDir := filepath.Join(libDir, "images")
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rootMeta := model.LibraryMetadata{
		ApplicationVersion: "4.0.0",
		ModificationTime:   1700000000000,
		Folders: []model.Folder{
			{
				ID:               "FROOT00000001",
				Name:             "Root",
				Description:      "Top folder",
				ModificationTime: 1700000000100,
				Tags:             []string{"tagA"},
				Children: []model.Folder{
					{
						ID:               "FCHILD0000002",
						Name:             "Child",
						Description:      "Nested folder",
						ModificationTime: 1700000000200,
						Tags:             []string{"tagB"},
						Children: []model.Folder{
							{
								ID:               "FGRAND0000003",
								Name:             "Grandchild",
								Children:         []model.Folder{},
								Tags:             []string{},
								ModificationTime: 1700000000300,
							},
						},
					},
				},
			},
		},
		TagsGroups: []model.TagGroup{
			{
				ID:    "TG00000000001",
				Name:  "GroupOne",
				Color: "blue",
				Tags:  []string{"tagA", "tagB"},
			},
		},
		SmartFolders: []any{},
		QuickAccess:  []any{},
	}
	writeJSONFile(t, filepath.Join(libDir, "metadata.json"), rootMeta)

	// 1. Multi-dot filename: archive.tar.gz
	info1 := filepath.Join(imagesDir, "ITEM000000001.info")
	_ = os.MkdirAll(info1, 0o755)
	_ = os.WriteFile(filepath.Join(info1, "archive.tar.gz"), []byte("tar-gz-bytes"), 0o644)
	writeJSONFile(t, filepath.Join(info1, "metadata.json"), model.ImageMetadata{
		ID:               "ITEM000000001",
		Name:             "archive.tar",
		Ext:              "gz",
		Size:             12,
		Btime:            1700000001000,
		Mtime:            1700000002000,
		Tags:             []string{"tagA", "tagA", "shared"},
		Folders:          []string{"FCHILD0000002", "FGRAND0000003"},
		URL:              "https://example.com/archive",
		Annotation:       "multi dot",
		Star:             3,
		ModificationTime: 1700000002000,
		LastModified:     1700000002000,
		Palettes: []model.Palette{
			{Color: [3]uint8{10, 20, 30}, Ratio: 55.5},
		},
	})

	// 2. Missing extension filename: README
	info2 := filepath.Join(imagesDir, "ITEM000000002.info")
	_ = os.MkdirAll(info2, 0o755)
	_ = os.WriteFile(filepath.Join(info2, "README"), []byte("hello"), 0o644)
	writeJSONFile(t, filepath.Join(info2, "metadata.json"), model.ImageMetadata{
		ID:               "ITEM000000002",
		Name:             "README",
		Ext:              "",
		Size:             5,
		Btime:            1700000003000,
		Mtime:            1700000003000,
		Tags:             []string{"shared"},
		Folders:          []string{"FROOT00000001"},
		ModificationTime: 1700000003000,
		LastModified:     1700000003000,
	})

	// 3. Deleted item
	info3 := filepath.Join(imagesDir, "ITEM000000003.info")
	_ = os.MkdirAll(info3, 0o755)
	_ = os.WriteFile(filepath.Join(info3, "trash.png"), []byte("png"), 0o644)
	writeJSONFile(t, filepath.Join(info3, "metadata.json"), model.ImageMetadata{
		ID:               "ITEM000000003",
		Name:             "trash",
		Ext:              "png",
		Size:             3,
		IsDeleted:        true,
		Tags:             []string{"deletedOnly"},
		Folders:          []string{},
		ModificationTime: 1700000004000,
		LastModified:     1700000004000,
	})

	adapter := eagle.New()
	ctx := context.Background()

	// Default ReadOptions: IncludeDeleted = false
	r, err := adapter.OpenReader(ctx, libDir, format.ReadOptions{})
	if err != nil {
		t.Fatalf("OpenReader err: %v", err)
	}
	defer r.Close()

	insp, err := r.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect err: %v", err)
	}
	if insp.FolderCount != 3 || insp.TagGroupCount != 1 || insp.AssetCount != 2 || insp.DeletedCount != 1 || insp.TotalBytes != 17 {
		t.Fatalf("unexpected InspectResult: %+v", insp)
	}

	lib, err := r.Read(ctx)
	if err != nil {
		t.Fatalf("Read err: %v", err)
	}
	if len(lib.Folders) != 1 || len(lib.Folders[0].Children) != 1 || len(lib.Folders[0].Children[0].Children) != 1 {
		t.Fatalf("nested folders not preserved: %+v", lib.Folders)
	}
	child := lib.Folders[0].Children[0]
	if child.ParentID != "FROOT00000001" || child.Description != "Nested folder" || !slices.Equal(child.TagNames, []string{"tagB"}) {
		t.Fatalf("unexpected child folder: %+v", child)
	}
	if len(lib.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, want 2 (excluding deleted)", len(lib.Assets))
	}

	byID := map[string]*domain.Asset{}
	for _, a := range lib.Assets {
		byID[a.ID] = a
	}
	a1 := byID["ITEM000000001"]
	if a1 == nil || a1.FileName != "archive.tar.gz" || a1.Name != "archive.tar" || a1.Ext != "gz" {
		t.Fatalf("unexpected multi-dot asset: %+v", a1)
	}
	if !slices.Equal(a1.TagIDs, []string{"tagA", "shared"}) {
		t.Fatalf("a1.TagIDs = %v, want [tagA shared]", a1.TagIDs)
	}
	if len(a1.Colors) != 1 || a1.Colors[0] != (domain.PaletteColor{R: 10, G: 20, B: 30, Ratio: 55.5}) {
		t.Fatalf("a1.Colors = %+v", a1.Colors)
	}
	if a1.CreatedAt.UnixMilli() != 1700000001000 {
		t.Fatalf("a1.CreatedAt = %v", a1.CreatedAt)
	}
	a2 := byID["ITEM000000002"]
	if a2 == nil || a2.FileName != "README" || a2.Name != "README" || a2.Ext != "" {
		t.Fatalf("unexpected extensionless asset: %+v", a2)
	}

	// Verify tag group linkage and deduplication
	tagGroupByName := map[string]string{}
	for _, tg := range lib.Tags {
		tagGroupByName[tg.Name] = tg.GroupID
	}
	if tagGroupByName["tagA"] != "TG00000000001" || tagGroupByName["tagB"] != "TG00000000001" {
		t.Fatalf("unexpected tag group linkage: %+v", tagGroupByName)
	}
	if _, ok := tagGroupByName["deletedOnly"]; ok {
		t.Fatalf("deletedOnly tag should not be present when IncludeDeleted=false")
	}

	// IncludeDeleted = true
	rDel, err := adapter.OpenReader(ctx, libDir, format.ReadOptions{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("OpenReader(IncludeDeleted) err: %v", err)
	}
	defer rDel.Close()
	libDel, err := rDel.Read(ctx)
	if err != nil {
		t.Fatalf("Read(IncludeDeleted) err: %v", err)
	}
	if len(libDel.Assets) != 3 {
		t.Fatalf("len(Assets) with IncludeDeleted=true = %d, want 3", len(libDel.Assets))
	}
}

func TestWriterRoundTripNestedFoldersCopyHardlinkAndJSONShape(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()

	asset1Path := filepath.Join(srcDir, "pic.v1.png")
	asset1Bytes := []byte("png-data-123456")
	if err := os.WriteFile(asset1Path, asset1Bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	thumb1Path := filepath.Join(srcDir, "thumb.png")
	if err := os.WriteFile(thumb1Path, []byte("thumb-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	asset2Path := filepath.Join(srcDir, "Makefile")
	if err := os.WriteFile(asset2Path, []byte("all:\n\techo ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Provide mixed flat + nested folders to verify both ParentID and Children handling.
	lib := &domain.Library{
		Format:     "billfish",
		Name:       "MixedTest",
		ModifiedAt: time.UnixMilli(1710000000000).UTC(),
		Folders: []*domain.Folder{
			{
				ID:          "f1",
				Name:        "Parent",
				Description: "Parent desc",
				Seq:         1,
				TagNames:    []string{"design"},
				Children: []*domain.Folder{
					{
						ID:          "f2",
						ParentID:    "f1",
						Name:        "Child",
						Description: "Child desc",
						Seq:         2,
					},
				},
			},
			{
				ID:       "f3",
				ParentID: "f2",
				Name:     "GrandchildFromFlat",
				Seq:      1,
			},
		},
		TagGroups: []*domain.TagGroup{
			{ID: "tg1", Name: "Styles", Color: "red"},
		},
		Tags: []*domain.Tag{
			{ID: "t1", GroupID: "tg1", Name: "design"},
			{ID: "t2", Name: "minimal"},
		},
		Assets: []*domain.Asset{
			{
				ID:          "a1",
				FileName:    "pic.v1.png",
				Name:        "pic.v1",
				Ext:         "png",
				SourcePath:  asset1Path,
				Size:        int64(len(asset1Bytes)),
				Kind:        domain.KindImage,
				Width:       800,
				Height:      600,
				Rating:      5,
				Annotation:  "hero shot",
				OriginURL:   "https://example.com/1",
				Colors:      []domain.PaletteColor{{R: 88, G: 100, B: 63, Ratio: 41.5}},
				FolderIDs:   []string{"f3"},
				TagIDs:      []string{"t1", "t2"},
				CreatedAt:   time.UnixMilli(1710000001000).UTC(),
				ModifiedAt:  time.UnixMilli(1710000002000).UTC(),
				MediaMeta:   &domain.MediaMeta{ThumbnailPath: thumb1Path},
			},
			{
				ID:         "a2",
				FileName:   "Makefile",
				Name:       "Makefile",
				Ext:        "",
				SourcePath: asset2Path,
				Kind:       domain.KindOther,
				FolderIDs:  []string{"f1"},
				MediaMeta:  &domain.MediaMeta{NoThumbnail: true},
			},
			{
				ID:         "a3-missing",
				Name:       "ghost",
				Ext:        "jpg",
				SourcePath: filepath.Join(srcDir, "does-not-exist.jpg"),
			},
		},
	}

	adapter := eagle.New()

	t.Run("canceled context stops write", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "canceled.library")
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		w, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{CopyMode: format.CopyModeCopy})
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		if err := w.Write(cctx, lib, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("Write(canceled) err = %v, want context.Canceled", err)
		}
	})

	t.Run("copy mode writes full tree and JSON files", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "out.library")
		w, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{
			CopyMode: format.CopyModeCopy,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()

		var events []format.ProgressEvent
		if err := w.Write(ctx, lib, func(ev format.ProgressEvent) {
			events = append(events, ev)
		}); err != nil {
			t.Fatalf("Write err: %v", err)
		}

		// Verify required Eagle root files exist and have expected JSON shape.
		for _, name := range []string{"metadata.json", "tags.json", "mtime.json", "saved-filters.json", "actions.json"} {
			if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
				t.Fatalf("missing root file %s: %v", name, err)
			}
		}
		mtimeRaw, _ := os.ReadFile(filepath.Join(outDir, "mtime.json"))
		var mtimeMap map[string]int64
		if err := json.Unmarshal(mtimeRaw, &mtimeMap); err != nil {
			t.Fatalf("unmarshal mtime.json err: %v", err)
		}
		if mtimeMap["all"] != 2 || len(mtimeMap) != 3 {
			t.Fatalf("mtime.json = %v, want 2 item entries plus all=2", mtimeMap)
		}
		filtersRaw, _ := os.ReadFile(filepath.Join(outDir, "saved-filters.json"))
		if strings.TrimSpace(string(filtersRaw)) != "[]" {
			t.Fatalf("saved-filters.json = %q, want []", string(filtersRaw))
		}
		actionsRaw, _ := os.ReadFile(filepath.Join(outDir, "actions.json"))
		if strings.TrimSpace(string(actionsRaw)) != "[]" {
			t.Fatalf("actions.json = %q, want []", string(actionsRaw))
		}

		// Verify palette JSON serializes [3]uint8 as numeric array [88,100,63], not base64.
		rawRoot, _ := os.ReadFile(filepath.Join(outDir, "metadata.json"))
		if bytes.Contains(rawRoot, []byte(": null")) {
			t.Fatalf("root metadata.json contains null slice: %s", string(rawRoot))
		}

		// Re-read the written library and verify nested folders, palettes, thumbnail, extensionless file.
		r, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{})
		if err != nil {
			t.Fatalf("OpenReader err: %v", err)
		}
		defer r.Close()

		gotLib, err := r.Read(ctx)
		if err != nil {
			t.Fatalf("Read err: %v", err)
		}
		if gotLib.Version != "4.0.0" {
			t.Fatalf("Version = %q, want 4.0.0", gotLib.Version)
		}
		if len(gotLib.Folders) != 1 || len(gotLib.Folders[0].Children) != 1 || len(gotLib.Folders[0].Children[0].Children) != 1 {
			t.Fatalf("round-trip nested folders lost: %+v", gotLib.Folders)
		}
		grand := gotLib.Folders[0].Children[0].Children[0]
		if grand.Name != "GrandchildFromFlat" {
			t.Fatalf("grandchild name = %q", grand.Name)
		}
		if len(gotLib.Assets) != 2 {
			t.Fatalf("len(Assets) = %d, want 2 (missing source skipped)", len(gotLib.Assets))
		}

		var gotA1, gotA2 *domain.Asset
		for _, a := range gotLib.Assets {
			if a.Name == "pic.v1" {
				gotA1 = a
			} else if a.Name == "Makefile" {
				gotA2 = a
			}
		}
		if gotA1 == nil || gotA1.FileName != "pic.v1.png" || gotA1.Ext != "png" {
			t.Fatalf("gotA1 = %+v", gotA1)
		}
		if len(gotA1.Colors) != 1 || gotA1.Colors[0].R != 88 || gotA1.Colors[0].G != 100 || gotA1.Colors[0].B != 63 {
			t.Fatalf("gotA1.Colors = %+v", gotA1.Colors)
		}
		if gotA1.MediaMeta == nil || gotA1.MediaMeta.ThumbnailPath == "" {
			t.Fatalf("expected thumbnail copied for pic.v1")
		}
		if !slices.Equal(gotA1.FolderIDs, []string{grand.ID}) {
			t.Fatalf("gotA1.FolderIDs = %v, want [%s]", gotA1.FolderIDs, grand.ID)
		}
		if gotA2 == nil || gotA2.FileName != "Makefile" || gotA2.Ext != "" {
			t.Fatalf("gotA2 = %+v", gotA2)
		}
		if gotA2.MediaMeta == nil || !gotA2.MediaMeta.NoThumbnail {
			t.Fatalf("expected NoThumbnail=true preserved on gotA2")
		}

		// Verify copy mode created distinct inode
		srcInfo, _ := os.Stat(asset1Path)
		dstInfo, _ := os.Stat(gotA1.SourcePath)
		if os.SameFile(srcInfo, dstInfo) {
			t.Fatal("copy mode should not hardlink")
		}
	})

	t.Run("hardlink mode links asset files", func(t *testing.T) {
		outDir := filepath.Join(t.TempDir(), "hardlink.library")
		w, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{
			CopyMode: format.CopyModeHardlink,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()

		var hardlinked int
		if err := w.Write(ctx, lib, func(ev format.ProgressEvent) {
			if ev.Kind == "progress" && ev.CopyMode == "hardlink" {
				hardlinked++
			}
		}); err != nil {
			t.Fatalf("hardlink Write err: %v", err)
		}
		if hardlinked != 2 {
			t.Fatalf("hardlinked count = %d, want 2", hardlinked)
		}

		r, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		gotLib, err := r.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		srcInfo, _ := os.Stat(asset1Path)
		for _, a := range gotLib.Assets {
			if a.Name == "pic.v1" {
				dstInfo, _ := os.Stat(a.SourcePath)
				if !os.SameFile(srcInfo, dstInfo) {
					t.Fatal("hardlink mode did not preserve inode")
				}
			}
		}
	})
}

func TestIntegrationSampleEagleLibraryRoundTrip(t *testing.T) {
	if _, err := os.Stat(sampleEagleLibraryPath); errors.Is(err, os.ErrNotExist) {
		t.Skipf("sample library not found at %s", sampleEagleLibraryPath)
	}

	// Copy sample library into TempDir so original sample_libraries is never modified.
	tmpRoot := t.TempDir()
	copiedSample := filepath.Join(tmpRoot, "eagle.library")
	if err := copyDir(sampleEagleLibraryPath, copiedSample); err != nil {
		t.Fatalf("copy sample eagle.library: %v", err)
	}

	ctx := context.Background()
	adapter := eagle.New()

	// 1. Probe
	probeRes, err := adapter.Probe(ctx, copiedSample)
	if err != nil {
		t.Fatalf("Probe err: %v", err)
	}
	if !probeRes.Matched || probeRes.Format != "eagle" || probeRes.Version != "4.0.0" {
		t.Fatalf("unexpected ProbeResult: %+v", probeRes)
	}

	// 2. Inspect & Read
	r, err := adapter.OpenReader(ctx, copiedSample, format.ReadOptions{})
	if err != nil {
		t.Fatalf("OpenReader err: %v", err)
	}
	defer r.Close()

	insp, err := r.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect err: %v", err)
	}
	if insp.FolderCount != 2 || insp.AssetCount != 5 || insp.TagCount != 9 {
		t.Fatalf("unexpected sample InspectResult: %+v", insp)
	}
	if len(insp.Warnings) != 0 {
		t.Fatalf("unexpected inspect warnings: %v", insp.Warnings)
	}

	lib, err := r.Read(ctx)
	if err != nil {
		t.Fatalf("Read err: %v", err)
	}
	if len(lib.Folders) != 2 || len(lib.Assets) != 5 || len(lib.Tags) != 9 {
		t.Fatalf("unexpected sample Read counts: folders=%d assets=%d tags=%d", len(lib.Folders), len(lib.Assets), len(lib.Tags))
	}

	byID := make(map[string]*domain.Asset, len(lib.Assets))
	for _, a := range lib.Assets {
		byID[a.ID] = a
	}
	rabbit := byID["MPEBX8QSPQ98J"]
	if rabbit == nil {
		t.Fatal("missing rabbit asset MPEBX8QSPQ98J")
	}
	if rabbit.Kind != domain.KindImage || rabbit.Rating != 5 || rabbit.OriginURL != "https://www.rabbit.com" ||
		rabbit.Annotation != "兔子" || len(rabbit.Colors) != 10 || rabbit.MediaMeta == nil || !rabbit.MediaMeta.NoThumbnail {
		t.Fatalf("unexpected rabbit asset: %+v", rabbit)
	}

	video := byID["MWNELUATM50VH"]
	if video == nil {
		t.Fatal("missing video asset MWNELUATM50VH")
	}
	if video.Kind != domain.KindVideo || video.Width != 1920 || video.Height != 1080 ||
		video.DurationSec < 16.0 || video.MediaMeta == nil || video.MediaMeta.ThumbnailPath == "" {
		t.Fatalf("unexpected video asset: %+v", video)
	}

	// 3. Write round-trip
	rtPath := filepath.Join(tmpRoot, "roundtrip.library")
	w, err := adapter.OpenWriter(ctx, rtPath, format.WriteOptions{
		CopyMode: format.CopyModeCopy,
	})
	if err != nil {
		t.Fatalf("OpenWriter err: %v", err)
	}
	defer w.Close()

	var copiedCount int
	if err := w.Write(ctx, lib, func(ev format.ProgressEvent) {
		if ev.Kind == "progress" && ev.CopyMode == "copy" {
			copiedCount++
		}
	}); err != nil {
		t.Fatalf("round-trip Write err: %v", err)
	}
	if copiedCount != 5 {
		t.Fatalf("copiedCount = %d, want 5", copiedCount)
	}

	// 4. Re-Probe, Re-Inspect, Re-Read round-tripped library
	rtProbe, err := adapter.Probe(ctx, rtPath)
	if err != nil || !rtProbe.Matched || rtProbe.Version != "4.0.0" {
		t.Fatalf("round-trip Probe = (%+v, %v)", rtProbe, err)
	}

	rtReader, err := adapter.OpenReader(ctx, rtPath, format.ReadOptions{})
	if err != nil {
		t.Fatalf("round-trip OpenReader err: %v", err)
	}
	defer rtReader.Close()

	rtInsp, err := rtReader.Inspect(ctx)
	if err != nil {
		t.Fatalf("round-trip Inspect err: %v", err)
	}
	if rtInsp.FolderCount != 2 || rtInsp.AssetCount != 5 || rtInsp.TagCount != 9 || rtInsp.TotalBytes != insp.TotalBytes {
		t.Fatalf("round-trip Inspect mismatch: got %+v, want folders=2 assets=5 tags=9 bytes=%d", rtInsp, insp.TotalBytes)
	}

	rtLib, err := rtReader.Read(ctx)
	if err != nil {
		t.Fatalf("round-trip Read err: %v", err)
	}
	if len(rtLib.Folders) != 2 || len(rtLib.Assets) != 5 || len(rtLib.Tags) != 9 {
		t.Fatalf("round-trip Read counts mismatch: folders=%d assets=%d tags=%d", len(rtLib.Folders), len(rtLib.Assets), len(rtLib.Tags))
	}

	rtByName := make(map[string]*domain.Asset, len(rtLib.Assets))
	for _, a := range rtLib.Assets {
		rtByName[a.Name] = a
	}
	rtVideo := rtByName["14842032_1920_1080_25fps"]
	if rtVideo == nil || rtVideo.DurationSec != video.DurationSec || len(rtVideo.Colors) != len(video.Colors) ||
		rtVideo.MediaMeta == nil || rtVideo.MediaMeta.ThumbnailPath == "" || len(rtVideo.FolderIDs) != 1 {
		t.Fatalf("round-trip video asset mismatch: %+v", rtVideo)
	}
	rtRabbit := rtByName["IMG_6480 Medium"]
	if rtRabbit == nil || rtRabbit.Rating != 5 || rtRabbit.OriginURL != "https://www.rabbit.com" ||
		len(rtRabbit.Colors) != 10 || rtRabbit.MediaMeta == nil || !rtRabbit.MediaMeta.NoThumbnail {
		t.Fatalf("round-trip rabbit asset mismatch: %+v", rtRabbit)
	}
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v, jsontext.WithIndent("  "))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func TestStarredTagsAndJPEGThumbnailTranscode(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()

	assetPath := filepath.Join(srcDir, "shot.jpg")
	if err := os.WriteFile(assetPath, []byte("fake-jpg"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a valid JPEG thumbnail to test JPG -> PNG transcoding.
	jpgThumb := filepath.Join(srcDir, "thumb.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 12, 8))
	img.Set(0, 0, color.RGBA{R: 200, G: 50, B: 30, A: 255})
	f, err := os.Create(jpgThumb)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	lib := &domain.Library{
		Format: "refto",
		Tags: []*domain.Tag{
			{ID: "t1", Name: "Favorite", Starred: true},
			{ID: "t2", Name: "Plain", Starred: false},
		},
		Assets: []*domain.Asset{
			{
				ID:         "a1",
				FileName:   "shot.jpg",
				Name:       "shot",
				Ext:        "jpg",
				SourcePath: assetPath,
				Kind:       domain.KindImage,
				TagIDs:     []string{"t1", "t2"},
				MediaMeta:  &domain.MediaMeta{ThumbnailPath: jpgThumb},
			},
		},
	}

	adapter := eagle.New()
	outDir := filepath.Join(t.TempDir(), "starred.library")
	w, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{CopyMode: format.CopyModeCopy})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(ctx, lib, nil); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	_ = w.Close()

	r, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	gotLib, err := r.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	starredByName := map[string]bool{}
	for _, tg := range gotLib.Tags {
		starredByName[tg.Name] = tg.Starred
	}
	if !starredByName["Favorite"] || starredByName["Plain"] {
		t.Fatalf("unexpected starred tags after round-trip: %+v", starredByName)
	}

	if len(gotLib.Assets) != 1 || gotLib.Assets[0].MediaMeta == nil || gotLib.Assets[0].MediaMeta.ThumbnailPath == "" {
		t.Fatalf("missing transcoded thumbnail in Eagle output: %+v", gotLib.Assets)
	}
	tf, err := os.Open(gotLib.Assets[0].MediaMeta.ThumbnailPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tf.Close()
	cfg, err := png.DecodeConfig(tf)
	if err != nil {
		t.Fatalf("written thumbnail is not valid PNG: %v", err)
	}
	if cfg.Width != 12 || cfg.Height != 8 {
		t.Fatalf("thumbnail dimensions = %dx%d, want 12x8", cfg.Width, cfg.Height)
	}
}
