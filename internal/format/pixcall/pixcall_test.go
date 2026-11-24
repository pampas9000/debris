package pixcall_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/pixcall"
	"debris/internal/format/pixcall/model"
	"debris/internal/kit"
)

const samplePixcallPath = "/Users/kazuha/dev/debris/resources/sample_libraries/pixcall.library"

func copyDirRecursive(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
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
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy sample library %q -> %q: %v", src, dst, err)
	}
}

func TestModelTableNames(t *testing.T) {
	cases := []struct {
		tabler interface{ TableName() string }
		want   string
	}{
		{model.Entry{}, "entries"},
		{model.RecentEntry{}, "recent_entries"},
		{model.Folders{}, "folders"},
		{model.Tag{}, "tags"},
		{model.TagGroups{}, "tag_groups"},
		{model.Board{}, "boards"},
		{model.BoardEntry{}, "board_entries"},
		{model.Kvs{}, "kvs"},
		{model.Media{}, "media"},
		{model.Exif{}, "exif"},
		{model.Caption{}, "captions"},
		{model.RemoteEvents{}, "remote_events"},
	}
	for _, tc := range cases {
		if got := tc.tabler.TableName(); got != tc.want {
			t.Errorf("%T.TableName() = %q, want %q", tc.tabler, got, tc.want)
		}
	}
}

