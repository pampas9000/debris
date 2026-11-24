package refto

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"
	"gorm.io/gorm"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/refto/model"
	"debris/internal/kit"
)

type writer struct {
	rootPath string
	opts     format.WriteOptions
	db       *gorm.DB
	closer   io.Closer

	closeOnce sync.Once
	closeErr  error
}

func newWriter(ctx context.Context, path string, opts format.WriteOptions) (*writer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("refto: writer path is required")
	}

	mode, err := format.ParseCopyMode(string(opts.CopyMode))
	if err != nil {
		return nil, fmt.Errorf("refto: %w", err)
	}
	opts.CopyMode = mode

	return &writer{
		rootPath: resolvePath(trimmed),
		opts:     opts,
	}, nil
}

func (w *writer) Close() error {
	w.closeOnce.Do(func() {
		if w.db != nil {
			_ = w.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
		}
		if w.closer != nil {
			w.closeErr = w.closer.Close()
		}
	})
	return w.closeErr
}

func (w *writer) Write(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lib == nil {
		return errors.New("refto: library must not be nil")
	}

	emitEvent(emit, format.ProgressEvent{
		Kind:    "stage",
		Stage:   "init",
		Message: "initializing Refto library",
	})

	if err := w.prepareStorage(); err != nil {
		return err
	}

	db, closer, err := kit.OpenReadWrite(mainDBPath(w.rootPath))
	if err != nil {
		return fmt.Errorf("refto: open target act.db: %w", err)
	}
	w.db = db
	w.closer = closer

	if err := model.Migrate(db); err != nil {
		return err
	}

	now := time.Now().UTC()
	nowMs := now.UnixMilli()

	emitEvent(emit, format.ProgressEvent{
		Kind:    "stage",
		Stage:   "prepare",
		Message: "preparing Refto metadata",
	})

	// 1. Tag groups & tags (deduplicated case-insensitively to satisfy UNIQUE constraints).
	tagGroups, groupSidecars, groupIDMap := buildReftoTagGroups(lib.TagGroups, now)
	tags, tagSidecars, tagIDMap, tagInfoByID := buildReftoTags(lib.Tags, groupIDMap, now)

	// 2. Folders in topological parent-before-child order; cover_item_id applied after items.
	folders, folderIDMap, folderCoverRefs := buildReftoFolders(lib.Folders, nowMs)

	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, g := range tagGroups {
			if err := tx.Create(&g).Error; err != nil {
				return fmt.Errorf("refto: insert tag_group %q: %w", g.Name, err)
			}
		}
		for _, tg := range tags {
			if err := tx.Create(&tg).Error; err != nil {
				return fmt.Errorf("refto: insert tag %q: %w", tg.Name, err)
			}
		}
		for _, f := range folders {
			if err := tx.Create(&f).Error; err != nil {
				return fmt.Errorf("refto: insert folder %q: %w", f.Name, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// 3. Assign chronological ULIDs to assets so item order matches CreatedAt.
	assetTimes := make([]time.Time, len(lib.Assets))
	for i, a := range lib.Assets {
		if a != nil && !a.CreatedAt.IsZero() {
			assetTimes[i] = a.CreatedAt
		} else if a != nil && !a.ModifiedAt.IsZero() {
			assetTimes[i] = a.ModifiedAt
		} else {
			assetTimes[i] = now
		}
	}
	assetULIDs := kit.AssignULIDs(assetTimes)

	totalAssets := int64(len(lib.Assets))
	assetIDMap := make(map[string]string, len(lib.Assets))

	emitEvent(emit, format.ProgressEvent{
		Kind:    "stage",
		Stage:   "assets",
		Total:   totalAssets,
		Message: "transferring assets",
	})

	var nextRID int64 = 1
	for i, asset := range lib.Assets {
		if err := ctx.Err(); err != nil {
			return err
		}
		if asset == nil {
			emitEvent(emit, format.ProgressEvent{
				Kind:     "progress",
				Stage:    "assets",
				Current:  int64(i + 1),
				Total:    totalAssets,
				CopyMode: "skip",
			})
			continue
		}

		if strings.TrimSpace(asset.SourcePath) == "" {
			emitEvent(emit, format.ProgressEvent{
				Kind:    "warning",
				Stage:   "assets",
				AssetID: asset.ID,
				Message: fmt.Sprintf("skipping asset %q: empty sourcePath", asset.ID),
			})
			emitEvent(emit, format.ProgressEvent{
				Kind:     "progress",
				Stage:    "assets",
				Current:  int64(i + 1),
				Total:    totalAssets,
				AssetID:  asset.ID,
				CopyMode: "skip",
			})
			continue
		}

		srcInfo, statErr := os.Stat(asset.SourcePath)
		if statErr != nil || srcInfo.IsDir() {
			msg := fmt.Sprintf("skipping asset %q: source file missing (%q)", asset.ID, asset.SourcePath)
			if statErr != nil {
				msg = fmt.Sprintf("skipping asset %q: %v", asset.ID, statErr)
			}
			emitEvent(emit, format.ProgressEvent{
				Kind:    "warning",
				Stage:   "assets",
				AssetID: asset.ID,
				Message: msg,
			})
			emitEvent(emit, format.ProgressEvent{
				Kind:     "progress",
				Stage:    "assets",
				Current:  int64(i + 1),
				Total:    totalAssets,
				AssetID:  asset.ID,
				CopyMode: "skip",
			})
			continue
		}

		itemID := assetULIDs[i]
		if asset.ID != "" {
			assetIDMap[asset.ID] = itemID
		}

		baseName, ext := resolveWriteNameAndExt(asset, itemID)
		fileName := buildFileName(baseName, ext)
		itemDir := filepath.Join(w.rootPath, "items", itemID)
		if err := os.MkdirAll(itemDir, 0o755); err != nil {
			return fmt.Errorf("refto: create item dir %q: %w", itemDir, err)
		}

		dstAssetPath := filepath.Join(itemDir, fileName)
		usedMode, err := kit.TransferFile(ctx, asset.SourcePath, dstAssetPath, string(w.opts.CopyMode), nil)
		if err != nil {
			return fmt.Errorf("refto: transfer asset %q: %w", asset.ID, err)
		}

		fileSize := asset.Size
		if fileSize <= 0 {
			fileSize = srcInfo.Size()
		}

		hashVal := strings.TrimSpace(asset.Hash)
		if !strings.HasPrefix(strings.ToLower(hashVal), "sha256:") {
			if computed, hErr := computeFileSHA256(dstAssetPath); hErr == nil {
				hashVal = computed
			}
		}

		// Handle optional thumbnail conversion/transfer to thumbs/<last2>/<ULID>.jpg.
		thumbState := "pending"
		thumbExt := ""
		thumbW, thumbH := 0, 0
		if asset.MediaMeta != nil {
			if asset.MediaMeta.NoThumbnail {
				thumbState = "none"
			} else if tp := strings.TrimSpace(asset.MediaMeta.ThumbnailPath); tp != "" {
				if st, sErr := os.Stat(tp); sErr == nil && !st.IsDir() {
					tw, th, tErr := writeReftoThumbnail(ctx, tp, w.rootPath, itemID, string(w.opts.CopyMode))
					if tErr != nil {
						if ctx.Err() != nil {
							return tErr
						}
						emitEvent(emit, format.ProgressEvent{
							Kind:    "warning",
							Stage:   "assets",
							AssetID: asset.ID,
							Message: fmt.Sprintf("failed to write thumbnail for %q: %v", asset.ID, tErr),
						})
					} else {
						thumbState = "ready"
						thumbExt = "jpg"
						thumbW, thumbH = tw, th
					}
				}
			}
		}

		addedAt := assetTimes[i].UTC()
		addedMs := addedAt.UnixMilli()
		updatedAt := asset.ModifiedAt.UTC()
		if updatedAt.IsZero() {
			updatedAt = addedAt
		}
		updatedMs := updatedAt.UnixMilli()

		fileCTime := srcInfo.ModTime().UTC()
		if !asset.CreatedAt.IsZero() {
			fileCTime = asset.CreatedAt.UTC()
		}
		fileCTimeMs := fileCTime.UnixMilli()
		fileMTime := srcInfo.ModTime().UTC()
		fileMTimeMs := fileMTime.UnixMilli()

		var deletedAt *time.Time
		var deletedAtMs *int64
		if asset.IsDeleted {
			del := updatedAt
			deletedAt = &del
			deletedAtMs = &updatedMs
		}

		var archivedAtMs int64
		if asset.IsHidden {
			archivedAtMs = updatedMs
		}

		var pinnedAtMs int64
		if asset.MediaMeta != nil && asset.MediaMeta.Extra != nil {
			if pv, ok := asInt64(asset.MediaMeta.Extra["pinnedAt"]); ok && pv > 0 {
				pinnedAtMs = pv
			}
		}

		rating := min(max(asset.Rating, 0), 5)

		// Deduplicate item folder links and preserve manual positions if present.
		var folderPositions map[string]float64
		if asset.MediaMeta != nil && asset.MediaMeta.Extra != nil {
			if rawPos, ok := asset.MediaMeta.Extra["reftoFolderPositions"].(map[string]float64); ok {
				folderPositions = rawPos
			}
		}
		var itemFolders []model.ItemFolder
		var sidecarFolders []string
		folderSeen := make(map[string]struct{}, len(asset.FolderIDs))
		for idx, rawFID := range asset.FolderIDs {
			mappedFID, ok := folderIDMap[rawFID]
			if !ok || mappedFID == "" {
				continue
			}
			if _, dup := folderSeen[mappedFID]; dup {
				continue
			}
			folderSeen[mappedFID] = struct{}{}
			pos := float64(idx + 1)
			if folderPositions != nil {
				if p, has := folderPositions[rawFID]; has && p > 0 {
					pos = p
				}
			}
			itemFolders = append(itemFolders, model.ItemFolder{
				ItemID:   itemID,
				FolderID: mappedFID,
				AddedAt:  addedMs,
				Position: pos,
			})
			sidecarFolders = append(sidecarFolders, mappedFID)
		}

		// Deduplicate item tag links.
		var itemTags []model.ItemTag
		var sidecarTags []model.ItemTagSidecar
		var tagNames []string
		tagSeen := make(map[string]struct{}, len(asset.TagIDs))
		for _, rawTID := range asset.TagIDs {
			mappedTID, ok := tagIDMap[rawTID]
			if !ok || mappedTID == "" {
				continue
			}
			if _, dup := tagSeen[mappedTID]; dup {
				continue
			}
			tagSeen[mappedTID] = struct{}{}
			tInfo := tagInfoByID[mappedTID]
			itemTags = append(itemTags, model.ItemTag{
				ItemID:     itemID,
				TagID:      mappedTID,
				Source:     "user",
				State:      "accepted",
				Confidence: 1,
				CreatedAt:  addedMs,
			})
			sidecarTags = append(sidecarTags, model.ItemTagSidecar{
				ID:         mappedTID,
				Name:       tInfo.Name,
				Source:     "user",
				State:      "accepted",
				Confidence: 1,
				AddedAt:    addedAt,
			})
			tagNames = append(tagNames, tInfo.Name)
		}
		tagsText := strings.Join(tagNames, " ")

		// Build palettes and primary_color.
		primaryColor := ""
		palettes := make([]model.Palette, 0, len(asset.Colors))
		for rank, c := range asset.Colors {
			if rank == 0 {
				primaryColor = model.RGBToHex(c.R, c.G, c.B)
			}
			l, okA, okB := model.SRGBToOKLab(c.R, c.G, c.B)
			ratio := c.Ratio / 100.0
			if ratio < 0 {
				ratio = 0
			} else if ratio > 1 {
				ratio = 1
			}
			palettes = append(palettes, model.Palette{
				ItemID: itemID,
				Rank:   rank,
				RGB:    model.PackRGB(c.R, c.G, c.B),
				L:      l,
				A:      okA,
				B:      okB,
				Ratio:  ratio,
				Accent: 0,
			})
		}

		metaJSON := buildItemMetaJSON(asset, ext, fileSize, fileMTimeMs)
		mimeType := resolveMimeType(ext)
		reftoKind := mapDomainKindToRefto(asset.Kind, ext)

		rid := nextRID
		nextRID++

		dbItem := model.Item{
			RID:          rid,
			ID:           itemID,
			Kind:         reftoKind,
			Ext:          ext,
			Mime:         mimeType,
			Name:         baseName,
			FileName:     fileName,
			Storage:      "managed",
			RelPath:      fileName,
			Size:         fileSize,
			Hash:         hashVal,
			Width:        asset.Width,
			Height:       asset.Height,
			Duration:     asset.DurationSec,
			Meta:         metaJSON,
			Rating:       rating,
			Note:         asset.Annotation,
			SourceURL:    asset.OriginURL,
			FileCTime:    &fileCTimeMs,
			FileMTime:    &fileMTimeMs,
			AddedAt:      addedMs,
			UpdatedAt:    updatedMs,
			DeletedAt:    deletedAtMs,
			PrimaryColor: primaryColor,
			ThumbState:   thumbState,
			ThumbExt:     thumbExt,
			ThumbW:       thumbW,
			ThumbH:       thumbH,
			AIState:      "pending",
			TagsText:     tagsText,
			PinnedAt:     pinnedAtMs,
			ArchivedAt:   archivedAtMs,
		}

		if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&dbItem).Error; err != nil {
				return err
			}
			if err := tx.Exec(
				`INSERT INTO items_fts(rowid, name, tags, note, source, body) VALUES (?, ?, ?, ?, ?, '')`,
				rid<<4, baseName, tagsText, asset.Annotation, asset.OriginURL,
			).Error; err != nil {
				return err
			}
			for _, p := range palettes {
				if err := tx.Create(&p).Error; err != nil {
					return err
				}
			}
			for _, rel := range itemFolders {
				if err := tx.Create(&rel).Error; err != nil {
					return err
				}
			}
			for _, rel := range itemTags {
				if err := tx.Create(&rel).Error; err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return fmt.Errorf("refto: insert item %q into act.db: %w", asset.ID, err)
		}

		sidecar := model.ItemSidecar{
			Version: model.ItemSidecarVersion,
			Item: model.ItemSidecarBody{
				ID:        itemID,
				Kind:      reftoKind,
				Ext:       ext,
				Mime:      mimeType,
				Name:      baseName,
				FileName:  fileName,
				Storage:   "managed",
				RelPath:   fileName,
				Size:      fileSize,
				Hash:      hashVal,
				Width:     asset.Width,
				Height:    asset.Height,
				Duration:  asset.DurationSec,
				Rating:    rating,
				Note:      asset.Annotation,
				SourceURL: asset.OriginURL,
				FileCTime: &fileCTime,
				FileMTime: &fileMTime,
				AddedAt:   addedAt,
				UpdatedAt: updatedAt,
				DeletedAt: deletedAt,
			},
			Folders: sidecarFolders,
			Tags:    sidecarTags,
		}
		if err := kit.WriteJSON(filepath.Join(itemDir, "item.json"), sidecar); err != nil {
			return fmt.Errorf("refto: write item.json for %q: %w", asset.ID, err)
		}

		emitEvent(emit, format.ProgressEvent{
			Kind:     "progress",
			Stage:    "assets",
			Current:  int64(i + 1),
			Total:    totalAssets,
			Bytes:    fileSize,
			AssetID:  asset.ID,
			CopyMode: usedMode,
		})
	}

	emitEvent(emit, format.ProgressEvent{
		Kind:    "stage",
		Stage:   "metadata",
		Message: "finalizing Refto library metadata",
	})

	// Apply folder cover_item_id references now that all items exist.
	// Use UpdateColumn so GORM never overwrites millisecond UpdatedAt timestamps.
	for _, ref := range folderCoverRefs {
		if targetItemID, ok := assetIDMap[ref.domainAssetID]; ok && targetItemID != "" {
			_ = db.WithContext(ctx).
				Model(&model.Folder{}).
				Where("id = ?", ref.reftoFolderID).
				UpdateColumn("cover_item_id", targetItemID).Error
		}
	}

	if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		return fmt.Errorf("refto: checkpoint act.db: %w", err)
	}

	tagsSidecar := model.TagsSidecar{
		Version: model.TagsSidecarVersion,
		Groups:  groupSidecars,
		Tags:    tagSidecars,
	}
	if err := kit.WriteJSON(filepath.Join(w.rootPath, "tags.json"), tagsSidecar); err != nil {
		return fmt.Errorf("refto: write tags.json: %w", err)
	}

	libName := strings.TrimSpace(w.opts.LibraryName)
	if libName == "" {
		libName = strings.TrimSpace(lib.Name)
	}
	if libName == "" {
		libName = libraryNameFromPath(w.rootPath)
	}
	libCreated := lib.CreatedAt
	if libCreated.IsZero() {
		libCreated = now
	}
	manifest := model.LibraryManifest{
		Schema:     model.ManifestSchema,
		ID:         kit.NewULID(libCreated),
		Name:       libName,
		CreatedAt:  libCreated.UTC(),
		AppVersion: model.DefaultAppVersion,
		Storage:    "managed",
	}
	if err := kit.WriteJSON(filepath.Join(w.rootPath, "act-library.json"), manifest); err != nil {
		return fmt.Errorf("refto: write act-library.json: %w", err)
	}

	emitEvent(emit, format.ProgressEvent{
		Kind:    "stage",
		Stage:   "done",
		Message: "Refto library written",
	})
	return nil
}

func (w *writer) prepareStorage() error {
	if w.opts.Overwrite {
		for _, sub := range []string{"act.db", "act.db-wal", "act.db-shm", "act-library.json", "tags.json", "items", "thumbs"} {
			_ = os.RemoveAll(filepath.Join(w.rootPath, sub))
		}
	}
	for _, dir := range []string{w.rootPath, filepath.Join(w.rootPath, "items"), filepath.Join(w.rootPath, "thumbs")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("refto: create directory %q: %w", dir, err)
		}
	}
	return nil
}

