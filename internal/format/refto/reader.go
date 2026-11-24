package refto

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/refto/model"
	"debris/internal/kit"
)

type reader struct {
	rootPath string
	db       *gorm.DB
	closer   io.Closer
	opts     format.ReadOptions

	closeOnce sync.Once
	closeErr  error
}

func newReader(ctx context.Context, path string, opts format.ReadOptions) (*reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, errors.New("refto: reader path is required")
	}

	rootPath := resolvePath(trimmed)
	info, err := os.Stat(rootPath)
	if err != nil {
		return nil, fmt.Errorf("refto: open reader %q: %w", rootPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("refto: %q is not a directory", rootPath)
	}

	dbPath := mainDBPath(rootPath)
	db, closer, err := kit.OpenReadOnly(dbPath)
	if err != nil {
		return nil, fmt.Errorf("refto: open database %q: %w", dbPath, err)
	}

	return &reader{
		rootPath: rootPath,
		db:       db,
		closer:   closer,
		opts:     opts,
	}, nil
}

func (r *reader) Close() error {
	r.closeOnce.Do(func() {
		if r.closer != nil {
			r.closeErr = r.closer.Close()
		}
	})
	return r.closeErr
}

type inspectStats struct {
	manifest           model.LibraryManifest
	hasManifest        bool
	userVersion        int
	suggestedTagCount  int
	linkedCount        int
	missingLinkedCount int
	versionCount       int
	stackCount         int
	livePhotoCount     int
	smartFolderCount   int
	boardCount         int
}

func (r *reader) loadInspectStats(ctx context.Context) inspectStats {
	var st inspectStats
	if raw, err := os.ReadFile(filepath.Join(r.rootPath, "act-library.json")); err == nil {
		if uErr := json.Unmarshal(raw, &st.manifest); uErr == nil {
			st.hasManifest = true
		}
	}
	_ = r.db.WithContext(ctx).Raw("PRAGMA user_version").Row().Scan(&st.userVersion)

	st.suggestedTagCount = r.countQuery(ctx, "SELECT COUNT(DISTINCT tag_id) FROM item_tags WHERE state = 'suggested'")
	st.linkedCount = r.countQuery(ctx, "SELECT COUNT(*) FROM items WHERE storage = 'linked' AND deleted_at IS NULL")
	st.missingLinkedCount = r.countQuery(ctx, "SELECT COUNT(*) FROM items WHERE storage = 'linked' AND missing != 0 AND deleted_at IS NULL")
	if r.tableExists(ctx, "versions") {
		st.versionCount = r.countQuery(ctx, "SELECT COUNT(*) FROM versions")
	}
	st.stackCount = r.countQuery(ctx, "SELECT COUNT(DISTINCT stack_id) FROM items WHERE stack_id != '' AND deleted_at IS NULL")
	st.livePhotoCount = r.countQuery(ctx, "SELECT COUNT(*) FROM items WHERE (live != '' OR live_id != '') AND deleted_at IS NULL")
	if r.tableExists(ctx, "smart_folders") {
		st.smartFolderCount = r.countQuery(ctx, "SELECT COUNT(*) FROM smart_folders")
	}
	if r.tableExists(ctx, "boards") {
		st.boardCount = r.countQuery(ctx, "SELECT COUNT(*) FROM boards")
	}
	return st
}

func (r *reader) tableExists(ctx context.Context, name string) bool {
	var count int
	err := r.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Row().Scan(&count)
	return err == nil && count > 0
}

func (r *reader) countQuery(ctx context.Context, sql string) int {
	var count int
	if err := r.db.WithContext(ctx).Raw(sql).Row().Scan(&count); err != nil {
		return 0
	}
	return count
}

