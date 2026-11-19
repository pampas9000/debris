package pixcall

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/pixcall/model"
	"debris/internal/kit"
)

const insertBatchSize = 200

// Writer implements format.Writer for Pixcall libraries (schema_version 22).
type Writer struct {
	rootPath string
	opts     format.WriteOptions
	db       *gorm.DB
	closer   io.Closer

	closeOnce sync.Once
	closeErr  error
}

// OpenWriter prepares a Pixcall writer targeting path.
func (a *Adapter) OpenWriter(ctx context.Context, path string, opts format.WriteOptions) (format.Writer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("pixcall: empty target path")
	}

	mode, err := format.ParseCopyMode(string(opts.CopyMode))
	if err != nil {
		return nil, fmt.Errorf("pixcall: %w", err)
	}
	opts.CopyMode = mode

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("pixcall: resolve target path %q: %w", path, err)
	}

	return &Writer{
		rootPath: absPath,
		opts:     opts,
	}, nil
}

// Close closes the writer's SQLite connection if it was opened.
func (w *Writer) Close() error {
	w.closeOnce.Do(func() {
		if w.closer != nil {
			w.closeErr = w.closer.Close()
		}
	})
	return w.closeErr
}

// Write materializes lib as a Pixcall schema_version 22 library on disk.
func (w *Writer) Write(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lib == nil {
		return errors.New("pixcall: nil library")
	}

	send := func(ev format.ProgressEvent) {
		if emit != nil {
			emit(ev)
		}
	}

	send(format.ProgressEvent{
		Kind:    "stage",
		Stage:   "init",
		Message: "initializing Pixcall library",
	})

	installUUID, err := newUUIDv4()
	if err != nil {
		return fmt.Errorf("pixcall: generate installation uuid: %w", err)
	}

	if !w.opts.DryRun {
		if err := w.prepareStorage(ctx, installUUID); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	libCreated := lib.CreatedAt
	if libCreated.IsZero() {
		libCreated = now
	}
	libModified := lib.ModifiedAt
	if libModified.IsZero() {
		libModified = libCreated
	}

	tagGroups, groupIDMap := buildPixcallTagGroups(lib.TagGroups)
	tags, tagIDMap := buildPixcallTags(lib.Tags, groupIDMap)

	usedNames := map[int64]map[string]struct{}{
		rootEntryID:  make(map[string]struct{}),
		trashEntryID: make(map[string]struct{}),
	}

	orderedFolders := flattenAndOrderFolders(lib.Folders)
	folderIDMap := make(map[string]int64, len(orderedFolders))
	folderRelPaths := make(map[int64][]string, len(orderedFolders)+2)
	folderIsTrash := make(map[int64]bool, len(orderedFolders)+2)
	folderIsHidden := make(map[int64]bool, len(orderedFolders)+2)
	folderChildren := make(map[int64][]int64)

	folderRelPaths[rootEntryID] = nil
	folderRelPaths[trashEntryID] = nil
	folderIsTrash[trashEntryID] = true

	var entries []model.Entry
	entries = append(entries,
		model.Entry{
			ID:          rootEntryID,
			Revision:    1,
			ParentID:    -1,
			Name:        "Pixcall",
			Kind:        0,
			Size:        0,
			ContentType: "unknown",
			Ranking:     9000000000000000000,
			CreatedAt:   libCreated,
			UpdatedAt:   libModified,
			Status:      64,
		},
		model.Entry{
			ID:          trashEntryID,
			Revision:    1,
			ParentID:    -1,
			Name:        "Trash",
			Kind:        0,
			Size:        0,
			ContentType: "unknown",
			Ranking:     8999999999000000000,
			CreatedAt:   libCreated,
			UpdatedAt:   libModified,
			Status:      64,
		},
	)

	folderModelsMap := make(map[int64]*model.Folders, len(orderedFolders)+2)
	folderModelsMap[rootEntryID] = &model.Folders{EntryID: rootEntryID, Revision: 1}
	folderModelsMap[trashEntryID] = &model.Folders{EntryID: trashEntryID, Revision: 1}

	coverDomainRefByFolder := make(map[int64]string)

	for idx, df := range orderedFolders {
		if err := ctx.Err(); err != nil {
			return err
		}

		parentID := rootEntryID
		if df.IsRecycle {
			parentID = trashEntryID
		}
		if df.ParentID != "" {
			if mappedParent, ok := folderIDMap[df.ParentID]; ok {
				parentID = mappedParent
			}
		}

		inTrash := folderIsTrash[parentID] || df.IsRecycle
		if df.IsRecycle && !folderIsTrash[parentID] {
			parentID = trashEntryID
			inTrash = true
		}

		if usedNames[parentID] == nil {
			usedNames[parentID] = make(map[string]struct{})
		}
		uniqueName := disambiguateName(usedNames[parentID], sanitizeEntryName(df.Name, "Folder"), false)

		newID := kit.NewSnowflakeID()
		folderIDMap[df.ID] = newID
		folderIsTrash[newID] = inTrash
		folderIsHidden[newID] = df.IsHidden || folderIsHidden[parentID]
		folderChildren[parentID] = append(folderChildren[parentID], newID)
		usedNames[newID] = make(map[string]struct{})

		parentRel := folderRelPaths[parentID]
		rel := make([]string, len(parentRel)+1)
		copy(rel, parentRel)
		rel[len(parentRel)] = uniqueName
		folderRelPaths[newID] = rel

		if !w.opts.DryRun {
			var dirParts []string
			if inTrash {
				dirParts = append([]string{w.rootPath, ".pixcall", "trash"}, rel...)
			} else {
				dirParts = append([]string{w.rootPath}, rel...)
			}
			if err := os.MkdirAll(filepath.Join(dirParts...), 0o755); err != nil {
				return fmt.Errorf("pixcall: create folder %q: %w", uniqueName, err)
			}
		}

		created := df.CreatedAt
		if created.IsZero() {
			created = now
		}
		modified := df.ModifiedAt
		if modified.IsZero() {
			modified = created
		}

		ranking := df.Seq
		if ranking == 0 {
			ranking = 9000000000000000000 - int64(idx)*1000000000
		}

		var namePinyin *string
		if p := kit.PinyinCamelCase(uniqueName); p != "" && p != uniqueName {
			namePinyin = &p
		}

		isHidden := 0
		if df.IsHidden {
			isHidden = 1
		}
		isDeleted := 0
		if inTrash {
			isDeleted = 1
		}

		entries = append(entries, model.Entry{
			ID:          newID,
			Revision:    1,
			ParentID:    parentID,
			Name:        uniqueName,
			Kind:        0,
			Description: strPtr(df.Description),
			Size:        0,
			ContentType: "unknown",
			Ranking:     ranking,
			CreatedAt:   created,
			UpdatedAt:   modified,
			NamePinyin:  namePinyin,
			Status:      64,
			IsHidden:    isHidden,
			IsDeleted:   isDeleted,
		})

		fm := &model.Folders{
			EntryID:   newID,
			Revision:  1,
			Ranking:   ranking,
			Icon:      strPtr(df.Icon),
			IconColor: strPtr(df.IconColor),
		}
		if df.Extra != nil {
			if v, ok := df.Extra["layout"].(string); ok {
				fm.Layout = strPtr(v)
			}
			if v, ok := df.Extra["sortBy"].(string); ok {
				fm.SortBy = strPtr(v)
			}
			if v, ok := df.Extra["sortOrder"].(string); ok {
				fm.SortOrder = strPtr(v)
			}
		}
		folderModelsMap[newID] = fm
		if df.CoverAssetID != "" {
			coverDomainRefByFolder[newID] = df.CoverAssetID
		}
	}

	send(format.ProgressEvent{
		Kind:    "stage",
		Stage:   "transfer",
		Total:   int64(len(lib.Assets)),
		Message: "transferring assets",
	})

	type folderDirectStat struct {
		fileCount    int
		fileSize     int64
		pubFileCount int
		pubFileSize  int64
	}
	directStats := make(map[int64]*folderDirectStat)
	addDirectStat := func(folderID int64, size int64, public bool) {
		st := directStats[folderID]
		if st == nil {
			st = &folderDirectStat{}
			directStats[folderID] = st
		}
		st.fileCount++
		st.fileSize += size
		if public {
			st.pubFileCount++
			st.pubFileSize += size
		}
	}

	assetIDMap := make(map[string]int64, len(lib.Assets))
	var medias []model.Media
	var exifs []model.Exif
	totalAssets := int64(len(lib.Assets))

	for idx, a := range lib.Assets {
		if err := ctx.Err(); err != nil {
			return err
		}

		parentID := rootEntryID
		if len(a.FolderIDs) > 0 {
			for _, fid := range a.FolderIDs {
				if mapped, ok := folderIDMap[fid]; ok {
					if !a.IsDeleted || folderIsTrash[mapped] {
						parentID = mapped
						break
					}
				}
			}
		}
		if a.IsDeleted && !folderIsTrash[parentID] {
			parentID = trashEntryID
		}

		inTrash := a.IsDeleted || folderIsTrash[parentID]
		rawName := sanitizeEntryName(a.EffectiveFileName(), sanitizeEntryName(a.ID, "asset"))
		if usedNames[parentID] == nil {
			usedNames[parentID] = make(map[string]struct{})
		}
		uniqueName := disambiguateName(usedNames[parentID], rawName, true)

		rel := folderRelPaths[parentID]
		var dstParts []string
		if inTrash {
			dstParts = append([]string{w.rootPath, ".pixcall", "trash"}, rel...)
		} else {
			dstParts = append([]string{w.rootPath}, rel...)
		}
		dstParts = append(dstParts, uniqueName)
		dstPath := filepath.Join(dstParts...)

		fileSize := a.Size
		if a.SourcePath == "" {
			send(format.ProgressEvent{
				Kind:    "warning",
				Stage:   "transfer",
				AssetID: a.ID,
				Message: fmt.Sprintf("missing source path for asset %q", uniqueName),
			})
			send(format.ProgressEvent{
				Kind:     "progress",
				Stage:    "transfer",
				Current:  int64(idx + 1),
				Total:    totalAssets,
				AssetID:  a.ID,
				CopyMode: "skip",
			})
			continue
		}

		srcInfo, statErr := os.Stat(a.SourcePath)
		if statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				send(format.ProgressEvent{
					Kind:    "warning",
					Stage:   "transfer",
					AssetID: a.ID,
					Message: fmt.Sprintf("source file not found %q: %v", a.SourcePath, statErr),
				})
				send(format.ProgressEvent{
					Kind:     "progress",
					Stage:    "transfer",
					Current:  int64(idx + 1),
					Total:    totalAssets,
					AssetID:  a.ID,
					CopyMode: "skip",
				})
				continue
			}
			return fmt.Errorf("pixcall: stat source %q: %w", a.SourcePath, statErr)
		}
		if fileSize == 0 {
			fileSize = srcInfo.Size()
		}

		if w.opts.DryRun {
			send(format.ProgressEvent{
				Kind:     "progress",
				Stage:    "transfer",
				Current:  int64(idx + 1),
				Total:    totalAssets,
				Bytes:    fileSize,
				AssetID:  a.ID,
				CopyMode: "skip",
			})
		} else {
			if w.opts.Overwrite && w.opts.CopyMode == format.CopyModeHardlink {
				if dstInfo, err := os.Stat(dstPath); err == nil && !os.SameFile(srcInfo, dstInfo) {
					_ = os.Remove(dstPath)
				}
			}
			usedMode, err := kit.TransferFile(ctx, a.SourcePath, dstPath, string(w.opts.CopyMode), nil)
			if err != nil {
				return fmt.Errorf("pixcall: transfer %q -> %q: %w", a.SourcePath, dstPath, err)
			}
			send(format.ProgressEvent{
				Kind:     "progress",
				Stage:    "transfer",
				Current:  int64(idx + 1),
				Total:    totalAssets,
				Bytes:    fileSize,
				AssetID:  a.ID,
				CopyMode: usedMode,
			})
		}

		entryID := kit.NewSnowflakeID()
		if a.ID != "" {
			assetIDMap[a.ID] = entryID
		}

		created := a.CreatedAt
		if created.IsZero() {
			created = now
		}
		modified := a.ModifiedAt
		if modified.IsZero() {
			modified = created
		}
		mtimeMs := modified.UnixMilli()

		mappedTags := mapAssetTags(a.TagIDs, tagIDMap)
		var tagsStr *string
		if len(mappedTags) > 0 {
			joined := strings.Join(mappedTags, "|")
			tagsStr = &joined
		}

		_, ext := kit.SplitFilename(uniqueName)
		if ext == "" {
			ext = strings.TrimPrefix(strings.ToLower(a.Ext), ".")
		}
		contentType := mimeTypeFromExt(ext, a.Kind)

		metaStr := buildEntryMetadataJSON(a, created, modified)
		var namePinyin *string
		if p := kit.PinyinCamelCase(uniqueName); p != "" && p != uniqueName {
			namePinyin = &p
		}

		isHidden := 0
		if a.IsHidden {
			isHidden = 1
		}
		isDeleted := 0
		if inTrash {
			isDeleted = 1
		}

		entries = append(entries, model.Entry{
			ID:          entryID,
			Revision:    1,
			ParentID:    parentID,
			Name:        uniqueName,
			Kind:        1,
			Description: strPtr(a.Annotation),
			Link:        strPtr(a.OriginURL),
			Tags:        tagsStr,
			Rating:      intPtr(a.Rating),
			Size:        fileSize,
			ContentType: contentType,
			ContentHash: strPtr(a.Hash),
			Metadata:    strPtr(metaStr),
			Ranking:     9000000000000000000 - int64(idx)*1000000000,
			CreatedAt:   created,
			UpdatedAt:   modified,
			NamePinyin:  namePinyin,
			Status:      2069,
			Mtime:       &mtimeMs,
			IsHidden:    isHidden,
			IsDeleted:   isDeleted,
			IsPrivate:   0,
		})

		if medJSON := buildMediaMetadataJSON(a); medJSON != "" {
			medias = append(medias, model.Media{
				EntryID:  entryID,
				Metadata: medJSON,
			})
		}
		if a.MediaMeta != nil && len(a.MediaMeta.EXIF) > 0 {
			if rawExif, err := json.Marshal(a.MediaMeta.EXIF); err == nil {
				exifs = append(exifs, model.Exif{
					EntryID:  entryID,
					Metadata: string(rawExif),
				})
			}
		}

		isPublic := !a.IsHidden && !inTrash && !folderIsHidden[parentID]
		addDirectStat(parentID, fileSize, isPublic)
	}

	// Resolve folder cover IDs from mapped asset IDs.
	for fid, domainAssetID := range coverDomainRefByFolder {
		if mappedAssetID, ok := assetIDMap[domainAssetID]; ok {
			if fm := folderModelsMap[fid]; fm != nil {
				idCopy := mappedAssetID
				fm.CoverID = &idCopy
			}
		}
	}

	// Accumulate recursive folder stats bottom-up (orderedFolders is parent-before-child).
	for i := len(orderedFolders) - 1; i >= 0; i-- {
		fid := folderIDMap[orderedFolders[i].ID]
		fm := folderModelsMap[fid]
		if st := directStats[fid]; st != nil {
			fm.FileCount += st.fileCount
			fm.FileSize += st.fileSize
			fm.PubFileCount += st.pubFileCount
			fm.PubFileSize += st.pubFileSize
		}
		for _, childID := range folderChildren[fid] {
			if childFM := folderModelsMap[childID]; childFM != nil {
				fm.FolderCount += 1 + childFM.FolderCount
				fm.FileCount += childFM.FileCount
				fm.FileSize += childFM.FileSize
				if !folderIsHidden[fid] && !folderIsHidden[childID] && !folderIsTrash[childID] {
					fm.PubFolderCount += 1 + childFM.PubFolderCount
					fm.PubFileCount += childFM.PubFileCount
					fm.PubFileSize += childFM.PubFileSize
				}
			}
		}
	}

	for _, rootID := range []int64{rootEntryID, trashEntryID} {
		fm := folderModelsMap[rootID]
		if st := directStats[rootID]; st != nil {
			fm.FileCount += st.fileCount
			fm.FileSize += st.fileSize
			fm.PubFileCount += st.pubFileCount
			fm.PubFileSize += st.pubFileSize
		}
		for _, childID := range folderChildren[rootID] {
			if childFM := folderModelsMap[childID]; childFM != nil {
				fm.FolderCount += 1 + childFM.FolderCount
				fm.FileCount += childFM.FileCount
				fm.FileSize += childFM.FileSize
				if rootID == rootEntryID && !folderIsHidden[childID] && !folderIsTrash[childID] {
					fm.PubFolderCount += 1 + childFM.PubFolderCount
					fm.PubFileCount += childFM.PubFileCount
					fm.PubFileSize += childFM.PubFileSize
				}
			}
		}
	}

	folders := make([]model.Folders, 0, len(folderModelsMap))
	folders = append(folders, *folderModelsMap[rootEntryID], *folderModelsMap[trashEntryID])
	for _, df := range orderedFolders {
		fid := folderIDMap[df.ID]
		folders = append(folders, *folderModelsMap[fid])
	}

	if w.opts.DryRun {
		send(format.ProgressEvent{
			Kind:    "stage",
			Stage:   "done",
			Message: "dry-run complete",
		})
		return nil
	}

	send(format.ProgressEvent{
		Kind:    "stage",
		Stage:   "database",
		Message: "writing main.db",
	})

	kvs := buildSeedKvs(installUUID)

	if err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(tagGroups) > 0 {
			if err := tx.CreateInBatches(&tagGroups, insertBatchSize).Error; err != nil {
				return fmt.Errorf("pixcall: insert tag_groups: %w", err)
			}
		}
		if len(tags) > 0 {
			if err := tx.CreateInBatches(&tags, insertBatchSize).Error; err != nil {
				return fmt.Errorf("pixcall: insert tags: %w", err)
			}
		}
		if len(entries) > 0 {
			if err := tx.CreateInBatches(&entries, insertBatchSize).Error; err != nil {
				return fmt.Errorf("pixcall: insert entries: %w", err)
			}
		}
		if len(folders) > 0 {
			if err := tx.CreateInBatches(&folders, insertBatchSize).Error; err != nil {
				return fmt.Errorf("pixcall: insert folders: %w", err)
			}
		}
		if len(medias) > 0 {
			if err := tx.CreateInBatches(&medias, insertBatchSize).Error; err != nil {
				return fmt.Errorf("pixcall: insert media: %w", err)
			}
		}
		if len(exifs) > 0 {
			if err := tx.CreateInBatches(&exifs, insertBatchSize).Error; err != nil {
				return fmt.Errorf("pixcall: insert exif: %w", err)
			}
		}
		if len(kvs) > 0 {
			if err := tx.CreateInBatches(&kvs, insertBatchSize).Error; err != nil {
				return fmt.Errorf("pixcall: insert kvs: %w", err)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	send(format.ProgressEvent{
		Kind:    "stage",
		Stage:   "done",
		Message: "Pixcall library written",
	})

	return nil
}

func (w *Writer) prepareStorage(ctx context.Context, installUUID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	dbPath := mainDBPath(w.rootPath)
	if _, err := os.Stat(dbPath); err == nil {
		if !w.opts.Overwrite {
			return fmt.Errorf("pixcall: target library already exists at %q", w.rootPath)
		}
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(dbPath + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("pixcall: remove existing database %q: %w", dbPath+suffix, err)
			}
		}
	}

	pixcallDir := filepath.Join(w.rootPath, ".pixcall")
	for _, dir := range []string{
		pixcallDir,
		filepath.Join(pixcallDir, "database"),
		filepath.Join(pixcallDir, "trash"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("pixcall: create directory %q: %w", dir, err)
		}
	}

	cfg := map[string]any{
		"preferences": map[string]any{
			"cache_path":          nil,
			"default_layout_mode": nil,
			"default_sort_by":     nil,
			"default_sort_order":  nil,
			"max_cache_size":      int64(5368709120),
		},
	}
	if err := kit.WriteJSON(filepath.Join(pixcallDir, "config.json"), cfg); err != nil {
		return fmt.Errorf("pixcall: write config.json: %w", err)
	}

	existingIDs, _ := filepath.Glob(filepath.Join(pixcallDir, "*.id"))
	if len(existingIDs) == 0 {
		idFilePath := filepath.Join(pixcallDir, installUUID+".id")
		f, err := os.OpenFile(idFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("pixcall: create id marker %q: %w", idFilePath, err)
		}
		if err := f.Close(); err != nil {
			return fmt.Errorf("pixcall: close id marker %q: %w", idFilePath, err)
		}
	}

	db, closer, err := kit.OpenReadWrite(dbPath)
	if err != nil {
		return fmt.Errorf("pixcall: open target database %q: %w", dbPath, err)
	}
	w.db = db
	w.closer = closer

	if err := model.Migrate(w.db.WithContext(ctx)); err != nil {
		return fmt.Errorf("pixcall: migrate schema: %w", err)
	}
	return nil
}