func buildReftoTagGroups(groups []*domain.TagGroup, now time.Time) ([]model.TagGroup, []model.TagGroupSidecar, map[string]string) {
	nowMs := now.UnixMilli()
	idMap := make(map[string]string, len(groups))
	seenNames := make(map[string]string, len(groups))
	dbRows := make([]model.TagGroup, 0, len(groups))
	sidecars := make([]model.TagGroupSidecar, 0, len(groups))

	ordered := make([]*domain.TagGroup, 0, len(groups))
	hasRanking := false
	for _, g := range groups {
		if g == nil {
			continue
		}
		if g.Ranking != 0 {
			hasRanking = true
		}
		ordered = append(ordered, g)
	}
	if hasRanking {
		slices.SortStableFunc(ordered, func(a, b *domain.TagGroup) int {
			return cmp.Compare(a.Ranking, b.Ranking)
		})
	}

	for _, g := range ordered {
		name := strings.TrimSpace(g.Name)
		if name == "" {
			name = strings.TrimSpace(g.ID)
		}
		if name == "" {
			continue
		}
		if existingID, dup := seenNames[name]; dup {
			if g.ID != "" {
				idMap[g.ID] = existingID
			}
			continue
		}
		newID := kit.NewULID(now)
		seenNames[name] = newID
		if g.ID != "" {
			idMap[g.ID] = newID
		}
		pos := len(dbRows)
		color := sanitizeReftoColor(g.Color)
		dbRows = append(dbRows, model.TagGroup{
			ID:        newID,
			Name:      name,
			Color:     color,
			Position:  pos,
			CreatedAt: nowMs,
		})
		sidecars = append(sidecars, model.TagGroupSidecar{
			ID:        newID,
			Name:      name,
			Color:     color,
			Position:  pos,
			CreatedAt: now,
		})
	}
	return dbRows, sidecars, idMap
}

