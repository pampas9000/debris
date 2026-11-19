package pixcall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
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

// Reader implements format.Reader for Pixcall libraries.
type Reader struct {
	rootPath string
	db       *gorm.DB
	closer   io.Closer
	opts     format.ReadOptions

	closeOnce sync.Once
	closeErr  error
}

// OpenReader opens a Pixcall library at path in strict read-only mode.
func (a *Adapter) OpenReader(ctx context.Context, path string, opts format.ReadOptions) (format.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("pixcall: empty library path")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("pixcall: resolve library path %q: %w", path, err)
	}

	dbPath := mainDBPath(absPath)
	db, closer, err := kit.OpenReadOnly(dbPath)
	if err != nil {
		return nil, fmt.Errorf("pixcall: open database %q: %w", dbPath, err)
	}

	return &Reader{
		rootPath: absPath,
		db:       db,
		closer:   closer,
		opts:     opts,
	}, nil
}

// Close closes the underlying read-only SQLite connection idempotently.
func (r *Reader) Close() error {
	r.closeOnce.Do(func() {
		if r.closer != nil {
			r.closeErr = r.closer.Close()
		}
	})
	return r.closeErr
}

type entryLineage struct {
	isDeleted bool
	isHidden  bool
	inTrash   bool
	dirParts  []string // folder names from root (id=1 or id=2) down to this entry's parent
}

func resolveEntryLineage(entry model.Entry, byID map[int64]model.Entry) entryLineage {
	res := entryLineage{
		isDeleted: entry.IsDeleted != 0 || entry.ID == trashEntryID,
		isHidden:  entry.IsHidden != 0,
		inTrash:   entry.ID == trashEntryID,
	}

	var reversed []string
	visited := map[int64]struct{}{entry.ID: {}}
	currParent := entry.ParentID

	for currParent > 0 {
		if currParent == rootEntryID {
			break
		}
		if currParent == trashEntryID {
			res.isDeleted = true
			res.inTrash = true
			break
		}
		if _, seen := visited[currParent]; seen {
			break
		}
		visited[currParent] = struct{}{}

		parent, ok := byID[currParent]
		if !ok {
			break
		}
		if parent.IsDeleted != 0 {
			res.isDeleted = true
		}
		if parent.IsHidden != 0 {
			res.isHidden = true
		}
		if parent.Kind == 0 && parent.Name != "" {
			reversed = append(reversed, parent.Name)
		}
		currParent = parent.ParentID
	}

	if len(reversed) > 0 {
		res.dirParts = make([]string, len(reversed))
		for i := range reversed {
			res.dirParts[i] = reversed[len(reversed)-1-i]
		}
	}
	return res
}

func (r *Reader) readSchemaVersion(ctx context.Context) (string, error) {
	var kvs []model.Kvs
	if err := r.db.WithContext(ctx).Where("k = ?", "schema_version").Limit(1).Find(&kvs).Error; err != nil {
		return "", fmt.Errorf("pixcall: query schema_version: %w", err)
	}
	if len(kvs) == 0 {
		return "", nil
	}
	return strings.TrimSpace(kvs[0].V), nil
}

// Inspect returns summary counts and metadata for the Pixcall library.
func (r *Reader) Inspect(ctx context.Context) (format.InspectResult, error) {
	if err := ctx.Err(); err != nil {
		return format.InspectResult{}, err
	}

	version, err := r.readSchemaVersion(ctx)
	if err != nil {
		return format.InspectResult{}, err
	}

	var tagGroupCount, tagCount int64
	if err := r.db.WithContext(ctx).Model(&model.TagGroups{}).Count(&tagGroupCount).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("pixcall: count tag_groups: %w", err)
	}
	if err := r.db.WithContext(ctx).Model(&model.Tag{}).Count(&tagCount).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("pixcall: count tags: %w", err)
	}

	var entries []model.Entry
	if err := r.db.WithContext(ctx).
		Select("id, parent_id, name, kind, size, is_hidden, is_deleted").
		Find(&entries).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("pixcall: query entries: %w", err)
	}

	byID := make(map[int64]model.Entry, len(entries))
	for _, e := range entries {
		byID[e.ID] = e
	}

	res := format.InspectResult{
		Format:        formatID,
		Version:       version,
		RootPath:      r.rootPath,
		Name:          deriveLibraryName(r.rootPath),
		TagGroupCount: int(tagGroupCount),
		TagCount:      int(tagCount),
	}

	for _, e := range entries {
		if e.ID == rootEntryID || e.ID == trashEntryID {
			continue
		}
		lineage := resolveEntryLineage(e, byID)
		included := (r.opts.IncludeDeleted || !lineage.isDeleted) && (r.opts.IncludeHidden || !lineage.isHidden)

		switch e.Kind {
		case 0:
			if included {
				res.FolderCount++
			}
		case 1:
			if lineage.isDeleted {
				res.DeletedCount++
			}
			if lineage.isHidden {
				res.HiddenCount++
			}
			if included {
				res.AssetCount++
				res.TotalBytes += e.Size
			}
		}
	}

	return res, nil
}

