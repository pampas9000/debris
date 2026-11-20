package billfish

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
)

const sampleBillfishLibraryPath = "/Users/kazuha/dev/debris/resources/sample_libraries/billfish.library"

func TestAdapterMetadataAndProbeNegatives(t *testing.T) {
	adapter := New()
	if adapter.ID() != "billfish" {
		t.Fatalf("ID() = %q, want billfish", adapter.ID())
	}
	if adapter.DisplayName() != "Billfish" {
		t.Fatalf("DisplayName() = %q, want Billfish", adapter.DisplayName())
	}
	caps := adapter.Capabilities()
	if !caps.SupportsFolders || !caps.SupportsNestedFolders || caps.SupportsMultiFolderAsset ||
		!caps.SupportsTagGroups || !caps.SupportsNestedTags || !caps.SupportsColors ||
		!caps.SupportsRecycleBin || !caps.SupportsThumbnails || !caps.SupportsRating ||
		!caps.SupportsOriginURL || !caps.SupportsAnnotation {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}

	ctx := context.Background()

	// Empty path
	res, err := adapter.Probe(ctx, "")
	if err != nil || res.Matched {
		t.Fatalf("Probe empty path = (%+v, %v), want unmatched nil err", res, err)
	}

	// Nonexistent path
	res, err = adapter.Probe(ctx, filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil || res.Matched {
		t.Fatalf("Probe missing path = (%+v, %v), want unmatched nil err", res, err)
	}

	// Empty directory
	emptyDir := t.TempDir()
	res, err = adapter.Probe(ctx, emptyDir)
	if err != nil || res.Matched {
		t.Fatalf("Probe empty dir = (%+v, %v), want unmatched nil err", res, err)
	}

	// Invalid SQLite file at .bf/billfish.db
	badDBDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(badDBDir, ".bf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDBDir, ".bf", "billfish.db"), []byte("not sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = adapter.Probe(ctx, badDBDir)
	if err != nil || res.Matched {
		t.Fatalf("Probe invalid db = (%+v, %v), want unmatched nil err", res, err)
	}
}

func TestParseAndFormatBillfishColors(t *testing.T) {
	raw := "24.65,1979954|21.30,6916989|badentry|10.00,notanint|1.04,4018010"
	colors := parseBillfishColors(raw)
	if len(colors) != 3 {
		t.Fatalf("got %d colors, want 3: %#v", len(colors), colors)
	}
	if colors[0].R != 0x1E || colors[0].G != 0x36 || colors[0].B != 0x32 || colors[0].Ratio != 24.65 {
		t.Fatalf("unexpected first color: %+v", colors[0])
	}

	formatted := formatBillfishColors(colors)
	want := "24.65,1979954|21.30,6916989|1.04,4018010"
	if formatted != want {
		t.Fatalf("formatBillfishColors = %q, want %q", formatted, want)
	}

	if got := parseBillfishColors(""); got != nil {
		t.Fatalf("parseBillfishColors(\"\") = %#v, want nil", got)
	}
}

func TestWriteAndReadRoundTripSynthetic(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()

	// Create sample source asset files with > 9 items to verify multi-digit IDs (strconv.FormatInt).
	img1 := filepath.Join(srcDir, "cover.png")
	img2 := filepath.Join(srcDir, "sub.jpg")
	vid1 := filepath.Join(srcDir, "clip.mp4")
	aud1 := filepath.Join(srcDir, "theme.mp3")
	del1 := filepath.Join(srcDir, "trashed.png")
	hid1 := filepath.Join(srcDir, "secret.png")
	dupA := filepath.Join(srcDir, "a", "same.png")
	dupB := filepath.Join(srcDir, "b", "same.png")
	for _, p := range []string{img1, img2, vid1, aud1, del1, hid1, dupA, dupB} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("payload:"+filepath.Base(p)+":"+p), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	createdAt := time.Date(2026, 3, 15, 10, 20, 30, 0, time.UTC)
	modifiedAt := time.Date(2026, 3, 16, 11, 22, 33, 0, time.UTC)

	lib := &domain.Library{
		Name: "SyntheticLib",
		Folders: []*domain.Folder{
			{ID: "12", Name: "Parent", Seq: 500_000_000, CreatedAt: createdAt},
			{ID: "15", ParentID: "12", Name: "Child", Description: "nested child", Color: "#FF8800", Seq: 900_001_000, CreatedAt: createdAt},
			{ID: "20", Name: "TrashFolder", IsRecycle: true, Seq: 900_002_000},
		},
		TagGroups: []*domain.TagGroup{
			{ID: "10", Name: "GroupA", Color: "#112233", Ranking: 1},
		},
		Tags: []*domain.Tag{
			{ID: "100", GroupID: "10", Name: "Duplicate", Seq: 900_001_000, CreatedAt: createdAt},
			{ID: "101", GroupID: "10", Name: "Duplicate", Seq: 900_002_500, CreatedAt: createdAt},
			{ID: "102", ParentID: "101", Name: "ChildTag", Seq: 900_003_000, CreatedAt: createdAt},
		},
		Assets: []*domain.Asset{
			{
				ID:         "1",
				FileName:   "cover.png",
				SourcePath: img1,
				Kind:       domain.KindImage,
				Width:      800,
				Height:     600,
				Rating:     5,
				Annotation: "Main cover",
				OriginURL:  "https://example.com/1",
				Colors:     []domain.PaletteColor{{R: 255, G: 0, B: 128, Ratio: 55.5}},
				FolderIDs:  []string{"12"},
				TagIDs:     []string{"100", "102"},
				CreatedAt:  createdAt,
				ModifiedAt: modifiedAt,
				MediaMeta:  &domain.MediaMeta{Rotation: 180, HFlip: true},
			},
			{
				ID:         "2",
				FileName:   "sub.jpg",
				SourcePath: img2,
				Kind:       domain.KindImage,
				Width:      1024,
				Height:     768,
				FolderIDs:  []string{"15"},
				TagIDs:     []string{"101"},
				CreatedAt:  createdAt,
				ModifiedAt: modifiedAt,
			},
			{
				ID:          "3",
				FileName:    "clip.mp4",
				SourcePath:  vid1,
				Kind:        domain.KindVideo,
				Width:       1920,
				Height:      1080,
				DurationSec: 12.345,
				FolderIDs:   []string{"15"},
				MediaMeta:   &domain.MediaMeta{Rotation: 90},
				CreatedAt:   createdAt,
				ModifiedAt:  modifiedAt,
			},
			{
				ID:          "4",
				FileName:    "theme.mp3",
				SourcePath:  aud1,
				Kind:        domain.KindAudio,
				DurationSec: 45.2,
				MediaMeta:   &domain.MediaMeta{Bitrate: 320000, BPM: 128},
				CreatedAt:   createdAt,
				ModifiedAt:  modifiedAt,
			},
			{
				ID:         "5",
				FileName:   "same.png",
				SourcePath: dupA,
				Kind:       domain.KindImage,
				FolderIDs:  []string{"12"},
				CreatedAt:  createdAt,
			},
			{
				ID:         "6",
				FileName:   "same.png",
				SourcePath: dupB,
				Kind:       domain.KindImage,
				FolderIDs:  []string{"12"},
				CreatedAt:  createdAt,
			},
			{
				ID:         "7",
				FileName:   "trashed.png",
				SourcePath: del1,
				Kind:       domain.KindImage,
				IsDeleted:  true,
				CreatedAt:  createdAt,
			},
			{
				ID:         "8",
				FileName:   "secret.png",
				SourcePath: hid1,
				Kind:       domain.KindImage,
				IsHidden:   true,
				CreatedAt:  createdAt,
			},
			{
				ID:         "9",
				FileName:   "missing.png",
				SourcePath: filepath.Join(srcDir, "nonexistent.png"),
				Kind:       domain.KindImage,
			},
		},
	}

	// Real write with hardlink mode (and Overwrite check).
	adapter := New()
	outDir := t.TempDir()
	writer, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{
		CopyMode:       format.CopyModeHardlink,
		IncludeDeleted: true,
		IncludeHidden:  true,
	})
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	var events []format.ProgressEvent
	if err := writer.Write(ctx, lib, func(ev format.ProgressEvent) {
		events = append(events, ev)
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer Close: %v", err)
	}

	var hardlinkedCount, skipCount, warnCount int
	for _, ev := range events {
		switch ev.Kind {
		case "warning":
			warnCount++
		case "progress":
			switch ev.CopyMode {
			case "hardlink":
				hardlinkedCount++
			case "skip":
				skipCount++
			}
		}
	}
	if hardlinkedCount != 8 || skipCount != 1 || warnCount != 1 {
		t.Fatalf("unexpected write events: hardlink=%d skip=%d warn=%d", hardlinkedCount, skipCount, warnCount)
	}

	// Attempting to write again without Overwrite must fail.
	noOverwriteWriter, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{Overwrite: false})
	if err != nil {
		t.Fatal(err)
	}
	if err := noOverwriteWriter.Write(ctx, lib, nil); err == nil {
		t.Fatalf("expected error writing over existing DB with Overwrite=false")
	}
	_ = noOverwriteWriter.Close()

	// Overwrite=true with hardlink must succeed cleanly.
	overwriteWriter, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{
		CopyMode:       format.CopyModeHardlink,
		Overwrite:      true,
		IncludeDeleted: true,
		IncludeHidden:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := overwriteWriter.Write(ctx, lib, nil); err != nil {
		t.Fatalf("Overwrite write failed: %v", err)
	}
	_ = overwriteWriter.Close()

	// Read back excluding deleted & hidden first.
	readerFiltered, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{})
	if err != nil {
		t.Fatalf("OpenReader filtered: %v", err)
	}
	inspectFiltered, err := readerFiltered.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect filtered: %v", err)
	}
	if inspectFiltered.FolderCount != 2 || inspectFiltered.TagGroupCount != 1 || inspectFiltered.TagCount != 3 ||
		inspectFiltered.AssetCount != 6 || inspectFiltered.DeletedCount != 1 || inspectFiltered.HiddenCount != 1 {
		t.Fatalf("unexpected inspectFiltered: %+v", inspectFiltered)
	}
	_ = readerFiltered.Close()

	// Read back including deleted & hidden.
	readerAll, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{IncludeDeleted: true, IncludeHidden: true})
	if err != nil {
		t.Fatalf("OpenReader all: %v", err)
	}
	defer readerAll.Close()

	gotLib, err := readerAll.Read(ctx)
	if err != nil {
		t.Fatalf("Read all: %v", err)
	}
	if gotLib.Name != "SyntheticLib" || gotLib.Version != "41" {
		t.Fatalf("unexpected lib header: %+v", gotLib)
	}
	if len(gotLib.Folders) != 3 || len(gotLib.TagGroups) != 1 || len(gotLib.Tags) != 3 || len(gotLib.Assets) != 8 {
		t.Fatalf("unexpected counts: folders=%d groups=%d tags=%d assets=%d",
			len(gotLib.Folders), len(gotLib.TagGroups), len(gotLib.Tags), len(gotLib.Assets))
	}

	// Verify nested tags and duplicate tag names kept distinct by ID.
	tagByID := make(map[string]*domain.Tag)
	for _, tg := range gotLib.Tags {
		tagByID[tg.ID] = tg
	}
	if len(tagByID) != 3 {
		t.Fatalf("expected 3 unique tag IDs, got %d", len(tagByID))
	}
	var childTag *domain.Tag
	for _, tg := range gotLib.Tags {
		if tg.Name == "ChildTag" {
			childTag = tg
		}
	}
	if childTag == nil || childTag.ParentID == "" || tagByID[childTag.ParentID].Name != "Duplicate" {
		t.Fatalf("nested ChildTag parent mismatch: %+v", childTag)
	}

	// Verify assets mapped properly, including nested folder path, duration, audio metadata, and recycle bin path.
	assetByName := make(map[string]*domain.Asset)
	for _, a := range gotLib.Assets {
		assetByName[a.FileName] = a
		if _, err := os.Stat(a.SourcePath); err != nil {
			t.Fatalf("asset %q SourcePath %q missing: %v", a.FileName, a.SourcePath, err)
		}
	}
	if a := assetByName["clip.mp4"]; a == nil || a.Kind != domain.KindVideo || a.DurationSec != 12.345 || a.MediaMeta == nil || a.MediaMeta.Rotation != 90 {
		t.Fatalf("unexpected clip.mp4: %+v", a)
	}
	if a := assetByName["theme.mp3"]; a == nil || a.Kind != domain.KindAudio || a.DurationSec != 45.2 || a.MediaMeta == nil || a.MediaMeta.Bitrate != 320000 || a.MediaMeta.BPM != 128 {
		t.Fatalf("unexpected theme.mp3: %+v", a)
	}
	if a := assetByName["trashed.png"]; a == nil || !a.IsDeleted || filepath.Base(filepath.Dir(a.SourcePath)) != ".recycle" {
		t.Fatalf("unexpected trashed.png: %+v", a)
	}
	if a := assetByName["same (2).png"]; a == nil {
		t.Fatalf("expected disambiguated same (2).png, got keys: %v", assetByName)
	}
	if a := assetByName["cover.png"]; a == nil || a.CreatedAt.Unix() != createdAt.Unix() || a.Rating != 5 || a.Annotation != "Main cover" || a.OriginURL != "https://example.com/1" {
		t.Fatalf("unexpected cover.png: %+v", a)
	}
}

func TestWriteCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter := New()
	if _, err := adapter.OpenWriter(ctx, t.TempDir(), format.WriteOptions{}); err == nil {
		t.Fatal("expected canceled context error from OpenWriter")
	}
}

func TestSampleLibraryProbeInspectReadWriteRoundTrip(t *testing.T) {
	if _, err := os.Stat(sampleBillfishLibraryPath); os.IsNotExist(err) {
		t.Skipf("sample library not present at %s", sampleBillfishLibraryPath)
	}

	// Copy sample library into TempDir so original sample_libraries is never touched.
	sampleCopy := filepath.Join(t.TempDir(), "billfish.library")
	copyDirRecursive(t, sampleBillfishLibraryPath, sampleCopy)

	ctx := context.Background()
	adapter := New()

	// 1. Probe
	probeRes, err := adapter.Probe(ctx, sampleCopy)
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if !probeRes.Matched || probeRes.Format != "billfish" || probeRes.Version != "41" || probeRes.Name != "sample_libraries" {
		t.Fatalf("unexpected ProbeResult: %+v", probeRes)
	}

	// 2. OpenReader + Inspect
	reader, err := adapter.OpenReader(ctx, sampleCopy, format.ReadOptions{})
	if err != nil {
		t.Fatalf("OpenReader failed: %v", err)
	}
	defer reader.Close()

	inspectRes, err := reader.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if inspectRes.FolderCount != 2 || inspectRes.TagGroupCount != 1 || inspectRes.TagCount != 5 ||
		inspectRes.AssetCount != 4 || inspectRes.DeletedCount != 0 || inspectRes.TotalBytes <= 0 {
		t.Fatalf("unexpected InspectResult: %+v", inspectRes)
	}

	// 3. Read
	lib, err := reader.Read(ctx)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(lib.Folders) != 2 || len(lib.TagGroups) != 1 || len(lib.Tags) != 5 || len(lib.Assets) != 4 {
		t.Fatalf("unexpected Read counts: folders=%d groups=%d tags=%d assets=%d",
			len(lib.Folders), len(lib.TagGroups), len(lib.Tags), len(lib.Assets))
	}

	// Verify duplicate tag name handling ("风景" ids 2 and 3) and nested tag ("SubTag" id 5 -> pid 4).
	tagsByID := make(map[string]*domain.Tag, len(lib.Tags))
	for _, tg := range lib.Tags {
		tagsByID[tg.ID] = tg
	}
	if tagsByID["2"] == nil || tagsByID["2"].Name != "风景" || tagsByID["3"] == nil || tagsByID["3"].Name != "风景" {
		t.Fatalf("expected two distinct 风景 tags (2 and 3), got %#v", tagsByID)
	}
	if tagsByID["5"] == nil || tagsByID["5"].Name != "SubTag" || tagsByID["5"].ParentID != "4" {
		t.Fatalf("expected SubTag(5) under Test(4), got %+v", tagsByID["5"])
	}

	// Verify assets.
	assetsByName := make(map[string]*domain.Asset, len(lib.Assets))
	for _, a := range lib.Assets {
		assetsByName[a.FileName] = a
		if _, err := os.Stat(a.SourcePath); err != nil {
			t.Fatalf("asset %q SourcePath %q missing: %v", a.FileName, a.SourcePath, err)
		}
		if a.CreatedAt.Year() < 2026 {
			t.Fatalf("asset %q CreatedAt=%v looks wrong (expected Unix seconds >= 2026)", a.FileName, a.CreatedAt)
		}
	}

	mov := assetsByName["Apple iPhone 15 - video.MOV"]
	if mov == nil {
		t.Fatal("missing Apple iPhone 15 - video.MOV")
	}
	if mov.Kind != domain.KindVideo || mov.Width != 1440 || mov.Height != 1920 ||
		mov.DurationSec != 2.005 || mov.MediaMeta == nil || mov.MediaMeta.Rotation != 90 ||
		!slices.Equal(mov.FolderIDs, []string{"4"}) || !slices.Equal(mov.TagIDs, []string{"2"}) {
		t.Fatalf("unexpected video asset: %+v (mediaMeta=%+v)", mov, mov.MediaMeta)
	}

	heic := assetsByName["Apple iPhone 15 - photo.HEIC"]
	if heic == nil || heic.Kind != domain.KindImage || !slices.Equal(heic.FolderIDs, []string{"4"}) ||
		!slices.Contains(heic.TagIDs, "3") || !slices.Contains(heic.TagIDs, "5") {
		t.Fatalf("unexpected HEIC asset: %+v", heic)
	}

	rabbit := assetsByName["IMG_6480 Medium.jpeg"]
	if rabbit == nil || rabbit.Rating != 5 || rabbit.Annotation != "兔子" || rabbit.OriginURL != "https://www.rabbit.com" ||
		len(rabbit.Colors) == 0 || rabbit.Colors[0].Ratio != 24.65 ||
		rabbit.Colors[0].R != 30 || rabbit.Colors[0].G != 54 || rabbit.Colors[0].B != 50 {
		t.Fatalf("unexpected IMG_6480 Medium.jpeg: %+v", rabbit)
	}

	// 4. Write round-trip
	roundTripDir := filepath.Join(t.TempDir(), "roundtrip.library")
	writer, err := adapter.OpenWriter(ctx, roundTripDir, format.WriteOptions{CopyMode: format.CopyModeCopy})
	if err != nil {
		t.Fatalf("OpenWriter roundtrip: %v", err)
	}
	var copyEvents int
	if err := writer.Write(ctx, lib, func(ev format.ProgressEvent) {
		if ev.Kind == "progress" && ev.CopyMode == "copy" {
			copyEvents++
		}
	}); err != nil {
		t.Fatalf("Write roundtrip: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer Close: %v", err)
	}
	if copyEvents != 4 {
		t.Fatalf("expected 4 copy progress events, got %d", copyEvents)
	}

	// 5. Re-Probe, Re-Inspect, Re-Read round-tripped library
	rtProbe, err := adapter.Probe(ctx, roundTripDir)
	if err != nil || !rtProbe.Matched || rtProbe.Version != "41" || rtProbe.Name != "sample_libraries" {
		t.Fatalf("unexpected round-trip Probe: %+v err=%v", rtProbe, err)
	}

	rtReader, err := adapter.OpenReader(ctx, roundTripDir, format.ReadOptions{})
	if err != nil {
		t.Fatalf("OpenReader roundtrip: %v", err)
	}
	defer rtReader.Close()

	rtLib, err := rtReader.Read(ctx)
	if err != nil {
		t.Fatalf("Read roundtrip: %v", err)
	}
	if len(rtLib.Folders) != len(lib.Folders) || len(rtLib.TagGroups) != len(lib.TagGroups) ||
		len(rtLib.Tags) != len(lib.Tags) || len(rtLib.Assets) != len(lib.Assets) {
		t.Fatalf("roundtrip count mismatch: got (%d,%d,%d,%d), want (%d,%d,%d,%d)",
			len(rtLib.Folders), len(rtLib.TagGroups), len(rtLib.Tags), len(rtLib.Assets),
			len(lib.Folders), len(lib.TagGroups), len(lib.Tags), len(lib.Assets))
	}

	rtAssets := make(map[string]*domain.Asset, len(rtLib.Assets))
	for _, a := range rtLib.Assets {
		rtAssets[a.FileName] = a
		if _, err := os.Stat(a.SourcePath); err != nil {
			t.Fatalf("roundtrip asset %q missing on disk at %q: %v", a.FileName, a.SourcePath, err)
		}
	}
	rtMov := rtAssets["Apple iPhone 15 - video.MOV"]
	if rtMov == nil || rtMov.DurationSec != 2.005 || rtMov.MediaMeta == nil || rtMov.MediaMeta.Rotation != 90 {
		t.Fatalf("roundtrip video mismatch: %+v", rtMov)
	}
	rtRabbit := rtAssets["IMG_6480 Medium.jpeg"]
	if rtRabbit == nil || rtRabbit.Rating != 5 || rtRabbit.Annotation != "兔子" ||
		len(rtRabbit.Colors) != len(rabbit.Colors) || rtRabbit.Colors[0] != rabbit.Colors[0] {
		t.Fatalf("roundtrip rabbit mismatch: %+v", rtRabbit)
	}
	// Verify video and photo retained distinct "风景" tags (not collapsed by name!),
	// and rabbit retained its 1 tag ("🐰").
	if len(rtMov.TagIDs) != 1 || len(rtRabbit.TagIDs) != 1 {
		t.Fatalf("unexpected tag IDs after roundtrip: mov=%v rabbit=%v", rtMov.TagIDs, rtRabbit.TagIDs)
	}
	rtPhoto := rtAssets["Apple iPhone 15 - photo.HEIC"]
	if rtPhoto == nil || len(rtPhoto.TagIDs) != 2 || rtMov.TagIDs[0] == rtPhoto.TagIDs[0] && rtMov.TagIDs[0] == rtPhoto.TagIDs[1] {
		t.Fatalf("unexpected photo tag IDs after roundtrip: %v", rtPhoto)
	}
	if slices.Contains(rtPhoto.TagIDs, rtMov.TagIDs[0]) {
		t.Fatalf("duplicate tag names collapsed during roundtrip: mov=%v photo=%v", rtMov.TagIDs, rtPhoto.TagIDs)
	}
}

func copyDirRecursive(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatalf("copyDirRecursive(%q -> %q): %v", src, dst, err)
	}
}