func buildReftoTags(tags []*domain.Tag, groupIDMap map[string]string, now time.Time) (
	dbRows []model.Tag,
	sidecars []model.TagSidecar,
	idMap map[string]string,
	byID map[string]model.Tag,
) {
	idMap = make(map[string]string, len(tags))
	byID = make(map[string]model.Tag, len(tags))
	seenLower := make(map[string]string, len(tags))
	dbRows = make([]model.Tag, 0, len(tags))
	sidecars = make([]model.TagSidecar, 0, len(tags))

	for _, t := range tags {
		if t == nil {
			continue
		}
		name := strings.TrimSpace(t.Name)
		if name == "" {
			name = strings.TrimSpace(t.ID)
		}
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if existingID, dup := seenLower[lower]; dup {
			if t.ID != "" {
				idMap[t.ID] = existingID
			}
			continue
		}

		created := t.CreatedAt.UTC()
		if created.IsZero() {
			created = now
		}
		createdMs := created.UnixMilli()
		newID := kit.NewULID(created)
		seenLower[lower] = newID
		if t.ID != "" {
			idMap[t.ID] = newID
		}

		pinyin := kit.PinyinSearchIndex(name)
		var groupPtr *string
		groupStr := ""
		if t.GroupID != "" {
			if mappedGID, ok := groupIDMap[t.GroupID]; ok && mappedGID != "" {
				groupStr = mappedGID
				groupPtr = &groupStr
			}
		}
		starred := 0
		if t.Starred {
			starred = 1
		}

		color := sanitizeReftoColor(t.Color)
		row := model.Tag{
			ID:        newID,
			Name:      name,
			Pinyin:    pinyin,
			Color:     color,
			Starred:   starred,
			GroupID:   groupPtr,
			CreatedAt: createdMs,
			UpdatedAt: createdMs,
		}
		dbRows = append(dbRows, row)
		byID[newID] = row
		sidecars = append(sidecars, model.TagSidecar{
			ID:        newID,
			Name:      name,
			Pinyin:    pinyin,
			Color:     color,
			Starred:   t.Starred,
			GroupID:   groupStr,
			CreatedAt: created,
			UpdatedAt: created,
		})
	}
	return dbRows, sidecars, idMap, byID
}