func TestAdapterMetadata(t *testing.T) {
	a := pixcall.New()
	if a.ID() != "pixcall" {
		t.Fatalf("ID() = %q, want pixcall", a.ID())
	}
	if a.DisplayName() != "Pixcall" {
		t.Fatalf("DisplayName() = %q, want Pixcall", a.DisplayName())
	}
	caps := a.Capabilities()
	if !caps.SupportsFolders || !caps.SupportsNestedFolders || !caps.SupportsTagGroups ||
		!caps.SupportsColors || !caps.SupportsRecycleBin || !caps.SupportsRating ||
		!caps.SupportsOriginURL || !caps.SupportsAnnotation {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
	if caps.SupportsMultiFolderAsset || caps.SupportsNestedTags {
		t.Fatalf("unexpected multi-folder or nested-tag capability: %+v", caps)
	}
}

func TestProbe(t *testing.T) {
	a := pixcall.New()
	ctx := context.Background()

	res, err := a.Probe(ctx, "")
	if err != nil || res.Matched {
		t.Fatalf("empty path probe = (%+v, %v), want unmatched nil err", res, err)
	}

	emptyDir := t.TempDir()
	res, err = a.Probe(ctx, emptyDir)
	if err != nil || res.Matched {
		t.Fatalf("empty dir probe = (%+v, %v), want unmatched nil err", res, err)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := a.Probe(canceledCtx, emptyDir); err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestSampleLibraryProbeReadWriteRoundTrip(t *testing.T) {
	if _, err := os.Stat(samplePixcallPath); os.IsNotExist(err) {
		t.Skipf("sample library not present at %s", samplePixcallPath)
	}

	tmpDir := t.TempDir()
	libCopy := filepath.Join(tmpDir, "pixcall.library")
	copyDirRecursive(t, samplePixcallPath, libCopy)

	ctx := context.Background()
	adapter := pixcall.New()

	probeRes, err := adapter.Probe(ctx, libCopy)
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if !probeRes.Matched || probeRes.Format != "pixcall" || probeRes.Version != "22" || probeRes.Confidence != 100 {
		t.Fatalf("unexpected ProbeResult: %+v", probeRes)
	}

	reader, err := adapter.OpenReader(ctx, libCopy, format.ReadOptions{})
	if err != nil {
		t.Fatalf("OpenReader error: %v", err)
	}
	defer reader.Close()

	inspectRes, err := reader.Inspect(ctx)
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	if inspectRes.Version != "22" || inspectRes.TagGroupCount != 2 || inspectRes.TagCount != 3 ||
		inspectRes.FolderCount != 0 || inspectRes.AssetCount != 6 || inspectRes.TotalBytes != 22594861 {
		t.Fatalf("unexpected InspectResult: %+v", inspectRes)
	}

	lib, err := reader.Read(ctx)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(lib.TagGroups) != 2 || len(lib.Tags) != 3 || len(lib.Assets) != 6 {
		t.Fatalf("unexpected Read counts: groups=%d tags=%d assets=%d", len(lib.TagGroups), len(lib.Tags), len(lib.Assets))
	}

	tagByID := make(map[string]*domain.Tag, len(lib.Tags))
	for _, tg := range lib.Tags {
		tagByID[tg.ID] = tg
	}

	assetByName := make(map[string]*domain.Asset, len(lib.Assets))
	for _, asset := range lib.Assets {
		assetByName[asset.FileName] = asset
		if _, err := os.Stat(asset.SourcePath); err != nil {
			t.Errorf("asset %q SourcePath missing %q: %v", asset.FileName, asset.SourcePath, err)
		}
	}

	heic := assetByName["2_q80.heic"]
	if heic == nil || heic.Kind != domain.KindImage || heic.Width != 1440 || heic.Height != 1920 ||
		heic.Rating != 2 || heic.OriginURL != "https://www.xiaohongshu.com" || len(heic.Colors) != 6 {
		t.Fatalf("unexpected heic asset: %+v", heic)
	}
	if len(heic.TagIDs) != 1 || tagByID[heic.TagIDs[0]].Name != "风景" {
		t.Fatalf("unexpected heic tags: %+v", heic.TagIDs)
	}
	if heic.ModifiedAt.UnixMilli() != 1794217497398 {
		t.Fatalf("heic ModifiedAt ms = %d, want 1794217497398", heic.ModifiedAt.UnixMilli())
	}

	avif := assetByName["小黄人.avif"]
	if avif == nil || avif.Annotation != "测试小黄人" || avif.Rating != 5 || avif.Width != 4032 || avif.Height != 3024 {
		t.Fatalf("unexpected avif asset: %+v", avif)
	}

	mov := assetByName["Apple iPhone 15 - video.MOV"]
	if mov == nil || mov.Kind != domain.KindVideo || mov.Width != 1440 || mov.Height != 1920 ||
		mov.DurationSec < 2.0 || mov.MediaMeta == nil || mov.MediaMeta.Rotation != -90 ||
		mov.MediaMeta.Bitrate != 15652382 {
		t.Fatalf("unexpected mov asset: %+v (mediaMeta=%+v)", mov, mov.MediaMeta)
	}

	// Write round-trip
	outLib := filepath.Join(tmpDir, "roundtrip.pixcall")
	writer, err := adapter.OpenWriter(ctx, outLib, format.WriteOptions{
		CopyMode: format.CopyModeCopy,
	})
	if err != nil {
		t.Fatalf("OpenWriter error: %v", err)
	}
	defer writer.Close()

	var events []format.ProgressEvent
	err = writer.Write(ctx, lib, func(ev format.ProgressEvent) {
		events = append(events, ev)
	})
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close error: %v", err)
	}

	// Verify required .pixcall files
	for _, rel := range []string{
		".pixcall/database/main.db",
		".pixcall/config.json",
		".pixcall/trash",
	} {
		if _, err := os.Stat(filepath.Join(outLib, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	idFiles, _ := filepath.Glob(filepath.Join(outLib, ".pixcall", "*.id"))
	if len(idFiles) != 1 {
		t.Fatalf("expected 1 .id file, got %v", idFiles)
	}

	var copiedCount int
	for _, ev := range events {
		if ev.Kind == "progress" && ev.CopyMode == "copy" {
			copiedCount++
		}
	}
	if copiedCount != 6 {
		t.Fatalf("copied progress events = %d, want 6", copiedCount)
	}

	// Re-probe and re-read written library
	rtProbe, err := adapter.Probe(ctx, outLib)
	if err != nil || !rtProbe.Matched || rtProbe.Version != "22" {
		t.Fatalf("round-trip Probe = (%+v, %v)", rtProbe, err)
	}
	rtReader, err := adapter.OpenReader(ctx, outLib, format.ReadOptions{})
	if err != nil {
		t.Fatalf("round-trip OpenReader error: %v", err)
	}
	defer rtReader.Close()

	rtLib, err := rtReader.Read(ctx)
	if err != nil {
		t.Fatalf("round-trip Read error: %v", err)
	}
	if len(rtLib.TagGroups) != 2 || len(rtLib.Tags) != 3 || len(rtLib.Assets) != 6 {
		t.Fatalf("round-trip counts mismatch: groups=%d tags=%d assets=%d",
			len(rtLib.TagGroups), len(rtLib.Tags), len(rtLib.Assets))
	}

	rtAssets := make(map[string]*domain.Asset, len(rtLib.Assets))
	for _, asset := range rtLib.Assets {
		rtAssets[asset.FileName] = asset
		if _, err := os.Stat(asset.SourcePath); err != nil {
			t.Fatalf("round-trip asset file missing %q: %v", asset.SourcePath, err)
		}
	}
	rtMov := rtAssets["Apple iPhone 15 - video.MOV"]
	if rtMov == nil || rtMov.MediaMeta == nil || rtMov.MediaMeta.Rotation != -90 || rtMov.MediaMeta.Bitrate != 15652382 {
		t.Fatalf("round-trip mov media mismatch: %+v", rtMov)
	}
}

func TestWriterEdgeCasesAndRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fileA := filepath.Join(srcDir, "shot.png")
	fileB := filepath.Join(srcDir, "shot_dup.png")
	fileTrash := filepath.Join(srcDir, "old.jpg")
	fileHidden := filepath.Join(srcDir, "secret.webp")
	for _, p := range []string{fileA, fileB, fileTrash, fileHidden} {
		if err := os.WriteFile(p, []byte("data:"+filepath.Base(p)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	lib := &domain.Library{
		Name: "EdgeTest",
		TagGroups: []*domain.TagGroup{
			{ID: "g1", Name: "颜色"},
			{ID: "g2", Name: "颜色"}, // duplicate group name -> deduplicated
		},
		Tags: []*domain.Tag{
			{ID: "t1", GroupID: "g1", Name: "红色"},
			{ID: "t2", GroupID: "g2", Name: "红色"}, // duplicate within same resolved group
			{ID: "t3", Name: "未分组"},
		},
		Folders: []*domain.Folder{
			{
				ID:          "f1",
				Name:        "项目",
				Description: "root folder",
				Children: []*domain.Folder{
					{ID: "f2", Name: "子目录"},
				},
			},
			{
				ID:   "f3",
				Name: "项目", // sibling collision under root -> disambiguated
			},
			{
				ID:        "f_rec",
				Name:      "废纸篓目录",
				IsRecycle: true,
			},
		},
		Assets: []*domain.Asset{
			{
				ID:         "a1",
				FileName:   "shot.png",
				SourcePath: fileA,
				Kind:       domain.KindImage,
				Width:      800,
				Height:     600,
				FolderIDs:  []string{"f2"},
				TagIDs:     []string{"t1", "t2", "t3"},
				Colors:     []domain.PaletteColor{{R: 255, G: 0, B: 0, Ratio: 50}},
				CreatedAt:  now,
				ModifiedAt: now,
			},
			{
				ID:         "a2",
				FileName:   "shot.png", // duplicate filename in same folder f2 -> "shot (2).png"
				SourcePath: fileB,
				Kind:       domain.KindImage,
				FolderIDs:  []string{"f2"},
				CreatedAt:  now,
				ModifiedAt: now,
			},
			{
				ID:         "a3",
				FileName:   "old.jpg",
				SourcePath: fileTrash,
				Kind:       domain.KindImage,
				IsDeleted:  true,
				CreatedAt:  now,
				ModifiedAt: now,
			},
			{
				ID:         "a4",
				FileName:   "secret.webp",
				SourcePath: fileHidden,
				Kind:       domain.KindImage,
				IsHidden:   true,
				FolderIDs:  []string{"f1"},
				CreatedAt:  now,
				ModifiedAt: now,
			},
			{
				ID:         "a5_missing",
				FileName:   "missing.png",
				SourcePath: filepath.Join(srcDir, "does_not_exist.png"),
				Kind:       domain.KindImage,
			},
		},
	}

	ctx := context.Background()
	adapter := pixcall.New()

	// Real write with Hardlink mode
	outDir := filepath.Join(tmpDir, "real_out")
	writer, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{
		CopyMode: format.CopyModeHardlink,
	})
	if err != nil {
		t.Fatalf("OpenWriter error: %v", err)
	}
	var warnCount, linkCount, skipCount int
	err = writer.Write(ctx, lib, func(ev format.ProgressEvent) {
		switch ev.Kind {
		case "warning":
			warnCount++
		case "progress":
			switch ev.CopyMode {
			case "hardlink":
				linkCount++
			case "skip":
				skipCount++
			}
		}
	})
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	_ = writer.Close()
	if warnCount != 1 || linkCount != 4 || skipCount != 1 {
		t.Fatalf("unexpected transfer tally: warn=%d link=%d skip=%d", warnCount, linkCount, skipCount)
	}

	// Verify hardlink shares inode
	srcInfo, _ := os.Stat(fileA)
	dstInfo, err := os.Stat(filepath.Join(outDir, "项目", "子目录", "shot.png"))
	if err != nil || !os.SameFile(srcInfo, dstInfo) {
		t.Fatalf("expected hardlinked file at 项目/子目录/shot.png (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "项目", "子目录", "shot (2).png")); err != nil {
		t.Fatalf("expected disambiguated file shot (2).png: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, ".pixcall", "trash", "old.jpg")); err != nil {
		t.Fatalf("expected trashed file in .pixcall/trash/old.jpg: %v", err)
	}

	// Verify Overwrite=false blocks overwrite, Overwrite=true succeeds
	wNoOverwrite, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{Overwrite: false})
	if err != nil {
		t.Fatal(err)
	}
	if err := wNoOverwrite.Write(ctx, lib, nil); err == nil {
		t.Fatal("expected error when Overwrite=false on existing library")
	}
	_ = wNoOverwrite.Close()

	wOverwrite, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{Overwrite: true, CopyMode: format.CopyModeHardlink})
	if err != nil {
		t.Fatal(err)
	}
	if err := wOverwrite.Write(ctx, lib, nil); err != nil {
		t.Fatalf("expected Overwrite=true with hardlink to succeed: %v", err)
	}
	_ = wOverwrite.Close()

	// Verify DB stats on root folder (pub_file_count excludes hidden & deleted) and seeded kvs
	db, closer, err := kit.OpenReadOnly(filepath.Join(outDir, ".pixcall", "database", "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	var rootFolder model.Folders
	if err := db.Where("entry_id = ?", 1).First(&rootFolder).Error; err != nil {
		_ = closer.Close()
		t.Fatal(err)
	}
	var schemaKV model.Kvs
	if err := db.Where("k = ?", "schema_version").First(&schemaKV).Error; err != nil {
		_ = closer.Close()
		t.Fatal(err)
	}
	_ = closer.Close()
	if schemaKV.V != "22" {
		t.Fatalf("schema_version = %q, want 22", schemaKV.V)
	}
	if rootFolder.FileCount != 3 || rootFolder.PubFileCount != 2 || rootFolder.FolderCount != 3 || rootFolder.PubFolderCount != 3 {
		t.Fatalf("unexpected root folder stats: %+v", rootFolder)
	}

	// Verify Read filtering with IncludeDeleted/IncludeHidden combinations
	rFiltered, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	filteredLib, err := rFiltered.Read(ctx)
	_ = rFiltered.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(filteredLib.Assets) != 2 || len(filteredLib.Folders) != 3 {
		t.Fatalf("filtered Read got assets=%d folders=%d, want 2 and 3", len(filteredLib.Assets), len(filteredLib.Folders))
	}

	rAll, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{IncludeDeleted: true, IncludeHidden: true})
	if err != nil {
		t.Fatal(err)
	}
	allLib, err := rAll.Read(ctx)
	_ = rAll.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(allLib.Assets) != 4 || len(allLib.Folders) != 4 {
		t.Fatalf("all Read got assets=%d folders=%d, want 4 and 4", len(allLib.Assets), len(allLib.Folders))
	}
	if len(allLib.TagGroups) != 1 || len(allLib.Tags) != 2 {
		t.Fatalf("deduplicated tags mismatch: groups=%d tags=%d", len(allLib.TagGroups), len(allLib.Tags))
	}
}

func TestWriterSanitizesFolderIconColorAndMedia(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	imgPath := filepath.Join(srcDir, "photo.heic")
	pdfPath := filepath.Join(srcDir, "doc.pdf")
	vidPath := filepath.Join(srcDir, "clip.mov")
	for _, p := range []string{imgPath, pdfPath, vidPath} {
		if err := os.WriteFile(p, []byte("payload:"+filepath.Base(p)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	lib := &domain.Library{
		Name: "SanitizeTest",
		TagGroups: []*domain.TagGroup{
			{ID: "g1", Name: "蓝色组", Color: "#3070e0"},
			{ID: "g2", Name: "红色组", Color: "#f25555"},
			{ID: "g3", Name: "无效组", Color: "not-a-color"},
			{ID: "g4", Name: "灰组", Color: "#d4d4d4"},
		},
		Folders: []*domain.Folder{
			{ID: "f_refto", Name: "ReftoGlobal", Icon: "global", Color: "#3070e0"},
			{ID: "f_eagle", Name: "EagleStar", Icon: "star", IconColor: "unknown"},
			{ID: "f_hexword", Name: "HexWord", Icon: "cafe"},
			{ID: "f_num", Name: "BillfishNum", Icon: "12"},
			{ID: "f_svg", Name: "BuiltinSVG", Icon: ":script:", IconColor: "purple"},
			{ID: "f_emoji1", Name: "EmojiSingle", Icon: "1F600", IconColor: "RED"},
			{ID: "f_emoji2", Name: "EmojiMulti", Icon: "1f576-fe0f", IconColor: "grey"},
			{ID: "f_keycap", Name: "EmojiKeycap", Icon: "0031-fe0f-20e3"},
		},
		Assets: []*domain.Asset{
			{
				ID:         "a_img",
				FileName:   "photo.heic",
				SourcePath: imgPath,
				Kind:       domain.KindImage,
				Width:      4000,
				Height:     3000,
				CreatedAt:  now,
				ModifiedAt: now,
				MediaMeta:  &domain.MediaMeta{Format: "heic"},
			},
			{
				ID:         "a_pdf",
				FileName:   "doc.pdf",
				SourcePath: pdfPath,
				Kind:       domain.KindOther,
				CreatedAt:  now,
				ModifiedAt: now,
				MediaMeta:  &domain.MediaMeta{Format: "pdf"},
			},
			{
				ID:          "a_vid",
				FileName:    "clip.mov",
				SourcePath:  vidPath,
				Kind:        domain.KindVideo,
				Width:       1920,
				Height:      1080,
				DurationSec: 2.5,
				CreatedAt:   now,
				ModifiedAt:  now,
				MediaMeta:   &domain.MediaMeta{Format: "mov,mp4,m4a,3gp,3g2,mj2", Bitrate: 1000000},
			},
		},
	}

	ctx := context.Background()
	outDir := filepath.Join(tmpDir, "out.pixcall")
	adapter := pixcall.New()
	writer, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{CopyMode: format.CopyModeCopy})
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := writer.Write(ctx, lib, nil); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, closer, err := kit.OpenReadOnly(filepath.Join(outDir, ".pixcall", "database", "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()

	type folderRow struct {
		Name      string
		Icon      *string
		IconColor *string
	}
	var folderRows []folderRow
	if err := db.Table("folders").
		Select("entries.name AS name, folders.icon AS icon, folders.icon_color AS icon_color").
		Joins("JOIN entries ON entries.id = folders.entry_id").
		Where("entries.id > ?", 2).
		Find(&folderRows).Error; err != nil {
		t.Fatalf("query folders: %v", err)
	}
	gotFolders := make(map[string]folderRow, len(folderRows))
	for _, r := range folderRows {
		gotFolders[r.Name] = r
	}

	deref := func(p *string) string {
		if p == nil {
			return "<nil>"
		}
		return *p
	}
	wantFolders := map[string][2]string{
		"ReftoGlobal": {"<nil>", "blue"},
		"EagleStar":   {"<nil>", "<nil>"},
		"HexWord":     {"<nil>", "<nil>"},
		"BillfishNum": {"<nil>", "<nil>"},
		"BuiltinSVG":  {":script:", "purple"},
		"EmojiSingle": {"1f600", "red"},
		"EmojiMulti":  {"1f576-fe0f", "gray"},
		"EmojiKeycap": {"0031-fe0f-20e3", "<nil>"},
	}
	for name, want := range wantFolders {
		row, ok := gotFolders[name]
		if !ok {
			t.Fatalf("missing folder %q in DB", name)
		}
		if gotIcon, gotColor := deref(row.Icon), deref(row.IconColor); gotIcon != want[0] || gotColor != want[1] {
			t.Errorf("folder %q: got (icon=%q, color=%q), want (%q, %q)", name, gotIcon, gotColor, want[0], want[1])
		}
	}

	var groupRows []model.TagGroups
	if err := db.Find(&groupRows).Error; err != nil {
		t.Fatalf("query tag_groups: %v", err)
	}
	gotGroups := make(map[string]string, len(groupRows))
	for _, g := range groupRows {
		gotGroups[g.Name] = deref(g.Color)
	}
	wantGroups := map[string]string{
		"蓝色组": "blue",
		"红色组": "red",
		"无效组": "<nil>",
		"灰组":  "<nil>",
	}
	for name, want := range wantGroups {
		if got := gotGroups[name]; got != want {
			t.Errorf("tag_group %q color = %q, want %q", name, got, want)
		}
	}

	type mediaEntryRow struct {
		Name string
		Kind string
	}
	var mediaRows []mediaEntryRow
	if err := db.Table("media").
		Select("entries.name AS name, entries.content_type AS kind").
		Joins("JOIN entries ON entries.id = media.entry_id").
		Find(&mediaRows).Error; err != nil {
		t.Fatalf("query media: %v", err)
	}
	if len(mediaRows) != 1 || mediaRows[0].Name != "clip.mov" {
		t.Fatalf("media table rows = %+v, want only clip.mov", mediaRows)
	}
}