// Read loads the full Pixcall library into the domain.Library IR.
func (r *Reader) Read(ctx context.Context) (*domain.Library, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	version, err := r.readSchemaVersion(ctx)
	if err != nil {
		return nil, err
	}

	var rawTagGroups []model.TagGroups
	if err := r.db.WithContext(ctx).Order("ranking ASC, id ASC").Find(&rawTagGroups).Error; err != nil {
		return nil, fmt.Errorf("pixcall: query tag_groups: %w", err)
	}

	var rawTags []model.Tag
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&rawTags).Error; err != nil {
		return nil, fmt.Errorf("pixcall: query tags: %w", err)
	}

	var rawEntries []model.Entry
	if err := r.db.WithContext(ctx).Order("parent_id ASC, ranking DESC, id ASC").Find(&rawEntries).Error; err != nil {
		return nil, fmt.Errorf("pixcall: query entries: %w", err)
	}

	var rawFolders []model.Folders
	if err := r.db.WithContext(ctx).Find(&rawFolders).Error; err != nil {
		return nil, fmt.Errorf("pixcall: query folders: %w", err)
	}

	var rawMedia []model.Media
	if err := r.db.WithContext(ctx).Find(&rawMedia).Error; err != nil {
		return nil, fmt.Errorf("pixcall: query media: %w", err)
	}

	var rawExif []model.Exif
	if err := r.db.WithContext(ctx).Find(&rawExif).Error; err != nil {
		return nil, fmt.Errorf("pixcall: query exif: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	lib := &domain.Library{
		Format:    formatID,
		Version:   version,
		RootPath:  r.rootPath,
		Name:      deriveLibraryName(r.rootPath),
		TagGroups: make([]*domain.TagGroup, 0, len(rawTagGroups)),
		Tags:      make([]*domain.Tag, 0, len(rawTags)),
	}

	for _, tg := range rawTagGroups {
		lib.TagGroups = append(lib.TagGroups, &domain.TagGroup{
			ID:      strconv.FormatInt(tg.ID, 10),
			Name:    tg.Name,
			Color:   derefString(tg.Color),
			Pinyin:  tg.Pinyin,
			Ranking: tg.Ranking,
		})
	}

	for _, t := range rawTags {
		var groupID string
		if t.GroupID != nil && *t.GroupID > 0 {
			groupID = strconv.FormatInt(*t.GroupID, 10)
		}
		lib.Tags = append(lib.Tags, &domain.Tag{
			ID:       strconv.FormatInt(t.ID, 10),
			GroupID:  groupID,
			Name:     t.Name,
			Pinyin:   t.Pinyin,
			Category: t.Category,
		})
	}

	entryByID := make(map[int64]model.Entry, len(rawEntries))
	for _, e := range rawEntries {
		entryByID[e.ID] = e
	}
	if root, ok := entryByID[rootEntryID]; ok {
		lib.CreatedAt = root.CreatedAt.Time
		lib.ModifiedAt = root.UpdatedAt.Time
	}

	folderByEntryID := make(map[int64]model.Folders, len(rawFolders))
	for _, f := range rawFolders {
		folderByEntryID[f.EntryID] = f
	}

	mediaByEntryID := make(map[int64]mediaMetadataJSON, len(rawMedia))
	for _, m := range rawMedia {
		if strings.TrimSpace(m.Metadata) == "" {
			continue
		}
		var parsed mediaMetadataJSON
		if json.Unmarshal([]byte(m.Metadata), &parsed) == nil {
			mediaByEntryID[m.EntryID] = parsed
		}
	}

	exifByEntryID := make(map[int64]map[string]any, len(rawExif))
	for _, ex := range rawExif {
		if strings.TrimSpace(ex.Metadata) == "" {
			continue
		}
		var parsed map[string]any
		if json.Unmarshal([]byte(ex.Metadata), &parsed) == nil && len(parsed) > 0 {
			exifByEntryID[ex.EntryID] = parsed
		}
	}

	folderDomainByID := make(map[string]*domain.Folder)
	includedFolders := make(map[int64]bool)

	for _, e := range rawEntries {
		if e.Kind != 0 || e.ID == rootEntryID || e.ID == trashEntryID {
			continue
		}
		lineage := resolveEntryLineage(e, entryByID)
		if !r.opts.IncludeDeleted && lineage.isDeleted {
			continue
		}
		if !r.opts.IncludeHidden && lineage.isHidden {
			continue
		}

		idStr := strconv.FormatInt(e.ID, 10)
		var parentID string
		if e.ParentID > 0 && e.ParentID != rootEntryID && e.ParentID != trashEntryID {
			parentID = strconv.FormatInt(e.ParentID, 10)
		}

		df := &domain.Folder{
			ID:          idStr,
			ParentID:    parentID,
			Name:        e.Name,
			Description: derefString(e.Description),
			Seq:         e.Ranking,
			IsRecycle:   lineage.isDeleted,
			IsHidden:    lineage.isHidden,
			CreatedAt:   e.CreatedAt.Time,
			ModifiedAt:  e.UpdatedAt.Time,
		}
		if fm, ok := folderByEntryID[e.ID]; ok {
			df.Icon = derefString(fm.Icon)
			df.IconColor = derefString(fm.IconColor)
			if fm.CoverID != nil && *fm.CoverID > 0 {
				df.CoverAssetID = strconv.FormatInt(*fm.CoverID, 10)
			} else if fm.FixedCoverID != nil && *fm.FixedCoverID > 0 {
				df.CoverAssetID = strconv.FormatInt(*fm.FixedCoverID, 10)
			}
			if fm.Ranking != 0 && df.Seq == 0 {
				df.Seq = fm.Ranking
			}
			extra := make(map[string]any)
			if fm.Layout != nil && *fm.Layout != "" {
				extra["layout"] = *fm.Layout
			}
			if fm.SortBy != nil && *fm.SortBy != "" {
				extra["sortBy"] = *fm.SortBy
			}
			if fm.SortOrder != nil && *fm.SortOrder != "" {
				extra["sortOrder"] = *fm.SortOrder
			}
			if len(extra) > 0 {
				df.Extra = extra
			}
		}

		lib.Folders = append(lib.Folders, df)
		folderDomainByID[idStr] = df
		includedFolders[e.ID] = true
	}

	// Populate ChildIDs after all included folders are collected.
	for _, df := range lib.Folders {
		if df.ParentID != "" {
			if parent, ok := folderDomainByID[df.ParentID]; ok {
				parent.ChildIDs = append(parent.ChildIDs, df.ID)
			} else {
				df.ParentID = ""
			}
		}
	}

	for _, e := range rawEntries {
		if e.Kind != 1 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		lineage := resolveEntryLineage(e, entryByID)
		if !r.opts.IncludeDeleted && lineage.isDeleted {
			continue
		}
		if !r.opts.IncludeHidden && lineage.isHidden {
			continue
		}

		asset := r.buildDomainAsset(e, lineage, includedFolders, mediaByEntryID, exifByEntryID)
		lib.Assets = append(lib.Assets, asset)
	}

	return lib, nil
}