type folderCoverRef struct {
	reftoFolderID string
	domainAssetID string
}

func buildReftoFolders(folders []*domain.Folder, nowMs int64) ([]model.Folder, map[string]string, []folderCoverRef) {
	byID := make(map[string]*domain.Folder)
	parentOf := make(map[string]string)
	var orderedIDs []string

	var collect func(list []*domain.Folder, implicitParent string)
	collect = func(list []*domain.Folder, implicitParent string) {
		for idx, f := range list {
			if f == nil {
				continue
			}
			key := f.ID
			if key == "" {
				key = fmt.Sprintf("__anon_%d_%p", idx, f)
			}
			if _, exists := byID[key]; !exists {
				byID[key] = f
				orderedIDs = append(orderedIDs, key)
			}
			if implicitParent != "" && parentOf[key] == "" {
				parentOf[key] = implicitParent
			}
			if f.ParentID != "" && f.ParentID != key {
				parentOf[key] = f.ParentID
			}
			if len(f.Children) > 0 {
				collect(f.Children, key)
			}
		}
	}
	collect(folders, "")

	idMap := make(map[string]string, len(byID))
	for _, oldID := range orderedIDs {
		idMap[oldID] = kit.NewULID(time.UnixMilli(nowMs))
	}

	var rootIDs []string
	childrenOf := make(map[string][]string)
	for _, oldID := range orderedIDs {
		pid := parentOf[oldID]
		if pid == "" || pid == oldID || byID[pid] == nil {
			rootIDs = append(rootIDs, oldID)
			continue
		}
		childrenOf[pid] = append(childrenOf[pid], oldID)
	}

	sortBySeq := func(ids []string) {
		slices.SortStableFunc(ids, func(a, b string) int {
			return cmp.Compare(byID[a].Seq, byID[b].Seq)
		})
	}
	sortBySeq(rootIDs)
	for pid, cids := range childrenOf {
		sortBySeq(cids)
		childrenOf[pid] = cids
	}

	var rows []model.Folder
	var coverRefs []folderCoverRef
	visited := make(map[string]bool, len(byID))

	var emitTree func(ids []string, parentNewID string)
	emitTree = func(ids []string, parentNewID string) {
		for idx, oldID := range ids {
			if visited[oldID] {
				continue
			}
			visited[oldID] = true
			src := byID[oldID]
			newID := idMap[oldID]

			var parentPtr *string
			if parentNewID != "" {
				p := parentNewID
				parentPtr = &p
			}

			createdMs := nowMs
			if !src.CreatedAt.IsZero() {
				createdMs = src.CreatedAt.UnixMilli()
			}
			updatedMs := createdMs
			if !src.ModifiedAt.IsZero() {
				updatedMs = src.ModifiedAt.UnixMilli()
			}

			autoTagsJSON := "[]"
			if len(src.TagNames) > 0 {
				if raw, err := json.Marshal(src.TagNames); err == nil {
					autoTagsJSON = string(raw)
				}
			}

			colorVal := sanitizeReftoColor(src.Color)
			if colorVal == "" {
				colorVal = sanitizeReftoColor(src.IconColor)
			}

			rows = append(rows, model.Folder{
				ID:          newID,
				ParentID:    parentPtr,
				Position:    idx,
				Name:        src.Name,
				Pinyin:      kit.PinyinSearchIndex(src.Name),
				Description: src.Description,
				Icon:        sanitizeReftoFolderIcon(src.Icon),
				Color:       colorVal,
				AutoTags:    autoTagsJSON,
				CreatedAt:   createdMs,
				UpdatedAt:   updatedMs,
			})
			if src.CoverAssetID != "" {
				coverRefs = append(coverRefs, folderCoverRef{
					reftoFolderID: newID,
					domainAssetID: src.CoverAssetID,
				})
			}

			emitTree(childrenOf[oldID], newID)
		}
	}

	emitTree(rootIDs, "")
	if len(visited) < len(orderedIDs) {
		var remaining []string
		for _, oldID := range orderedIDs {
			if !visited[oldID] {
				remaining = append(remaining, oldID)
			}
		}
		emitTree(remaining, "")
	}
	return rows, idMap, coverRefs
}