func (r *reader) Inspect(ctx context.Context) (format.InspectResult, error) {
	if err := ctx.Err(); err != nil {
		return format.InspectResult{}, err
	}

	stats := r.loadInspectStats(ctx)

	var folderCount int64
	if err := r.db.WithContext(ctx).Model(&model.Folder{}).Count(&folderCount).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("refto: count folders: %w", err)
	}

	var tagGroupCount int64
	if err := r.db.WithContext(ctx).Model(&model.TagGroup{}).Count(&tagGroupCount).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("refto: count tag groups: %w", err)
	}

	var items []model.Item
	if err := r.db.WithContext(ctx).
		Select("id", "storage", "rel_path", "abs_path", "file_name", "size", "deleted_at", "archived_at", "missing").
		Find(&items).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("refto: query items: %w", err)
	}

	var (
		assetCount         int
		deletedCount       int
		hiddenCount        int
		totalBytes         int64
		warnings           []string
		includedItemIDs    = make(map[string]struct{}, len(items))
		brokenLinkedActual int
	)

	for _, item := range items {
		isDeleted := item.DeletedAt != nil
		isHidden := !isDeleted && item.ArchivedAt > 0
		if isDeleted {
			deletedCount++
			if !r.opts.IncludeDeleted {
				continue
			}
		}
		if isHidden {
			hiddenCount++
			if !r.opts.IncludeHidden {
				continue
			}
		}

		includedItemIDs[item.ID] = struct{}{}
		assetCount++
		totalBytes += item.Size

		if item.Storage == "linked" {
			if item.Missing != 0 || strings.TrimSpace(item.AbsPath) == "" {
				brokenLinkedActual++
			} else if st, err := os.Stat(item.AbsPath); err != nil || st.IsDir() {
				brokenLinkedActual++
			}
		}
	}

	var itemTags []model.ItemTag
	if err := r.db.WithContext(ctx).Find(&itemTags).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("refto: query item_tags: %w", err)
	}
	var allTags []model.Tag
	if err := r.db.WithContext(ctx).Find(&allTags).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("refto: query tags: %w", err)
	}
	suggestionOnlyTags := r.loadSuggestionOnlyTagSet(ctx)

	activeTagIDs := filterActiveTagIDs(allTags, itemTags, includedItemIDs, suggestionOnlyTags, r.opts.IncludeSuggestedTags)

	if !r.opts.IncludeSuggestedTags && stats.suggestedTagCount > 0 {
		warnings = append(warnings, fmt.Sprintf("excluding %d AI-suggested tag(s) (pass --include-suggested-tags to include)", stats.suggestedTagCount))
	}
	if brokenLinkedActual > 0 {
		warnings = append(warnings, fmt.Sprintf("%d linked asset(s) are missing or unreachable on disk", brokenLinkedActual))
	}
	if stats.versionCount > 0 {
		warnings = append(warnings, fmt.Sprintf("library contains %d item version(s); only current primary versions will be exported", stats.versionCount))
	}
	if stats.stackCount > 0 {
		warnings = append(warnings, fmt.Sprintf("library contains %d asset stack(s); stack grouping is not supported in target formats", stats.stackCount))
	}
	if stats.livePhotoCount > 0 {
		warnings = append(warnings, fmt.Sprintf("library contains %d Live Photo item(s); paired live media metadata is not preserved", stats.livePhotoCount))
	}
	if stats.smartFolderCount > 0 {
		warnings = append(warnings, fmt.Sprintf("library contains %d smart folder(s) that will not be migrated", stats.smartFolderCount))
	}
	if stats.boardCount > 0 {
		warnings = append(warnings, fmt.Sprintf("library contains %d whiteboard(s) that will not be migrated", stats.boardCount))
	}

	name := strings.TrimSpace(stats.manifest.Name)
	if name == "" {
		name = libraryNameFromPath(r.rootPath)
	}
	version := strings.TrimSpace(stats.manifest.AppVersion)
	if version == "" && stats.userVersion > 0 {
		version = strconv.Itoa(stats.userVersion)
	}

	extra := map[string]any{
		"userVersion":       stats.userVersion,
		"suggestedTagCount": stats.suggestedTagCount,
		"linkedCount":       stats.linkedCount,
		"missingLinkedCount": max(stats.missingLinkedCount, brokenLinkedActual),
		"versionCount":      stats.versionCount,
		"stackCount":        stats.stackCount,
		"livePhotoCount":    stats.livePhotoCount,
		"smartFolderCount":  stats.smartFolderCount,
		"boardCount":        stats.boardCount,
	}

	return format.InspectResult{
		Format:        FormatID,
		Version:       version,
		RootPath:      r.rootPath,
		Name:          name,
		FolderCount:   int(folderCount),
		TagGroupCount: int(tagGroupCount),
		TagCount:      len(activeTagIDs),
		AssetCount:    assetCount,
		DeletedCount:  deletedCount,
		HiddenCount:   hiddenCount,
		TotalBytes:    totalBytes,
		Warnings:      warnings,
		Extra:         extra,
	}, nil
}

