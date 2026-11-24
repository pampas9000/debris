package refto_test

import (
	"context"
	"encoding/json/v2"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/refto"
	"debris/internal/format/refto/model"
	"debris/internal/kit"
)

const realSampleReftoPath = "/Users/kazuha/Pictures/Refto/My Library.reflib"

func TestAdapterMetadata(t *testing.T) {
	adapter := refto.New()
	if adapter.ID() != "refto" {
		t.Fatalf("ID() = %q, want refto", adapter.ID())
	}
	if adapter.DisplayName() != "Refto" {
		t.Fatalf("DisplayName() = %q, want Refto", adapter.DisplayName())
	}
	caps := adapter.Capabilities()
	if !caps.SupportsFolders || !caps.SupportsNestedFolders || !caps.SupportsMultiFolderAsset ||
		!caps.SupportsTagGroups || !caps.SupportsColors || !caps.SupportsRecycleBin ||
		!caps.SupportsThumbnails || !caps.SupportsRating || !caps.SupportsOriginURL ||
		!caps.SupportsAnnotation {
		t.Fatalf("unexpected false capability in Refto Capabilities(): %+v", caps)
	}
	if caps.SupportsNestedTags {
		t.Fatalf("SupportsNestedTags should be false for Refto")
	}
}

func TestColorHelpersAndOKLab(t *testing.T) {
	packed := model.PackRGB(36, 64, 55)
	if packed != 2375735 {
		t.Fatalf("PackRGB(36, 64, 55) = %d, want 2375735", packed)
	}
	r, g, b := model.UnpackRGB(packed)
	if r != 36 || g != 64 || b != 55 {
		t.Fatalf("UnpackRGB(2375735) = (%d,%d,%d), want (36,64,55)", r, g, b)
	}
	if hex := model.RGBToHex(r, g, b); hex != "#244037" {
		t.Fatalf("RGBToHex = %q, want #244037", hex)
	}
	l, a, okb := model.SRGBToOKLab(36, 64, 55)
	if math.Abs(l-0.34604) > 1e-4 || math.Abs(a-(-0.03739)) > 1e-4 || math.Abs(okb-0.00557) > 1e-4 {
		t.Fatalf("SRGBToOKLab(36,64,55) = (%f, %f, %f)", l, a, okb)
	}
}