var reftoNamedColorHex = map[string]string{
	"aqua":    "#5fcbed",
	"cyan":    "#5fcbed",
	"teal":    "#5fcbed",
	"blue":    "#3f81ff",
	"purple":  "#8473ff",
	"violet":  "#8473ff",
	"indigo":  "#8473ff",
	"orange":  "#ff9213",
	"gray":    "#8e8e93",
	"grey":    "#8e8e93",
	"yellow":  "#ffcc0f",
	"amber":   "#ffcc0f",
	"gold":    "#ffcc0f",
	"pink":    "#ff5e8e",
	"magenta": "#ff5e8e",
	"rose":    "#ff5e8e",
	"green":   "#7acf25",
	"lime":    "#7acf25",
	"emerald": "#7acf25",
	"red":     "#f25555",
	"black":   "#000000",
	"white":   "#ffffff",
	"brown":   "#a2845e",
}

// sanitizeReftoColor normalizes named or hex colors to lowercase "#rrggbb" or "".
// Refto's mygo/ui frontend passes non-empty folder and tag colors straight to ui.Hex,
// which panics on named strings such as Pixcall's "blue".
func sanitizeReftoColor(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	if mapped, ok := reftoNamedColorHex[s]; ok {
		return mapped
	}
	if !strings.HasPrefix(s, "#") {
		return ""
	}
	hexPart := s[1:]
	for i := range len(hexPart) {
		c := hexPart[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return ""
		}
	}
	switch len(hexPart) {
	case 3:
		return fmt.Sprintf("#%c%c%c%c%c%c",
			hexPart[0], hexPart[0],
			hexPart[1], hexPart[1],
			hexPart[2], hexPart[2])
	case 6, 8:
		return "#" + hexPart[:6]
	default:
		return ""
	}
}