func (r *Reader) buildDomainAsset(
	e model.Entry,
	lineage entryLineage,
	includedFolders map[int64]bool,
	mediaByEntryID map[int64]mediaMetadataJSON,
	exifByEntryID map[int64]map[string]any,
) *domain.Asset {
	base, ext := kit.SplitFilename(e.Name)
	asset := &domain.Asset{
		ID:         strconv.FormatInt(e.ID, 10),
		FileName:   e.Name,
		Name:       base,
		Ext:        ext,
		SourcePath: resolveAssetSourcePath(r.rootPath, e, lineage),
		Size:       e.Size,
		Hash:       derefString(e.ContentHash),
		Kind:       classifyAssetKind(e.ContentType, ext),
		Rating:     derefInt(e.Rating),
		Annotation: derefString(e.Description),
		OriginURL:  derefString(e.Link),
		TagIDs:     splitPipeIDs(derefString(e.Tags)),
		IsHidden:   lineage.isHidden,
		IsDeleted:  lineage.isDeleted,
		CreatedAt:  e.CreatedAt.Time,
		ModifiedAt: e.UpdatedAt.Time,
	}

	if e.ParentID > 0 && e.ParentID != rootEntryID && e.ParentID != trashEntryID && includedFolders[e.ParentID] {
		asset.FolderIDs = []string{strconv.FormatInt(e.ParentID, 10)}
	}

	if e.Mtime != nil && *e.Mtime > 0 {
		asset.ModifiedAt = time.UnixMilli(*e.Mtime).UTC()
	}

	var entryMeta entryMetadataJSON
	if e.Metadata != nil && strings.TrimSpace(*e.Metadata) != "" {
		_ = json.Unmarshal([]byte(*e.Metadata), &entryMeta)
	}

	asset.Width = entryMeta.ImageWidth
	asset.Height = entryMeta.ImageHeight
	asset.DurationSec = entryMeta.Duration
	asset.Colors = parseColorPalette(entryMeta.ColorPalette)

	if asset.CreatedAt.IsZero() && entryMeta.Creation != "" {
		if t, err := time.Parse(time.RFC3339Nano, entryMeta.Creation); err == nil {
			asset.CreatedAt = t
		}
	}
	if asset.ModifiedAt.IsZero() && entryMeta.Modification != "" {
		if t, err := time.Parse(time.RFC3339Nano, entryMeta.Modification); err == nil {
			asset.ModifiedAt = t
		}
	}

	var mm *domain.MediaMeta
	ensureMM := func() *domain.MediaMeta {
		if mm == nil {
			mm = &domain.MediaMeta{}
		}
		return mm
	}

	if entryMeta.IsAnimated != nil && *entryMeta.IsAnimated {
		ensureMM().IsAnimated = true
	}
	if entryMeta.Orientation != 0 {
		m := ensureMM()
		if m.Extra == nil {
			m.Extra = make(map[string]any)
		}
		m.Extra["orientation"] = entryMeta.Orientation
	}

	if med, ok := mediaByEntryID[e.ID]; ok {
		if asset.Width == 0 {
			asset.Width = med.Width
		}
		if asset.Height == 0 {
			asset.Height = med.Height
		}
		if asset.DurationSec == 0 {
			asset.DurationSec = med.Duration
		}
		if med.BitRate > 0 || med.Format != "" || med.Rotation != 0 || len(med.Extra) > 0 {
			m := ensureMM()
			m.Bitrate = med.BitRate
			m.Format = med.Format
			if med.Rotation != 0 {
				m.Rotation = int(math.Round(med.Rotation))
			}
			if len(med.Extra) > 0 {
				if m.Extra == nil {
					m.Extra = make(map[string]any)
				}
				for k, v := range med.Extra {
					m.Extra[k] = v
				}
			}
		}
	}

	if exifData, ok := exifByEntryID[e.ID]; ok {
		ensureMM().EXIF = exifData
	}

	asset.MediaMeta = mm
	return asset
}