func (r *reader) Read(ctx context.Context) (*domain.Library, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	stats := r.loadInspectStats(ctx)

	var rawGroups []model.TagGroup
	if err := r.db.WithContext(ctx).Order("position ASC, created_at ASC").Find(&rawGroups).Error; err != nil {
		return nil, fmt.Errorf("refto: read tag_groups: %w", err)
	}
	tagGroups := make([]*domain.TagGroup, 0, len(rawGroups))
	for _, g := range rawGroups {
		tagGroups = append(tagGroups, &domain.TagGroup{
			ID:      g.ID,
			Name:    g.Name,
			Color:   g.Color,
			Ranking: int64(g.Position),
		})
	}

	var rawFolders []model.Folder
	if err := r.db.WithContext(ctx).Order("position ASC, created_at ASC").Find(&rawFolders).Error; err != nil {
		return nil, fmt.Errorf("refto: read folders: %w", err)
	}
	folders := buildDomainFolders(rawFolders)

	var rawItems []model.Item
	if err := r.db.WithContext(ctx).Order("added_at ASC, rid ASC").Find(&rawItems).Error; err != nil {
		return nil, fmt.Errorf("refto: read items: %w", err)
	}

	var rawItemFolders []model.ItemFolder
	if err := r.db.WithContext(ctx).Order("folder_id ASC, position ASC, added_at ASC").Find(&rawItemFolders).Error; err != nil {
		return nil, fmt.Errorf("refto: read item_folders: %w", err)
	}

	var rawPalettes []model.Palette
	if err := r.db.WithContext(ctx).Order("item_id ASC, rank ASC").Find(&rawPalettes).Error; err != nil {
		return nil, fmt.Errorf("refto: read palettes: %w", err)
	}

	var rawTags []model.Tag
	if err := r.db.WithContext(ctx).Order("created_at ASC, name ASC").Find(&rawTags).Error; err != nil {
		return nil, fmt.Errorf("refto: read tags: %w", err)
	}

	var rawItemTags []model.ItemTag
	if err := r.db.WithContext(ctx).Order("created_at ASC").Find(&rawItemTags).Error; err != nil {
		return nil, fmt.Errorf("refto: read item_tags: %w", err)
	}
	suggestionOnlyTags := r.loadSuggestionOnlyTagSet(ctx)

	foldersByItem := make(map[string][]string)
	folderPosByItem := make(map[string]map[string]float64)
	for _, rel := range rawItemFolders {
		foldersByItem[rel.ItemID] = append(foldersByItem[rel.ItemID], rel.FolderID)
		if rel.Position != 0 {
			if folderPosByItem[rel.ItemID] == nil {
				folderPosByItem[rel.ItemID] = make(map[string]float64)
			}
			folderPosByItem[rel.ItemID][rel.FolderID] = rel.Position
		}
	}

	palettesByItem := make(map[string][]domain.PaletteColor)
	for _, p := range rawPalettes {
		rChan, gChan, bChan := model.UnpackRGB(p.RGB)
		palettesByItem[p.ItemID] = append(palettesByItem[p.ItemID], domain.PaletteColor{
			R:     rChan,
			G:     gChan,
			B:     bChan,
			Ratio: math.Round(p.Ratio*10000) / 100,
		})
	}

	includedItemIDs := make(map[string]struct{}, len(rawItems))
	includedItems := make([]model.Item, 0, len(rawItems))
	for _, item := range rawItems {
		isDeleted := item.DeletedAt != nil
		isHidden := !isDeleted && item.ArchivedAt > 0
		if isDeleted && !r.opts.IncludeDeleted {
			continue
		}
		if isHidden && !r.opts.IncludeHidden {
			continue
		}
		includedItemIDs[item.ID] = struct{}{}
		includedItems = append(includedItems, item)
	}

	activeTagSet := filterActiveTagIDs(rawTags, rawItemTags, includedItemIDs, suggestionOnlyTags, r.opts.IncludeSuggestedTags)
	tags := make([]*domain.Tag, 0, len(activeTagSet))
	for _, tg := range rawTags {
		if !activeTagSet[tg.ID] {
			continue
		}
		groupID := ""
		if tg.GroupID != nil {
			groupID = *tg.GroupID
		}
		tags = append(tags, &domain.Tag{
			ID:        tg.ID,
			GroupID:   groupID,
			Name:      tg.Name,
			Pinyin:    tg.Pinyin,
			Color:     tg.Color,
			Starred:   tg.Starred != 0,
			CreatedAt: msToTime(tg.CreatedAt),
		})
	}

	tagsByItem := make(map[string][]string)
	itemTagSeen := make(map[string]map[string]struct{})
	for _, rel := range rawItemTags {
		if _, ok := includedItemIDs[rel.ItemID]; !ok {
			continue
		}
		if !isItemTagIncluded(rel.State, r.opts.IncludeSuggestedTags) {
			continue
		}
		if !activeTagSet[rel.TagID] {
			continue
		}
		if itemTagSeen[rel.ItemID] == nil {
			itemTagSeen[rel.ItemID] = make(map[string]struct{})
		}
		if _, dup := itemTagSeen[rel.ItemID][rel.TagID]; dup {
			continue
		}
		itemTagSeen[rel.ItemID][rel.TagID] = struct{}{}
		tagsByItem[rel.ItemID] = append(tagsByItem[rel.ItemID], rel.TagID)
	}

	assets := make([]*domain.Asset, 0, len(includedItems))
	var latestModified time.Time
	for _, item := range includedItems {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		sourcePath, fileName := r.resolveItemSource(item)
		ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(item.Ext)), ".")
		if ext == "" && fileName != "" {
			_, ext = kit.SplitFilename(fileName)
		}
		name := item.Name
		if name == "" && fileName != "" {
			name, _ = kit.SplitFilename(fileName)
		}
		if fileName == "" && name != "" {
			if ext != "" {
				fileName = name + "." + ext
			} else {
				fileName = name
			}
		}

		size := item.Size
		if size <= 0 && sourcePath != "" {
			if st, statErr := os.Stat(sourcePath); statErr == nil && !st.IsDir() {
				size = st.Size()
			}
		}

		var createdAt time.Time
		if item.FileCTime != nil && *item.FileCTime > 0 {
			createdAt = msToTime(*item.FileCTime)
		}
		if createdAt.IsZero() {
			createdAt = msToTime(item.AddedAt)
		}
		modifiedAt := msToTime(item.UpdatedAt)
		if item.FileMTime != nil && *item.FileMTime > 0 && modifiedAt.IsZero() {
			modifiedAt = msToTime(*item.FileMTime)
		}
		if modifiedAt.After(latestModified) {
			latestModified = modifiedAt
		}

		mediaMeta := r.buildItemMediaMeta(item, folderPosByItem[item.ID])

		isDeleted := item.DeletedAt != nil
		isHidden := !isDeleted && item.ArchivedAt > 0

		assets = append(assets, &domain.Asset{
			ID:          item.ID,
			FileName:    fileName,
			Name:        name,
			Ext:         ext,
			SourcePath:  sourcePath,
			Size:        size,
			Hash:        item.Hash,
			Kind:        classifyDomainKind(item.Kind, ext),
			Width:       item.Width,
			Height:      item.Height,
			DurationSec: item.Duration,
			Rating:      item.Rating,
			Annotation:  item.Note,
			OriginURL:   item.SourceURL,
			Colors:      palettesByItem[item.ID],
			FolderIDs:   foldersByItem[item.ID],
			TagIDs:      tagsByItem[item.ID],
			IsHidden:    isHidden,
			IsDeleted:   isDeleted,
			CreatedAt:   createdAt,
			ModifiedAt:  modifiedAt,
			MediaMeta:   mediaMeta,
		})
	}

	name := strings.TrimSpace(stats.manifest.Name)
	if name == "" {
		name = libraryNameFromPath(r.rootPath)
	}
	version := strings.TrimSpace(stats.manifest.AppVersion)
	if version == "" && stats.userVersion > 0 {
		version = strconv.Itoa(stats.userVersion)
	}

	return &domain.Library{
		Format:     FormatID,
		Version:    version,
		RootPath:   r.rootPath,
		Name:       name,
		CreatedAt:  stats.manifest.CreatedAt,
		ModifiedAt: latestModified,
		Folders:    folders,
		TagGroups:  tagGroups,
		Tags:       tags,
		Assets:     assets,
	}, nil
}