// sanitizeReftoFolderIcon keeps only Refto-style ASCII icon names (e.g. "global")
// and strips foreign formats such as Pixcall's ":script:", emoji hex tokens, or Billfish numeric IDs.
func sanitizeReftoFolderIcon(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > 64 {
		return ""
	}
	if strings.HasPrefix(s, ":") || strings.HasSuffix(s, ":") {
		return ""
	}
	first := s[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return ""
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		valid := (c >= 'a' && c <= 'z') ||
			(c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') ||
			c == '.' || c == '_' || c == '-'
		if !valid {
			return ""
		}
	}
	return s
}

func resolveWriteNameAndExt(asset *domain.Asset, fallbackName string) (baseName, ext string) {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Ext)), ".")
	baseName = asset.Name
	if baseName == "" && asset.FileName != "" {
		parsedBase, parsedExt := kit.SplitFilename(asset.FileName)
		baseName = parsedBase
		if ext == "" {
			ext = parsedExt
		}
	}
	if baseName == "" && asset.SourcePath != "" {
		parsedBase, parsedExt := kit.SplitFilename(filepath.Base(asset.SourcePath))
		baseName = parsedBase
		if ext == "" {
			ext = parsedExt
		}
	}
	baseName = sanitizeFilenamePart(norm.NFC.String(baseName))
	if baseName == "" {
		baseName = fallbackName
	}
	return baseName, ext
}