func resolveAssetSourcePath(rootPath string, e model.Entry, lineage entryLineage) string {
	normalParts := make([]string, 0, len(lineage.dirParts)+2)
	normalParts = append(normalParts, rootPath)
	normalParts = append(normalParts, lineage.dirParts...)
	normalParts = append(normalParts, e.Name)
	normalPath := filepath.Join(normalParts...)

	if !lineage.isDeleted {
		return normalPath
	}

	trashNestedParts := make([]string, 0, len(lineage.dirParts)+4)
	trashNestedParts = append(trashNestedParts, rootPath, ".pixcall", "trash")
	trashNestedParts = append(trashNestedParts, lineage.dirParts...)
	trashNestedParts = append(trashNestedParts, e.Name)
	trashNestedPath := filepath.Join(trashNestedParts...)
	trashFlatPath := filepath.Join(rootPath, ".pixcall", "trash", e.Name)

	for _, candidate := range []string{trashNestedPath, trashFlatPath, normalPath} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	if e.SourcePath != nil && *e.SourcePath != "" {
		sp := *e.SourcePath
		if !filepath.IsAbs(sp) {
			sp = filepath.Join(rootPath, sp)
		}
		if _, err := os.Stat(sp); err == nil {
			return sp
		}
	}

	return trashFlatPath
}