func (r *reader) loadSuggestionOnlyTagSet(ctx context.Context) map[string]bool {
	out := make(map[string]bool)
	if !r.tableExists(ctx, "suggestion_tags") {
		return out
	}
	var ids []string
	if err := r.db.WithContext(ctx).Raw("SELECT tag_id FROM suggestion_tags").Scan(&ids).Error; err == nil {
		for _, id := range ids {
			out[id] = true
		}
	}
	return out
}

func isItemTagIncluded(state string, includeSuggested bool) bool {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "accepted", "":
		return true
	case "suggested":
		return includeSuggested
	default:
		return false
	}
}

func filterActiveTagIDs(
	allTags []model.Tag,
	itemTags []model.ItemTag,
	includedItems map[string]struct{},
	suggestionOnlyTags map[string]bool,
	includeSuggested bool,
) map[string]bool {
	active := make(map[string]bool)
	for _, rel := range itemTags {
		if _, ok := includedItems[rel.ItemID]; !ok {
			continue
		}
		if isItemTagIncluded(rel.State, includeSuggested) {
			active[rel.TagID] = true
		}
	}
	// Preserve user-curated standalone tags (e.g. grouped or starred) even if unattached.
	for _, tg := range allTags {
		if active[tg.ID] {
			continue
		}
		if includeSuggested {
			active[tg.ID] = true
			continue
		}
		if !suggestionOnlyTags[tg.ID] && (tg.Starred != 0 || (tg.GroupID != nil && *tg.GroupID != "")) {
			active[tg.ID] = true
		}
	}
	return active
}