func sanitizeFilenamePart(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == ".." || name == "/" || name == "" {
		return ""
	}
	return name
}

func buildFileName(name, ext string) string {
	if ext == "" {
		return name
	}
	return name + "." + ext
}

func computeFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func writeReftoThumbnail(ctx context.Context, src, rootPath, itemID, copyMode string) (width, height int, err error) {
	if len(itemID) < 2 {
		return 0, 0, errors.New("invalid item ULID for thumbnail bucketing")
	}
	bucket := strings.ToLower(itemID[len(itemID)-2:])
	dstDir := filepath.Join(rootPath, "thumbs", bucket)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return 0, 0, err
	}
	dst := filepath.Join(dstDir, itemID+".jpg")

	ext := strings.ToLower(filepath.Ext(src))
	if ext == ".jpg" || ext == ".jpeg" {
		if _, err := kit.TransferFile(ctx, src, dst, copyMode, nil); err != nil {
			return 0, 0, err
		}
		if f, oErr := os.Open(dst); oErr == nil {
			defer f.Close()
			if cfg, _, dErr := image.DecodeConfig(f); dErr == nil {
				return cfg.Width, cfg.Height, nil
			}
		}
		return 0, 0, nil
	}

	f, err := os.Open(src)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return 0, 0, fmt.Errorf("decode thumbnail %q: %w", src, err)
	}
	bounds := img.Bounds()

	tmpFile, err := os.CreateTemp(dstDir, ".debris-thumb-*.jpg")
	if err != nil {
		return 0, 0, err
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := jpeg.Encode(tmpFile, img, &jpeg.Options{Quality: 85}); err != nil {
		_ = tmpFile.Close()
		return 0, 0, err
	}
	if err := tmpFile.Close(); err != nil {
		return 0, 0, err
	}
	if err := os.Rename(tmpPath, dst); err != nil {
		return 0, 0, err
	}
	cleanup = false
	return bounds.Dx(), bounds.Dy(), nil
}