func TestProbe(t *testing.T) {
	adapter := refto.New()
	ctx := context.Background()

	t.Run("empty and missing", func(t *testing.T) {
		res, err := adapter.Probe(ctx, "")
		if err != nil || res.Matched {
			t.Fatalf("empty Probe = (%+v, %v)", res, err)
		}
		res, err = adapter.Probe(ctx, filepath.Join(t.TempDir(), "missing"))
		if err != nil || res.Matched {
			t.Fatalf("missing Probe = (%+v, %v)", res, err)
		}
	})

	t.Run("file instead of dir", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "file.txt")
		_ = os.WriteFile(f, []byte("x"), 0o644)
		res, err := adapter.Probe(ctx, f)
		if err != nil || res.Matched {
			t.Fatalf("file Probe = (%+v, %v)", res, err)
		}
	})

	t.Run("missing act.db", func(t *testing.T) {
		d := t.TempDir()
		_ = os.WriteFile(filepath.Join(d, "act-library.json"), []byte(`{"schema":1,"name":"X"}`), 0o644)
		res, err := adapter.Probe(ctx, d)
		if err != nil || res.Matched {
			t.Fatalf("missing act.db Probe = (%+v, %v)", res, err)
		}
	})

	t.Run("act.db only without manifest", func(t *testing.T) {
		d := filepath.Join(t.TempDir(), "Sketch.reflib")
		_ = os.MkdirAll(d, 0o755)
		db, closer, err := kit.OpenReadWrite(filepath.Join(d, "act.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Migrate(db); err != nil {
			_ = closer.Close()
			t.Fatal(err)
		}
		_ = closer.Close()

		res, err := adapter.Probe(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Matched || res.Format != "refto" || res.Confidence != 90 || res.Name != "Sketch" || res.Version != "0.1.0-beta.10" {
			t.Fatalf("unexpected ProbeResult: %+v", res)
		}
	})

	t.Run("valid with manifest", func(t *testing.T) {
		d := filepath.Join(t.TempDir(), "RefLib.reflib")
		_ = os.MkdirAll(d, 0o755)
		db, closer, err := kit.OpenReadWrite(filepath.Join(d, "act.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Migrate(db); err != nil {
			_ = closer.Close()
			t.Fatal(err)
		}
		_ = closer.Close()

		manifest := model.LibraryManifest{
			Schema:     1,
			ID:         kit.NewULID(time.Now()),
			Name:       "Studio Reference",
			CreatedAt:  time.Now().UTC(),
			AppVersion: "0.1.0-beta.10",
			Storage:    "managed",
		}
		if err := kit.WriteJSON(filepath.Join(d, "act-library.json"), manifest); err != nil {
			t.Fatal(err)
		}

		res, err := adapter.Probe(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if !res.Matched || res.Confidence != 100 || res.Name != "Studio Reference" || res.Version != "0.1.0-beta.10" {
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

func TestReadSpecialCasesAndWarnings(t *testing.T) {
	ctx := context.Background()
	libDir := filepath.Join(t.TempDir(), "Full.reflib")
	extDir := t.TempDir()
	_ = os.MkdirAll(libDir, 0o755)

	// External linked file (existing)
	linkedOK := filepath.Join(extDir, "linked_ok.png")
	_ = os.WriteFile(linkedOK, []byte("linked-bytes"), 0o644)
	linkedMissing := filepath.Join(extDir, "linked_missing.png")

	db, closer, err := kit.OpenReadWrite(filepath.Join(libDir, "act.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Migrate(db); err != nil {
		_ = closer.Close()
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	nowMs := now.UnixMilli()

	// Items
	idManaged := "01M84G2BZWA8R1Y5AZ18AEQK95"
	idLinkedOK := "01M84G2BZWA8R1Y5AZ18AEQK96"
	idLinkedMiss := "01M84G2BZWA8R1Y5AZ18AEQK97"
	idArchived := "01M84G2BZWA8R1Y5AZ18AEQK98"
	idDeleted := "01M84G2BZWA8R1Y5AZ18AEQK99"

	// Create managed item file, custom cover, and thumb
	itemDir := filepath.Join(libDir, "items", idManaged)
	_ = os.MkdirAll(itemDir, 0o755)
	_ = os.WriteFile(filepath.Join(itemDir, "hero.jpg"), []byte("hero-jpg-data"), 0o644)
	_ = os.WriteFile(filepath.Join(itemDir, "custom_cover.jpg"), []byte("cover-jpg"), 0o644)

	thumbDir := filepath.Join(libDir, "thumbs", "96")
	_ = os.MkdirAll(thumbDir, 0o755)
	_ = os.WriteFile(filepath.Join(thumbDir, idLinkedOK+".jpg"), []byte("thumb-96"), 0o644)

	// Managed archived & deleted files
	_ = os.MkdirAll(filepath.Join(libDir, "items", idArchived), 0o755)
	_ = os.WriteFile(filepath.Join(libDir, "items", idArchived, "arch.png"), []byte("arch"), 0o644)
	_ = os.MkdirAll(filepath.Join(libDir, "items", idDeleted), 0o755)
	_ = os.WriteFile(filepath.Join(libDir, "items", idDeleted, "trash.png"), []byte("trash"), 0o644)

	items := []model.Item{
		{
			RID:          1,
			ID:           idManaged,
			Kind:         "image",
			Ext:          "jpg",
			Mime:         "image/jpeg",
			Name:         "hero",
			FileName:     "hero.jpg",
			Storage:      "managed",
			RelPath:      "hero.jpg",
			Size:         13,
			Hash:         "sha256:abc",
			Width:        100,
			Height:       200,
			Meta:         `{"v":1,"container":"jpeg","exif":{"Model":"iPhone 15"}}`,
			Rating:       4,
			Note:         "hero note",
			SourceURL:    "https://example.com/hero",
			AddedAt:      nowMs,
			UpdatedAt:    nowMs,
			PrimaryColor: "#244037",
			ThumbState:   "ready",
			ThumbExt:     "jpg",
			PinnedAt:     nowMs,
			Cover:        "custom_cover.jpg",
			StackID:      "stack-1",
			StackPos:     0,
		},
		{
			RID:        2,
			ID:         idLinkedOK,
			Kind:       "image",
			Ext:        "png",
			Name:       "linked_ok",
			FileName:   "linked_ok.png",
			Storage:    "linked",
			AbsPath:    linkedOK,
			Size:       12,
			AddedAt:    nowMs + 1,
			UpdatedAt:  nowMs + 1,
			ThumbState: "ready",
			ThumbExt:   "jpg",
			Live:       "live.mov",
			LiveID:     "live-1",
		},
		{
			RID:        3,
			ID:         idLinkedMiss,
			Kind:       "image",
			Ext:        "png",
			Name:       "linked_missing",
			FileName:   "linked_missing.png",
			Storage:    "linked",
			AbsPath:    linkedMissing,
			Size:       99,
			AddedAt:    nowMs + 2,
			UpdatedAt:  nowMs + 2,
			ThumbState: "pending",
			AIState:    "pending",
			Missing:    1,
		},
		{
			RID:        4,
			ID:         idArchived,
			Kind:       "image",
			Ext:        "png",
			Name:       "arch",
			FileName:   "arch.png",
			Storage:    "managed",
			RelPath:    "arch.png",
			Size:       4,
			AddedAt:    nowMs + 3,
			UpdatedAt:  nowMs + 3,
			ThumbState: "pending",
			AIState:    "pending",
			ArchivedAt: nowMs + 3,
		},
		{
			RID:        5,
			ID:         idDeleted,
			Kind:       "image",
			Ext:        "png",
			Name:       "trash",
			FileName:   "trash.png",
			Storage:    "managed",
			RelPath:    "trash.png",
			Size:       5,
			AddedAt:    nowMs + 4,
			UpdatedAt:  nowMs + 4,
			ThumbState: "pending",
			AIState:    "pending",
			DeletedAt:  &nowMs,
		},
	}
	for _, it := range items {
		if err := db.Create(&it).Error; err != nil {
			_ = closer.Close()
			t.Fatal(err)
		}
	}

	// Tag groups & tags
	grpID := "01M84G2C7Y3KZP67FFJ3JFV001"
	_ = db.Create(&model.TagGroup{ID: grpID, Name: "Styles", Color: "blue", Position: 1, CreatedAt: nowMs}).Error

	tAccID := "01M84G2C7Y3KZP67FFJ3JFV010"
	tSugID := "01M84G2C7Y3KZP67FFJ3JFV011"
	tRejID := "01M84G2C7Y3KZP67FFJ3JFV012"
	_ = db.Create(&model.Tag{ID: tAccID, Name: "户外", Pinyin: "huwai hw", Starred: 1, GroupID: &grpID, CreatedAt: nowMs, UpdatedAt: nowMs}).Error
	_ = db.Create(&model.Tag{ID: tSugID, Name: "AI建议", Pinyin: "aijianyi ajy", CreatedAt: nowMs, UpdatedAt: nowMs}).Error
	_ = db.Create(&model.Tag{ID: tRejID, Name: "已拒绝", CreatedAt: nowMs, UpdatedAt: nowMs}).Error
	_ = db.Exec("INSERT INTO suggestion_tags(tag_id) VALUES (?)", tSugID).Error

	_ = db.Create(&model.ItemTag{ItemID: idManaged, TagID: tAccID, Source: "user", State: "accepted", Confidence: 1, CreatedAt: nowMs}).Error
	_ = db.Create(&model.ItemTag{ItemID: idManaged, TagID: tSugID, Source: "vision", State: "suggested", Confidence: 0.5, CreatedAt: nowMs}).Error
	_ = db.Create(&model.ItemTag{ItemID: idManaged, TagID: tRejID, Source: "vision", State: "rejected", Confidence: 0.2, CreatedAt: nowMs}).Error

	// Folders
	fRootID := "01M84G2C7Y3KZP67FFJ3JFV100"
	fChildID := "01M84G2C7Y3KZP67FFJ3JFV101"
	_ = db.Create(&model.Folder{
		ID:          fRootID,
		Position:    0,
		Name:        "Root",
		Description: "Top",
		AutoTags:    `["户外"]`,
		CoverItemID: &idManaged,
		CreatedAt:   nowMs,
		UpdatedAt:   nowMs,
	}).Error
	_ = db.Create(&model.Folder{
		ID:        fChildID,
		ParentID:  &fRootID,
		Position:  1,
		Name:      "Child",
		CreatedAt: nowMs,
		UpdatedAt: nowMs,
	}).Error
	_ = db.Create(&model.ItemFolder{ItemID: idManaged, FolderID: fRootID, AddedAt: nowMs, Position: 2.5}).Error
	_ = db.Create(&model.ItemFolder{ItemID: idManaged, FolderID: fChildID, AddedAt: nowMs, Position: 1.0}).Error

	// Palette
	_ = db.Create(&model.Palette{
		ItemID: idManaged,
		Rank:   0,
		RGB:    2375735,
		L:      0.346,
		A:      -0.037,
		B:      0.005,
		Ratio:  0.267,
	}).Error

	// Optional unsupported objects for warning checks
	_ = db.Exec(`INSERT INTO versions(id, item_id, n, storage, created_at) VALUES ('v1', ?, 1, 'managed', ?)`, idManaged, nowMs).Error
	_ = db.Exec(`INSERT INTO smart_folders(id, name, created_at, updated_at) VALUES ('sf1', 'Smart', ?, ?)`, nowMs, nowMs).Error
	_ = db.Exec(`INSERT INTO boards(id, name, created_at, updated_at) VALUES ('b1', 'Board', ?, ?)`, nowMs, nowMs).Error
	_ = closer.Close()

	adapter := refto.New()

	// Default ReadOptions: excludes deleted, archived, suggested tags
	rDefault, err := adapter.OpenReader(ctx, libDir, format.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer rDefault.Close()

	insp, err := rDefault.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if insp.AssetCount != 3 || insp.DeletedCount != 1 || insp.HiddenCount != 1 || insp.FolderCount != 2 || insp.TagGroupCount != 1 || insp.TagCount != 1 {
		t.Fatalf("unexpected InspectResult: %+v", insp)
	}
	joinedWarnings := strings.Join(insp.Warnings, "\n")
	for _, wantSub := range []string{"AI-suggested", "linked asset(s) are missing", "version(s)", "stack(s)", "Live Photo", "smart folder(s)", "whiteboard(s)"} {
		if !strings.Contains(joinedWarnings, wantSub) {
			t.Errorf("expected Inspect warning containing %q, got:\n%s", wantSub, joinedWarnings)
		}
	}

	lib, err := rDefault.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Assets) != 3 || len(lib.Tags) != 1 || lib.Tags[0].Name != "户外" || !lib.Tags[0].Starred {
		t.Fatalf("unexpected default Read tags/assets: tags=%+v assets=%d", lib.Tags, len(lib.Assets))
	}
	if len(lib.Folders) != 1 || len(lib.Folders[0].Children) != 1 || lib.Folders[0].CoverAssetID != idManaged || !slices.Equal(lib.Folders[0].TagNames, []string{"户外"}) {
		t.Fatalf("unexpected folder tree: %+v", lib.Folders)
	}

	byID := map[string]*domain.Asset{}
	for _, a := range lib.Assets {
		byID[a.ID] = a
	}
	hero := byID[idManaged]
	if hero == nil || hero.Hash != "sha256:abc" || hero.Rating != 4 || hero.Annotation != "hero note" || hero.OriginURL != "https://example.com/hero" {
		t.Fatalf("unexpected hero asset: %+v", hero)
	}
	if len(hero.Colors) != 1 || hero.Colors[0].R != 36 || hero.Colors[0].G != 64 || hero.Colors[0].B != 55 || hero.Colors[0].Ratio != 26.7 {
		t.Fatalf("unexpected hero colors: %+v", hero.Colors)
	}
	if hero.MediaMeta == nil || filepath.Base(hero.MediaMeta.ThumbnailPath) != "custom_cover.jpg" || hero.MediaMeta.Format != "jpeg" || hero.MediaMeta.EXIF["Model"] != "iPhone 15" {
		t.Fatalf("unexpected hero MediaMeta: %+v", hero.MediaMeta)
	}
	if !slices.Equal(hero.TagIDs, []string{tAccID}) {
		t.Fatalf("hero.TagIDs = %v, want [%s]", hero.TagIDs, tAccID)
	}
	lOK := byID[idLinkedOK]
	if lOK == nil || lOK.SourcePath != linkedOK || lOK.MediaMeta == nil || !strings.HasSuffix(lOK.MediaMeta.ThumbnailPath, filepath.Join("thumbs", "96", idLinkedOK+".jpg")) {
		t.Fatalf("unexpected linkedOK asset: %+v", lOK)
	}

	// IncludeSuggestedTags + IncludeDeleted + IncludeHidden
	rAll, err := adapter.OpenReader(ctx, libDir, format.ReadOptions{
		IncludeDeleted:       true,
		IncludeHidden:        true,
		IncludeSuggestedTags: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rAll.Close()
	libAll, err := rAll.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(libAll.Assets) != 5 {
		t.Fatalf("len(Assets) with all flags = %d, want 5", len(libAll.Assets))
	}
	// Rejected tag (tRejID) should still NOT be on hero, while suggested tag (tSugID) IS on hero.
	var heroAll *domain.Asset
	for _, a := range libAll.Assets {
		if a.ID == idManaged {
			heroAll = a
		}
	}
	if heroAll == nil || !slices.Equal(heroAll.TagIDs, []string{tAccID, tSugID}) {
		t.Fatalf("heroAll.TagIDs = %v, want [%s %s]", heroAll.TagIDs, tAccID, tSugID)
	}
}

func TestWriteAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()

	imgPath := filepath.Join(srcDir, "design.png")
	_ = os.WriteFile(imgPath, []byte("png-payload"), 0o644)

	// Create valid PNG thumbnail to verify PNG -> JPEG conversion into thumbs/.
	pngThumb := filepath.Join(srcDir, "thumb.png")
	im := image.NewRGBA(image.Rect(0, 0, 16, 10))
	im.Set(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	tf, err := os.Create(pngThumb)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(tf, im); err != nil {
		_ = tf.Close()
		t.Fatal(err)
	}
	_ = tf.Close()

	t1 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)

	lib := &domain.Library{
		Format:    "eagle",
		Name:      "RoundTrip",
		CreatedAt: t1,
		Folders: []*domain.Folder{
			{
				ID:           "f1",
				Name:         "Parent",
				Description:  "Desc",
				Seq:          1,
				TagNames:     []string{"建筑结构"},
				CoverAssetID: "a1",
				Children: []*domain.Folder{
					{ID: "f2", ParentID: "f1", Name: "Child", Seq: 2},
				},
			},
		},
		TagGroups: []*domain.TagGroup{
			{ID: "g1", Name: "Group1", Color: "red", Ranking: 1},
		},
		Tags: []*domain.Tag{
			{ID: "t1", GroupID: "g1", Name: "建筑结构", Starred: true, CreatedAt: t1},
			{ID: "t2", Name: "Design", CreatedAt: t1},
			{ID: "t3", Name: "design", CreatedAt: t1}, // case-insensitive duplicate of t2
		},
		Assets: []*domain.Asset{
			{
				ID:         "a1",
				FileName:   "design.png",
				Name:       "design",
				Ext:        "png",
				SourcePath: imgPath,
				Size:       11,
				Kind:       domain.KindImage,
				Width:      800,
				Height:     600,
				Rating:     5,
				Annotation: "hello world",
				OriginURL:  "https://example.com",
				Colors:     []domain.PaletteColor{{R: 36, G: 64, B: 55, Ratio: 50}},
				FolderIDs:  []string{"f1", "f2"},
				TagIDs:     []string{"t1", "t2", "t3"},
				CreatedAt:  t1,
				ModifiedAt: t2,
				MediaMeta: &domain.MediaMeta{
					ThumbnailPath: pngThumb,
					EXIF:          map[string]any{"Camera": "Test"},
				},
			},
			{
				ID:         "a-missing",
				Name:       "ghost",
				Ext:        "png",
				SourcePath: filepath.Join(srcDir, "missing.png"),
			},
		},
	}

	adapter := refto.New()
	for _, mode := range []format.CopyMode{format.CopyModeCopy, format.CopyModeHardlink} {
		t.Run(string(mode), func(t *testing.T) {
			outDir := filepath.Join(t.TempDir(), "out.reflib")
			w, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{CopyMode: mode})
			if err != nil {
				t.Fatal(err)
			}
			var events []format.ProgressEvent
			if err := w.Write(ctx, lib, func(ev format.ProgressEvent) {
				events = append(events, ev)
			}); err != nil {
				_ = w.Close()
				t.Fatalf("Write(%s) failed: %v", mode, err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}

			// Verify sidecars exist and are valid JSON.
			for _, f := range []string{"act-library.json", "tags.json", "act.db"} {
				if _, err := os.Stat(filepath.Join(outDir, f)); err != nil {
					t.Fatalf("missing %s: %v", f, err)
				}
			}

			// Verify FTS5 delete trigger works on act.db (confirms SQLite >= 3.43 + trigger validity).
			db, closer, err := kit.OpenReadWrite(filepath.Join(outDir, "act.db"))
			if err != nil {
				t.Fatal(err)
			}
			var uv int
			_ = db.Raw("PRAGMA user_version").Row().Scan(&uv)
			if uv != 25 {
				_ = closer.Close()
				t.Fatalf("user_version = %d, want 25", uv)
			}
			var lgen int
			_ = db.Raw("SELECT n FROM listing_gen WHERE id = 1").Row().Scan(&lgen)
			if lgen <= 1 {
				_ = closer.Close()
				t.Fatalf("listing_gen n = %d, expected > 1", lgen)
			}
			// Test trigger in rollback transaction so we don't mutate output.
			_ = db.Exec("BEGIN").Error
			if err := db.Exec("DELETE FROM items WHERE rid = 1").Error; err != nil {
				_ = db.Exec("ROLLBACK")
				_ = closer.Close()
				t.Fatalf("DELETE FROM items (FTS5 trigger) failed: %v", err)
			}
			_ = db.Exec("ROLLBACK")
			_ = closer.Close()

			// Read back and verify round-trip fidelity.
			r, err := adapter.OpenReader(ctx, outDir, format.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()

			got, err := r.Read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got.Name != "RoundTrip" || len(got.Assets) != 1 || len(got.Tags) != 2 || len(got.TagGroups) != 1 {
				t.Fatalf("unexpected round-trip counts: name=%q assets=%d tags=%d groups=%d",
					got.Name, len(got.Assets), len(got.Tags), len(got.TagGroups))
			}
			if len(got.Folders) != 1 || len(got.Folders[0].Children) != 1 || got.Folders[0].CoverAssetID != got.Assets[0].ID {
				t.Fatalf("unexpected round-trip folders: %+v", got.Folders)
			}
			a := got.Assets[0]
			if !strings.HasPrefix(a.Hash, "sha256:") || a.Rating != 5 || a.Annotation != "hello world" || a.OriginURL != "https://example.com" {
				t.Fatalf("unexpected round-trip asset: %+v", a)
			}
			if len(a.Colors) != 1 || a.Colors[0].R != 36 || a.Colors[0].G != 64 || a.Colors[0].B != 55 || a.Colors[0].Ratio != 50 {
				t.Fatalf("unexpected round-trip colors: %+v", a.Colors)
			}
			if a.MediaMeta == nil || a.MediaMeta.ThumbnailPath == "" || a.MediaMeta.EXIF["Camera"] != "Test" {
				t.Fatalf("unexpected round-trip MediaMeta: %+v", a.MediaMeta)
			}
			// Verify written thumbnail is valid JPEG 16x10.
			jf, err := os.Open(a.MediaMeta.ThumbnailPath)
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := jpeg.DecodeConfig(jf)
			_ = jf.Close()
			if err != nil || cfg.Width != 16 || cfg.Height != 10 {
				t.Fatalf("unexpected Refto thumbnail cfg=%+v err=%v", cfg, err)
			}

			// Verify item.json sidecar exists
			rawItemJSON, err := os.ReadFile(filepath.Join(outDir, "items", a.ID, "item.json"))
			if err != nil {
				t.Fatal(err)
			}
			var itemSidecar model.ItemSidecar
			if err := json.Unmarshal(rawItemJSON, &itemSidecar); err != nil {
				t.Fatal(err)
			}
			if itemSidecar.Item.ID != a.ID || len(itemSidecar.Folders) != 2 || len(itemSidecar.Tags) != 2 {
				t.Fatalf("unexpected item.json sidecar: %+v", itemSidecar)
			}

			// Verify hardlink shared inode
			if mode == format.CopyModeHardlink {
				srcSt, _ := os.Stat(imgPath)
				dstSt, _ := os.Stat(a.SourcePath)
				if !os.SameFile(srcSt, dstSt) {
					t.Fatal("hardlink mode did not preserve inode")
				}
			}
		})
	}
}

func TestRealSampleReftoLibrary(t *testing.T) {
	if _, err := os.ReadDir(realSampleReftoPath); err != nil {
		t.Skipf("real sample Refto library not accessible (%v)", err)
	}

	tmpCopy := filepath.Join(t.TempDir(), "My Library.reflib")
	if err := copyDirFiltered(realSampleReftoPath, tmpCopy); err != nil {
		t.Skipf("copy real sample skipped: %v", err)
	}

	ctx := context.Background()
	adapter := refto.New()
	probeRes, err := adapter.Probe(ctx, tmpCopy)
	if err != nil || !probeRes.Matched || probeRes.Format != "refto" {
		t.Fatalf("Probe real sample = (%+v, %v)", probeRes, err)
	}

	rDefault, err := adapter.OpenReader(ctx, tmpCopy, format.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer rDefault.Close()
	inspDefault, err := rDefault.Inspect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if inspDefault.AssetCount <= 0 || inspDefault.TotalBytes <= 0 {
		t.Fatalf("expected non-empty real sample library, got %+v", inspDefault)
	}

	rSuggested, err := adapter.OpenReader(ctx, tmpCopy, format.ReadOptions{IncludeSuggestedTags: true})
	if err != nil {
		t.Fatal(err)
	}
	defer rSuggested.Close()
	libSug, err := rSuggested.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(libSug.Assets) != inspDefault.AssetCount || len(libSug.Tags) < inspDefault.TagCount {
		t.Fatalf("unexpected real sample counts with suggested tags: assets=%d (want %d) tags=%d (want >= %d)",
			len(libSug.Assets), inspDefault.AssetCount, len(libSug.Tags), inspDefault.TagCount)
	}
}

func copyDirFiltered(src, dst string) error {
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
		if !d.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func TestWriterSanitizesColorsIconsPositionsAndTimestamps(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	imgPath := filepath.Join(srcDir, "cover.png")
	if err := os.WriteFile(imgPath, []byte("png-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := time.Date(2026, 11, 20, 12, 0, 0, 123000000, time.UTC)
	lib := &domain.Library{
		Name:      "SanitizeRefto",
		CreatedAt: ts,
		TagGroups: []*domain.TagGroup{
			{ID: "g2", Name: "AquaGroup", Color: "aqua", Ranking: 2000000000},
			{ID: "g1", Name: "BlueGroup", Color: "blue", Ranking: 1000000000},
			{ID: "g3", Name: "BadColorGroup", Color: "not-a-color", Ranking: 3000000000},
		},
		Tags: []*domain.Tag{
			{ID: "t1", GroupID: "g1", Name: "PurpleTag", Color: "purple", CreatedAt: ts},
			{ID: "t2", GroupID: "g2", Name: "ShortHexTag", Color: "#ABC", CreatedAt: ts},
			{ID: "t3", GroupID: "g3", Name: "BadTag", Color: "bogus", CreatedAt: ts},
		},
		Folders: []*domain.Folder{
			{
				ID:           "f2",
				Name:         "PixcallIconFolder",
				Icon:         ":script:",
				IconColor:    "blue",
				Seq:          9000000000000000000,
				CoverAssetID: "a1",
				CreatedAt:    ts,
				ModifiedAt:   ts,
			},
			{
				ID:         "f1",
				Name:       "ReftoIconFolder",
				Icon:       "global",
				Color:      "#3070E0",
				Seq:        100,
				CreatedAt:  ts,
				ModifiedAt: ts,
			},
			{
				ID:        "f3",
				Name:      "EmojiIconFolder",
				Icon:      "1f600",
				Color:     "grey",
				Seq:       9100000000000000000,
				CreatedAt: ts,
				ModifiedAt: ts,
			},
			{
				ID:        "f4",
				Name:      "NumIconFolder",
				Icon:      "12",
				Color:     "unknown-color",
				Seq:       9200000000000000000,
				CreatedAt: ts,
				ModifiedAt: ts,
			},
		},
		Assets: []*domain.Asset{
			{
				ID:         "a1",
				FileName:   "cover.png",
				SourcePath: imgPath,
				Kind:       domain.KindImage,
				FolderIDs:  []string{"f2"},
				TagIDs:     []string{"t1", "t2", "t3"},
				CreatedAt:  ts,
				ModifiedAt: ts,
			},
		},
	}

	outDir := filepath.Join(tmpDir, "out.reflib")
	adapter := refto.New()
	w, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{CopyMode: format.CopyModeCopy})
	if err != nil {
		t.Fatalf("OpenWriter: %v", err)
	}
	if err := w.Write(ctx, lib, nil); err != nil {
		_ = w.Close()
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	db, closer, err := kit.OpenReadOnly(filepath.Join(outDir, "act.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = closer.Close() }()

	var folders []model.Folder
	if err := db.Order("position ASC").Find(&folders).Error; err != nil {
		t.Fatalf("query folders: %v", err)
	}
	if len(folders) != 4 {
		t.Fatalf("folders len = %d, want 4", len(folders))
	}
	wantFolders := []struct {
		name  string
		icon  string
		color string
		pos   int
		cover bool
	}{
		{name: "ReftoIconFolder", icon: "global", color: "#3070e0", pos: 0},
		{name: "PixcallIconFolder", icon: "", color: "#3f81ff", pos: 1, cover: true},
		{name: "EmojiIconFolder", icon: "", color: "#8e8e93", pos: 2},
		{name: "NumIconFolder", icon: "", color: "", pos: 3},
	}
	for i, w := range wantFolders {
		got := folders[i]
		if got.Name != w.name || got.Icon != w.icon || got.Color != w.color || got.Position != w.pos {
			t.Errorf("folder[%d] = (name=%q, icon=%q, color=%q, pos=%d), want (name=%q, icon=%q, color=%q, pos=%d)",
				i, got.Name, got.Icon, got.Color, got.Position, w.name, w.icon, w.color, w.pos)
		}
		if w.cover && (got.CoverItemID == nil || *got.CoverItemID == "") {
			t.Errorf("folder %q missing cover_item_id", got.Name)
		}
		if got.CreatedAt < 1_000_000_000_000 || got.UpdatedAt < 1_000_000_000_000 {
			t.Errorf("folder %q timestamps not in ms: created=%d updated=%d", got.Name, got.CreatedAt, got.UpdatedAt)
		}
	}

	var groups []model.TagGroup
	if err := db.Order("position ASC").Find(&groups).Error; err != nil {
		t.Fatalf("query tag_groups: %v", err)
	}
	wantGroups := []struct {
		name  string
		color string
		pos   int
	}{
		{name: "BlueGroup", color: "#3f81ff", pos: 0},
		{name: "AquaGroup", color: "#5fcbed", pos: 1},
		{name: "BadColorGroup", color: "", pos: 2},
	}
	for i, w := range wantGroups {
		if groups[i].Name != w.name || groups[i].Color != w.color || groups[i].Position != w.pos {
			t.Errorf("tag_group[%d] = (%q, %q, %d), want (%q, %q, %d)",
				i, groups[i].Name, groups[i].Color, groups[i].Position, w.name, w.color, w.pos)
		}
	}

	var tags []model.Tag
	if err := db.Find(&tags).Error; err != nil {
		t.Fatalf("query tags: %v", err)
	}
	tagColors := make(map[string]string, len(tags))
	for _, tg := range tags {
		tagColors[tg.Name] = tg.Color
	}
	if tagColors["PurpleTag"] != "#8473ff" || tagColors["ShortHexTag"] != "#aabbcc" || tagColors["BadTag"] != "" {
		t.Errorf("unexpected act.db tag colors: %+v", tagColors)
	}

	rawTagsJSON, err := os.ReadFile(filepath.Join(outDir, "tags.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sidecar model.TagsSidecar
	if err := json.Unmarshal(rawTagsJSON, &sidecar); err != nil {
		t.Fatal(err)
	}
	if len(sidecar.Groups) != 3 || sidecar.Groups[0].Color != "#3f81ff" || sidecar.Groups[2].Color != "" {
		t.Errorf("unexpected tags.json groups: %+v", sidecar.Groups)
	}
	sidecarTagColors := make(map[string]string, len(sidecar.Tags))
	for _, tg := range sidecar.Tags {
		sidecarTagColors[tg.Name] = tg.Color
	}
	if sidecarTagColors["PurpleTag"] != "#8473ff" || sidecarTagColors["ShortHexTag"] != "#aabbcc" || sidecarTagColors["BadTag"] != "" {
		t.Errorf("unexpected tags.json tag colors: %+v", sidecarTagColors)
	}
}