func buildPixcallTagGroups(groups []*domain.TagGroup) ([]model.TagGroups, map[string]int64) {
	idMap := make(map[string]int64, len(groups))
	byName := make(map[string]int64, len(groups))
	out := make([]model.TagGroups, 0, len(groups))

	for idx, g := range groups {
		if g == nil || strings.TrimSpace(g.Name) == "" {
			continue
		}
		if existingID, ok := byName[g.Name]; ok {
			if g.ID != "" {
				idMap[g.ID] = existingID
			}
			continue
		}

		id := kit.NewSnowflakeID()
		pinyinStr := g.Pinyin
		if pinyinStr == "" {
			pinyinStr = kit.PinyinCamelCase(g.Name)
		}
		ranking := g.Ranking
		if ranking == 0 {
			ranking = int64(idx) * 1000000000
		}

		out = append(out, model.TagGroups{
			ID:       id,
			Revision: 1,
			Name:     g.Name,
			Pinyin:   pinyinStr,
			Color:    strPtr(g.Color),
			Ranking:  ranking,
		})
		byName[g.Name] = id
		if g.ID != "" {
			idMap[g.ID] = id
		}
	}
	return out, idMap
}

type tagDedupeKey struct {
	groupID int64
	name    string
}

func buildPixcallTags(tags []*domain.Tag, groupIDMap map[string]int64) ([]model.Tag, map[string]int64) {
	idMap := make(map[string]int64, len(tags))
	seen := make(map[tagDedupeKey]int64, len(tags))
	out := make([]model.Tag, 0, len(tags))

	for _, t := range tags {
		if t == nil || strings.TrimSpace(t.Name) == "" {
			continue
		}

		var gidVal int64
		var gidPtr *int64
		if t.GroupID != "" {
			if mappedGID, ok := groupIDMap[t.GroupID]; ok {
				gidVal = mappedGID
				gidPtr = &mappedGID
			}
		}

		key := tagDedupeKey{groupID: gidVal, name: t.Name}
		if existingID, ok := seen[key]; ok {
			if t.ID != "" {
				idMap[t.ID] = existingID
			}
			continue
		}

		pinyinStr := t.Pinyin
		category := t.Category
		if pinyinStr == "" || category == "" {
			p, c := kit.ExtractPinyin(t.Name)
			if pinyinStr == "" {
				pinyinStr = p
			}
			if category == "" {
				category = c
			}
		}
		if runes := []rune(category); len(runes) > 1 {
			category = strings.ToUpper(string(runes[:1]))
		} else if category == "" {
			category = "#"
		}

		id := kit.NewSnowflakeID()
		out = append(out, model.Tag{
			ID:       id,
			Revision: 1,
			GroupID:  gidPtr,
			Name:     t.Name,
			Pinyin:   pinyinStr,
			Category: category,
		})
		seen[key] = id
		if t.ID != "" {
			idMap[t.ID] = id
		}
	}
	return out, idMap
}