func buildItemMetaJSON(asset *domain.Asset, ext string, size, mtimeMs int64) string {
	if asset.MediaMeta != nil && asset.MediaMeta.Extra != nil {
		if rawMeta, ok := asset.MediaMeta.Extra["reftoMeta"].(map[string]any); ok && len(rawMeta) > 0 {
			if raw, err := json.Marshal(rawMeta); err == nil {
				return string(raw)
			}
		}
	}

	metaObj := map[string]any{
		"v": 1,
		"source": map[string]any{
			"size":  size,
			"mtime": mtimeMs,
		},
	}
	container := ext
	if asset.MediaMeta != nil {
		if asset.MediaMeta.Format != "" {
			container = asset.MediaMeta.Format
		}
		if len(asset.MediaMeta.EXIF) > 0 {
			metaObj["exif"] = asset.MediaMeta.EXIF
		}
	}
	if container != "" {
		metaObj["container"] = container
	}
	raw, err := json.Marshal(metaObj)
	if err != nil {
		return ""
	}
	return string(raw)
}

func resolveMimeType(ext string) string {
	if ext == "" {
		return ""
	}
	switch ext {
	case "jxl":
		return "image/jxl"
	case "avif":
		return "image/avif"
	case "heic":
		return "image/heic"
	case "heif":
		return "image/heif"
	case "webp":
		return "image/webp"
	case "mov":
		return "video/quicktime"
	}
	if m := mime.TypeByExtension("." + ext); m != "" {
		if idx := strings.IndexByte(m, ';'); idx >= 0 {
			return strings.TrimSpace(m[:idx])
		}
		return m
	}
	return ""
}

func mapDomainKindToRefto(kind domain.AssetKind, ext string) string {
	switch kind {
	case domain.KindImage:
		return "image"
	case domain.KindVideo:
		return "video"
	case domain.KindAudio:
		return "audio"
	case domain.KindFont:
		return "font"
	}
	switch strings.ToLower(strings.TrimPrefix(ext, ".")) {
	case "pdf":
		return "pdf"
	case "txt", "md", "markdown", "json", "yaml", "yml":
		return "text"
	case "doc", "docx", "xls", "xlsx", "ppt", "pptx":
		return "office"
	default:
		return "other"
	}
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func emitEvent(emit format.ProgressCallback, ev format.ProgressEvent) {
	if emit != nil {
		emit(ev)
	}
}