func buildDomainFolders(rows []model.Folder) []*domain.Folder {
	if len(rows) == 0 {
		return nil
	}
	nodes := make(map[string]*domain.Folder, len(rows))
	orderedIDs := make([]string, 0, len(rows))
	for _, f := range rows {
		parentID := ""
		if f.ParentID != nil {
			parentID = *f.ParentID
		}
		coverID := ""
		if f.CoverItemID != nil {
			coverID = *f.CoverItemID
		}
		var tagNames []string
		if trimmed := strings.TrimSpace(f.AutoTags); trimmed != "" && trimmed != "[]" {
			_ = json.Unmarshal([]byte(trimmed), &tagNames)
		}

		nodes[f.ID] = &domain.Folder{
			ID:           f.ID,
			ParentID:     parentID,
			Name:         f.Name,
			Description:  f.Description,
			Color:        f.Color,
			Seq:          int64(f.Position),
			Icon:         f.Icon,
			CreatedAt:    msToTime(f.CreatedAt),
			ModifiedAt:   msToTime(f.UpdatedAt),
			TagNames:     tagNames,
			CoverAssetID: coverID,
		}
		orderedIDs = append(orderedIDs, f.ID)
	}

	var roots []*domain.Folder
	for _, id := range orderedIDs {
		node := nodes[id]
		if node.ParentID == "" || node.ParentID == node.ID || nodes[node.ParentID] == nil {
			roots = append(roots, node)
			continue
		}
		parent := nodes[node.ParentID]
		parent.Children = append(parent.Children, node)
		parent.ChildIDs = append(parent.ChildIDs, node.ID)
	}

	var sortChildren func([]*domain.Folder)
	sortChildren = func(list []*domain.Folder) {
		slices.SortStableFunc(list, func(a, b *domain.Folder) int {
			return cmp.Compare(a.Seq, b.Seq)
		})
		for _, f := range list {
			if len(f.Children) > 0 {
				sortChildren(f.Children)
				f.ChildIDs = make([]string, 0, len(f.Children))
				for _, c := range f.Children {
					f.ChildIDs = append(f.ChildIDs, c.ID)
				}
			}
		}
	}
	sortChildren(roots)
	return roots
}