func flattenAndOrderFolders(roots []*domain.Folder) []*domain.Folder {
	byID := make(map[string]*domain.Folder)
	var all []*domain.Folder

	var collect func(f *domain.Folder, parentID string)
	collect = func(f *domain.Folder, parentID string) {
		if f == nil {
			return
		}
		if f.ID != "" {
			if _, exists := byID[f.ID]; exists {
				return
			}
		}
		copyFolder := *f
		if copyFolder.ParentID == "" && parentID != "" {
			copyFolder.ParentID = parentID
		}
		if copyFolder.ID != "" {
			byID[copyFolder.ID] = &copyFolder
		}
		all = append(all, &copyFolder)
		for _, child := range f.Children {
			collect(child, copyFolder.ID)
		}
	}

	for _, f := range roots {
		collect(f, "")
	}

	// Emit parents before children; break cycles/orphans gracefully.
	ordered := make([]*domain.Folder, 0, len(all))
	emitted := make(map[string]bool, len(all))
	visited := make(map[int]bool, len(all))

	for len(ordered) < len(all) {
		progressed := false
		for i, f := range all {
			if visited[i] {
				continue
			}
			if f.ParentID == "" || emitted[f.ParentID] || byID[f.ParentID] == nil {
				visited[i] = true
				if f.ID != "" {
					emitted[f.ID] = true
				}
				ordered = append(ordered, f)
				progressed = true
			}
		}
		if !progressed {
			// Cycle fallback: emit remaining folders in order.
			for i, f := range all {
				if !visited[i] {
					visited[i] = true
					if f.ID != "" {
						emitted[f.ID] = true
					}
					ordered = append(ordered, f)
				}
			}
		}
	}
	return ordered
}

func sanitizeEntryName(name, fallback string) string {
	cleaned := strings.TrimSpace(name)
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return fallback
	}
	return cleaned
}

func disambiguateName(used map[string]struct{}, name string, preserveExt bool) string {
	lower := strings.ToLower(name)
	if _, exists := used[lower]; !exists {
		used[lower] = struct{}{}
		return name
	}

	base, ext := name, ""
	if preserveExt {
		base, ext = kit.SplitFilename(name)
	}

	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (%d)", base, n)
		if ext != "" {
			candidate += "." + ext
		}
		candLower := strings.ToLower(candidate)
		if _, exists := used[candLower]; !exists {
			used[candLower] = struct{}{}
			return candidate
		}
	}
}

func mapAssetTags(domainTagIDs []string, tagIDMap map[string]int64) []string {
	if len(domainTagIDs) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(domainTagIDs))
	out := make([]string, 0, len(domainTagIDs))
	for _, tid := range domainTagIDs {
		mapped, ok := tagIDMap[tid]
		if !ok {
			continue
		}
		if _, dup := seen[mapped]; dup {
			continue
		}
		seen[mapped] = struct{}{}
		out = append(out, strconv.FormatInt(mapped, 10))
	}
	return out
}

func buildEntryMetadataJSON(a *domain.Asset, created, modified time.Time) string {
	hasThumb := false
	customThumb := false
	meta := entryMetadataJSON{
		ColorPalette: encodeColorPalette(a.Colors),
		Creation:     created.UTC().Format(time.RFC3339Nano),
		Modification: modified.UTC().Format(time.RFC3339Nano),
		CustomThumb:  &customThumb,
		HasThumb:     &hasThumb,
		ImageWidth:   a.Width,
		ImageHeight:  a.Height,
		Duration:     a.DurationSec,
	}
	if a.MediaMeta != nil {
		if a.MediaMeta.IsAnimated {
			v := true
			meta.IsAnimated = &v
		}
		if a.MediaMeta.Extra != nil {
			if rawOrient, ok := a.MediaMeta.Extra["orientation"]; ok {
				switch v := rawOrient.(type) {
				case int:
					meta.Orientation = v
				case float64:
					meta.Orientation = int(v)
				}
			}
		}
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return string(data)
}

func buildMediaMetadataJSON(a *domain.Asset) string {
	hasMediaFields := a.Kind == domain.KindVideo || a.Kind == domain.KindAudio || a.DurationSec > 0
	if a.MediaMeta != nil && (a.MediaMeta.Bitrate > 0 || a.MediaMeta.Rotation != 0 || a.MediaMeta.Format != "") {
		hasMediaFields = true
	}
	if !hasMediaFields {
		return ""
	}

	med := mediaMetadataJSON{
		Duration: a.DurationSec,
		Width:    a.Width,
		Height:   a.Height,
	}
	if a.MediaMeta != nil {
		med.BitRate = a.MediaMeta.Bitrate
		med.Format = a.MediaMeta.Format
		med.Rotation = float64(a.MediaMeta.Rotation)
	}
	data, err := json.Marshal(med)
	if err != nil {
		return ""
	}
	return string(data)
}

func buildSeedKvs(installUUID string) []model.Kvs {
	encodedUUID, _ := json.Marshal(installUUID)
	return []model.Kvs{
		{K: "schema_version", V: model.SchemaVersion},
		{K: "app.installation_id", V: string(encodedUUID)},
		{K: "config.library.import", V: `{"remote":{"total":null,"done":0},"fs":{"total":null,"done":0}}`},
		{K: "config.library.shutdown", V: `{"status":"clean"}`},
	}
}

func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