func (r *reader) resolveItemSource(item model.Item) (fullPath, fileName string) {
	if item.Storage == "linked" {
		abs := strings.TrimSpace(item.AbsPath)
		name := strings.TrimSpace(item.FileName)
		if name == "" && abs != "" {
			name = filepath.Base(abs)
		}
		if abs != "" {
			if st, err := os.Stat(abs); err == nil && !st.IsDir() {
				return abs, name
			}
		}
		return abs, name
	}

	itemDir := filepath.Join(r.rootPath, "items", item.ID)
	candidates := make([]string, 0, 2)
	if strings.TrimSpace(item.RelPath) != "" {
		candidates = append(candidates, item.RelPath)
	}
	if strings.TrimSpace(item.FileName) != "" && item.FileName != item.RelPath {
		candidates = append(candidates, item.FileName)
	}
	for _, rel := range candidates {
		p := filepath.Join(itemDir, filepath.FromSlash(rel))
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, filepath.Base(p)
		}
	}

	// Fallback scan inside items/<id>/ excluding item.json and cover/sidecar files.
	entries, err := os.ReadDir(itemDir)
	if err != nil {
		return "", item.FileName
	}
	var fallback string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if n == "item.json" || n == ".DS_Store" || (item.Cover != "" && n == item.Cover) {
			continue
		}
		if fallback == "" {
			fallback = n
		} else {
			fallback = ""
			break
		}
	}
	if fallback != "" {
		return filepath.Join(itemDir, fallback), fallback
	}
	return "", item.FileName
}

func (r *reader) buildItemMediaMeta(item model.Item, folderPositions map[string]float64) *domain.MediaMeta {
	var thumbPath string
	if strings.TrimSpace(item.Cover) != "" {
		cand := filepath.Join(r.rootPath, "items", item.ID, filepath.FromSlash(item.Cover))
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			thumbPath = cand
		}
	}
	if thumbPath == "" && item.ThumbState == "ready" && strings.TrimSpace(item.ThumbExt) != "" && len(item.ID) >= 2 {
		bucket := strings.ToLower(item.ID[len(item.ID)-2:])
		cand := filepath.Join(r.rootPath, "thumbs", bucket, item.ID+"."+strings.TrimPrefix(item.ThumbExt, "."))
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			thumbPath = cand
		}
	}

	var (
		formatStr string
		exifMap   map[string]any
		extra     map[string]any
	)
	if trimmed := strings.TrimSpace(item.Meta); trimmed != "" {
		var metaObj map[string]any
		if err := json.Unmarshal([]byte(trimmed), &metaObj); err == nil && len(metaObj) > 0 {
			if c, ok := metaObj["container"].(string); ok {
				formatStr = c
			}
			if exifRaw, ok := metaObj["exif"].(map[string]any); ok && len(exifRaw) > 0 {
				exifMap = exifRaw
			}
			extra = map[string]any{
				"reftoMeta": metaObj,
			}
		}
	}
	if len(folderPositions) > 0 {
		if extra == nil {
			extra = make(map[string]any)
		}
		extra["reftoFolderPositions"] = folderPositions
	}
	if item.PinnedAt > 0 {
		if extra == nil {
			extra = make(map[string]any)
		}
		extra["pinnedAt"] = item.PinnedAt
	}

	noThumb := item.ThumbState == "none"
	if thumbPath == "" && !noThumb && formatStr == "" && len(exifMap) == 0 && len(extra) == 0 {
		return nil
	}
	return &domain.MediaMeta{
		Format:        formatStr,
		EXIF:          exifMap,
		ThumbnailPath: thumbPath,
		NoThumbnail:   noThumb,
		Extra:         extra,
	}
}

func classifyDomainKind(reftoKind, ext string) domain.AssetKind {
	switch strings.ToLower(strings.TrimSpace(reftoKind)) {
	case "image":
		return domain.KindImage
	case "video":
		return domain.KindVideo
	case "audio":
		return domain.KindAudio
	case "font":
		return domain.KindFont
	}
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".") {
	case "psd", "psb", "ai", "sketch", "fig", "xd", "eps", "cdr",
		"afdesign", "afphoto", "clip", "kra", "blend", "c4d", "prproj", "aep":
		return domain.KindProject
	default:
		return domain.KindOther
	}
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
