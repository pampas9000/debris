package billfish

import (
	"context"
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

	"debris/internal/domain"
	"debris/internal/format"
	bfmodel "debris/internal/format/billfish/model"
	"debris/internal/kit"
	"gorm.io/gorm"
)

// Reader reads a Billfish library database read-only into domain IR.
type Reader struct {
	rootPath string
	opts     format.ReadOptions
	db       *gorm.DB
	closer   io.Closer

	closeMu sync.Mutex
	closed  bool
}

func newReader(ctx context.Context, path string, opts format.ReadOptions) (*Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("billfish: empty library path")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("billfish: resolve path %q: %w", path, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("billfish: stat library %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("billfish: library path %q is not a directory", absPath)
	}

	dbPath := billfishDBPath(absPath)
	db, closer, err := kit.OpenReadOnly(dbPath)
	if err != nil {
		return nil, fmt.Errorf("billfish: open database: %w", err)
	}

	return &Reader{
		rootPath: absPath,
		opts:     opts,
		db:       db,
		closer:   closer,
	}, nil
}

// Close closes the underlying read-only SQLite connection.
func (r *Reader) Close() error {
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

func (r *Reader) ensureOpen() error {
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	if r.closed || r.db == nil {
		return errors.New("billfish: reader is closed")
	}
	return nil
}

// Inspect summarizes the Billfish library counts and total bytes without loading payloads.
func (r *Reader) Inspect(ctx context.Context) (format.InspectResult, error) {
	if err := ctx.Err(); err != nil {
		return format.InspectResult{}, err
	}
	if err := r.ensureOpen(); err != nil {
		return format.InspectResult{}, err
	}

	var libRow bfmodel.Library
	if err := r.db.WithContext(ctx).Order("id ASC").Limit(1).Find(&libRow).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("billfish: query library metadata: %w", err)
	}

	var folders []bfmodel.Folder
	if err := r.db.WithContext(ctx).Order("seq ASC, id ASC").Find(&folders).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("billfish: query bf_folder: %w", err)
	}

	var tagGroupCount int64
	if err := r.db.WithContext(ctx).Model(&bfmodel.TagGroup{}).Count(&tagGroupCount).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("billfish: count bf_tag_group: %w", err)
	}

	var tagCount int64
	if err := r.db.WithContext(ctx).Model(&bfmodel.Tag{}).Count(&tagCount).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("billfish: count bf_tag_v2: %w", err)
	}

	var files []bfmodel.File
	if err := r.db.WithContext(ctx).Select("id, pid, is_hide, file_size").Order("id ASC").Find(&files).Error; err != nil {
		return format.InspectResult{}, fmt.Errorf("billfish: query bf_file: %w", err)
	}

	var materials []bfmodel.Material
	if err := optionalFind(ctx, r.db, "bf_material_v2", &materials, "file_id, is_recycle"); err != nil {
		return format.InspectResult{}, fmt.Errorf("billfish: query bf_material_v2: %w", err)
	}
	matRecycle := make(map[int64]bool, len(materials))
	for _, m := range materials {
		if m.IsRecycle != 0 {
			matRecycle[m.FileID] = true
		}
	}

	var recycles []bfmodel.Recycle
	if err := optionalFind(ctx, r.db, "bf_recycle", &recycles, "file_id"); err != nil {
		return format.InspectResult{}, fmt.Errorf("billfish: query bf_recycle: %w", err)
	}
	recycledFileIDs := make(map[int64]bool, len(recycles))
	for _, rec := range recycles {
		recycledFileIDs[rec.FileID] = true
	}

	folderHelper := newFolderTreeHelper(folders)

	visibleFolders := 0
	for _, f := range folders {
		if !r.opts.IncludeDeleted && folderHelper.isRecycled(f.ID) {
			continue
		}
		if !r.opts.IncludeHidden && folderHelper.isHidden(f.ID) {
			continue
		}
		visibleFolders++
	}

	res := format.InspectResult{
		Format:        FormatID,
		Version:       AppVersionForSchema(libRow.Version),
		RootPath:      r.rootPath,
		Name:          readLibraryName(r.rootPath),
		FolderCount:   visibleFolders,
		TagGroupCount: int(tagGroupCount),
		TagCount:      int(tagCount),
	}
	extra := make(map[string]any)
	if libRow.Version > 0 {
		extra["schemaVersion"] = libRow.Version
	}
	if libRow.Platform != "" {
		extra["platform"] = libRow.Platform
	}
	if len(extra) > 0 {
		res.Extra = extra
	}

	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return format.InspectResult{}, err
		}
		isDeleted := f.Pid == -1 || matRecycle[f.ID] || recycledFileIDs[f.ID] || folderHelper.isRecycled(f.Pid)
		isHidden := f.IsHide != 0 || folderHelper.isHidden(f.Pid)
		if isDeleted {
			res.DeletedCount++
		}
		if isHidden {
			res.HiddenCount++
		}
		if !r.opts.IncludeDeleted && isDeleted {
			continue
		}
		if !r.opts.IncludeHidden && isHidden {
			continue
		}
		res.AssetCount++
		res.TotalBytes += f.FileSize
	}

	return res, nil
}

// Read loads the complete Billfish library metadata and file paths into domain.Library.
func (r *Reader) Read(ctx context.Context) (*domain.Library, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.ensureOpen(); err != nil {
		return nil, err
	}

	var libRow bfmodel.Library
	if err := r.db.WithContext(ctx).Order("id ASC").Limit(1).Find(&libRow).Error; err != nil {
		return nil, fmt.Errorf("billfish: query library metadata: %w", err)
	}

	var rawFolders []bfmodel.Folder
	if err := r.db.WithContext(ctx).Order("seq ASC, id ASC").Find(&rawFolders).Error; err != nil {
		return nil, fmt.Errorf("billfish: query bf_folder: %w", err)
	}

	var rawTagGroups []bfmodel.TagGroup
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&rawTagGroups).Error; err != nil {
		return nil, fmt.Errorf("billfish: query bf_tag_group: %w", err)
	}

	var rawTags []bfmodel.Tag
	if err := r.db.WithContext(ctx).Order("seq ASC, id ASC").Find(&rawTags).Error; err != nil {
		return nil, fmt.Errorf("billfish: query bf_tag_v2: %w", err)
	}

	var rawTagJoinGroup []bfmodel.TagJoinGroup
	if err := optionalFind(ctx, r.db, "bf_tag_join_group", &rawTagJoinGroup, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_tag_join_group: %w", err)
	}

	var rawTagJoinFolder []bfmodel.TagJoinFolder
	if err := optionalFind(ctx, r.db, "bf_tag_join_folder", &rawTagJoinFolder, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_tag_join_folder: %w", err)
	}

	var rawFiles []bfmodel.File
	if err := r.db.WithContext(ctx).Order("id ASC").Find(&rawFiles).Error; err != nil {
		return nil, fmt.Errorf("billfish: query bf_file: %w", err)
	}

	var rawMaterials []bfmodel.Material
	if err := optionalFind(ctx, r.db, "bf_material_v2", &rawMaterials, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_material_v2: %w", err)
	}

	var rawUserdata []bfmodel.MaterialUserData
	if err := optionalFind(ctx, r.db, "bf_material_userdata", &rawUserdata, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_material_userdata: %w", err)
	}

	var rawVideos []bfmodel.MaterialVideo
	if err := optionalFind(ctx, r.db, "bf_material_video", &rawVideos, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_material_video: %w", err)
	}

	var rawImages []bfmodel.MaterialImage
	if err := optionalFind(ctx, r.db, "bf_material_image", &rawImages, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_material_image: %w", err)
	}

	var rawAudios []bfmodel.MaterialAudio
	if err := optionalFind(ctx, r.db, "bf_material_audio", &rawAudios, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_material_audio: %w", err)
	}

	var rawFonts []bfmodel.MaterialFont
	if err := optionalFind(ctx, r.db, "bf_material_font", &rawFonts, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_material_font: %w", err)
	}

	var rawProjects []bfmodel.MaterialProject
	if err := optionalFind(ctx, r.db, "bf_material_project", &rawProjects, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_material_project: %w", err)
	}

	var rawTagJoinFile []bfmodel.TagJoinFile
	if err := optionalFind(ctx, r.db, "bf_tag_join_file", &rawTagJoinFile, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_tag_join_file: %w", err)
	}

	var rawRecycles []bfmodel.Recycle
	if err := optionalFind(ctx, r.db, "bf_recycle", &rawRecycles, ""); err != nil {
		return nil, fmt.Errorf("billfish: query bf_recycle: %w", err)
	}

	// Build tag maps strictly by ID (never by name, since duplicate names are valid in Billfish).
	tagNameByID := make(map[int64]string, len(rawTags))
	for _, t := range rawTags {
		tagNameByID[t.ID] = t.Name
	}

	tagGroupIDByTagID := make(map[int64]int64, len(rawTagJoinGroup))
	for _, j := range rawTagJoinGroup {
		if _, exists := tagGroupIDByTagID[j.TagID]; !exists && j.Gid > 0 {
			tagGroupIDByTagID[j.TagID] = j.Gid
		}
	}

	tagGroupExists := make(map[int64]bool, len(rawTagGroups))
	domainTagGroups := make([]*domain.TagGroup, 0, len(rawTagGroups))
	for _, g := range rawTagGroups {
		tagGroupExists[g.ID] = true
		var parentID string
		if g.Pid > 0 {
			parentID = strconv.FormatInt(g.Pid, 10)
		}
		domainTagGroups = append(domainTagGroups, &domain.TagGroup{
			ID:       strconv.FormatInt(g.ID, 10),
			ParentID: parentID,
			Name:     g.Name,
			Color:    intToHexColor(g.Color),
			Ranking:  g.ID,
		})
	}

	domainTags := make([]*domain.Tag, 0, len(rawTags))
	for _, t := range rawTags {
		var parentID string
		if t.Pid > 0 {
			parentID = strconv.FormatInt(t.Pid, 10)
		}
		var groupID string
		if gid := tagGroupIDByTagID[t.ID]; gid > 0 && tagGroupExists[gid] {
			groupID = strconv.FormatInt(gid, 10)
		}
		var icon string
		if t.Icon != 0 {
			icon = strconv.Itoa(t.Icon)
		}
		domainTags = append(domainTags, &domain.Tag{
			ID:        strconv.FormatInt(t.ID, 10),
			ParentID:  parentID,
			GroupID:   groupID,
			Name:      t.Name,
			Color:     intToHexColor(t.Color),
			Icon:      icon,
			Seq:       int64(math.Round(t.Seq * seqScale)),
			CreatedAt: unixSecondsToTime(t.Born),
		})
	}

	// Build folder hierarchy & lookup maps.
	folderHelper := newFolderTreeHelper(rawFolders)
	folderTagsByFolderID := make(map[int64][]string)
	for _, jf := range rawTagJoinFolder {
		if name, ok := tagNameByID[jf.TagID]; ok && name != "" {
			folderTagsByFolderID[jf.FolderID] = append(folderTagsByFolderID[jf.FolderID], name)
		}
	}

	includedFolderIDs := make(map[int64]bool, len(rawFolders))
	domainFolderByID := make(map[int64]*domain.Folder, len(rawFolders))
	domainFolders := make([]*domain.Folder, 0, len(rawFolders))

	for _, f := range rawFolders {
		recycled := folderHelper.isRecycled(f.ID)
		hidden := folderHelper.isHidden(f.ID)
		if !r.opts.IncludeDeleted && recycled {
			continue
		}
		if !r.opts.IncludeHidden && hidden {
			continue
		}
		includedFolderIDs[f.ID] = true

		var parentID string
		if f.Pid > 0 && folderHelper.exists(f.Pid) {
			parentID = strconv.FormatInt(f.Pid, 10)
		}

		df := &domain.Folder{
			ID:          strconv.FormatInt(f.ID, 10),
			ParentID:    parentID,
			Name:        f.Name,
			Description: f.Desc,
			Color:       intToHexColor(f.Color),
			Seq:         int64(math.Round(f.Seq * seqScale)),
			IsRecycle:   recycled,
			IsHidden:    hidden,
			CreatedAt:   unixSecondsToTime(f.Born),
			TagNames:    folderTagsByFolderID[f.ID],
		}
		if f.CoverTid > 0 {
			df.CoverAssetID = strconv.FormatInt(f.CoverTid, 10)
		}
		domainFolderByID[f.ID] = df
		domainFolders = append(domainFolders, df)
	}

	for _, f := range rawFolders {
		if !includedFolderIDs[f.ID] || f.Pid <= 0 {
			continue
		}
		if parent, ok := domainFolderByID[f.Pid]; ok {
			parent.ChildIDs = append(parent.ChildIDs, strconv.FormatInt(f.ID, 10))
		}
	}

	// Index per-file auxiliary metadata tables.
	matByFileID := make(map[int64]bfmodel.Material, len(rawMaterials))
	for _, m := range rawMaterials {
		matByFileID[m.FileID] = m
	}
	udByFileID := make(map[int64]bfmodel.MaterialUserData, len(rawUserdata))
	for _, u := range rawUserdata {
		udByFileID[u.FileID] = u
	}
	vidByFileID := make(map[int64]bfmodel.MaterialVideo, len(rawVideos))
	for _, v := range rawVideos {
		vidByFileID[v.FileID] = v
	}
	imgByFileID := make(map[int64]bfmodel.MaterialImage, len(rawImages))
	for _, img := range rawImages {
		imgByFileID[img.FileID] = img
	}
	audByFileID := make(map[int64]bfmodel.MaterialAudio, len(rawAudios))
	for _, aud := range rawAudios {
		audByFileID[aud.FileID] = aud
	}
	fontByFileID := make(map[int64]bool, len(rawFonts))
	for _, f := range rawFonts {
		fontByFileID[f.FileID] = true
	}
	projectByFileID := make(map[int64]bool, len(rawProjects))
	for _, p := range rawProjects {
		projectByFileID[p.FileID] = true
	}
	recycleByFileID := make(map[int64]bfmodel.Recycle, len(rawRecycles))
	for _, rec := range rawRecycles {
		recycleByFileID[rec.FileID] = rec
	}
	tagIDsByFileID := make(map[int64][]string)
	for _, jf := range rawTagJoinFile {
		tagIDsByFileID[jf.FileID] = append(tagIDsByFileID[jf.FileID], strconv.FormatInt(jf.TagID, 10))
	}

	var libCreatedAt, libModifiedAt time.Time
	domainAssets := make([]*domain.Asset, 0, len(rawFiles))

	for _, f := range rawFiles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		mat, hasMat := matByFileID[f.ID]
		rec, hasRec := recycleByFileID[f.ID]
		isDeleted := f.Pid == -1 || (hasMat && mat.IsRecycle != 0) || hasRec || folderHelper.isRecycled(f.Pid)
		isHidden := f.IsHide != 0 || folderHelper.isHidden(f.Pid)

		if !r.opts.IncludeDeleted && isDeleted {
			continue
		}
		if !r.opts.IncludeHidden && isHidden {
			continue
		}

		baseName, ext := kit.SplitFilename(f.Name)
		sourcePath := r.resolveAssetSourcePath(f, rec, hasRec, isDeleted, folderHelper)

		size := f.FileSize
		if size == 0 {
			if st, statErr := os.Stat(sourcePath); statErr == nil && !st.IsDir() {
				size = st.Size()
			}
		}

		vid, hasVid := vidByFileID[f.ID]
		img, hasImg := imgByFileID[f.ID]
		aud, hasAud := audByFileID[f.ID]
		hasFont := fontByFileID[f.ID]
		hasProject := projectByFileID[f.ID]
		ud := udByFileID[f.ID]

		kind := classifyAssetKind(ext, hasVid, hasImg, hasAud, hasFont, hasProject)

		var durationSec float64
		if hasVid && vid.Duration > 0 {
			durationSec = float64(vid.Duration) / 1000.0
		} else if hasAud && aud.Duration > 0 {
			durationSec = float64(aud.Duration) / 1000.0
		}

		var folderIDs []string
		if f.Pid > 0 && includedFolderIDs[f.Pid] {
			folderIDs = []string{strconv.FormatInt(f.Pid, 10)}
		} else if isDeleted && hasRec && rec.FolderID > 0 && includedFolderIDs[rec.FolderID] {
			folderIDs = []string{strconv.FormatInt(rec.FolderID, 10)}
		}

		createdAt := unixSecondsToTime(f.Born)
		if createdAt.IsZero() {
			createdAt = unixSecondsToTime(f.Ctime)
		}
		modifiedAt := unixSecondsToTime(f.Mtime)
		if modifiedAt.IsZero() {
			modifiedAt = createdAt
		}
		if !createdAt.IsZero() && (libCreatedAt.IsZero() || createdAt.Before(libCreatedAt)) {
			libCreatedAt = createdAt
		}
		if !modifiedAt.IsZero() && (libModifiedAt.IsZero() || modifiedAt.After(libModifiedAt)) {
			libModifiedAt = modifiedAt
		}

		mediaMeta := r.buildMediaMeta(f.ID, img, hasImg, vid, hasVid, aud, hasAud, ud)

		domainAssets = append(domainAssets, &domain.Asset{
			ID:          strconv.FormatInt(f.ID, 10),
			FileName:    f.Name,
			Name:        baseName,
			Ext:         ext,
			SourcePath:  sourcePath,
			Size:        size,
			Hash:        f.Md5,
			Kind:        kind,
			Width:       mat.W,
			Height:      mat.H,
			DurationSec: durationSec,
			Rating:      ud.Score,
			Annotation:  ud.Note,
			OriginURL:   ud.Origin,
			Colors:      parseBillfishColors(mat.Colors),
			FolderIDs:   folderIDs,
			TagIDs:      tagIDsByFileID[f.ID],
			IsHidden:    isHidden,
			IsDeleted:   isDeleted,
			CreatedAt:   createdAt,
			ModifiedAt:  modifiedAt,
			MediaMeta:   mediaMeta,
		})
	}

	return &domain.Library{
		Format:     FormatID,
		Version:    AppVersionForSchema(libRow.Version),
		RootPath:   r.rootPath,
		Name:       readLibraryName(r.rootPath),
		CreatedAt:  libCreatedAt,
		ModifiedAt: libModifiedAt,
		Folders:    domainFolders,
		TagGroups:  domainTagGroups,
		Tags:       domainTags,
		Assets:     domainAssets,
	}, nil
}

func (r *Reader) resolveAssetSourcePath(f bfmodel.File, rec bfmodel.Recycle, hasRec, isDeleted bool, helper *folderTreeHelper) string {
	if isDeleted && (f.Pid == -1 || hasRec) {
		var candidates []string
		if hasRec && rec.RecycleName != "" {
			candidates = append(candidates,
				filepath.Join(r.rootPath, ".bf", ".recycle", rec.RecycleName),
				filepath.Join(r.rootPath, ".bf", ".recycle2", rec.RecycleName),
			)
		}
		if f.Name != "" {
			candidates = append(candidates,
				filepath.Join(r.rootPath, ".bf", ".recycle", f.Name),
				filepath.Join(r.rootPath, ".bf", ".recycle2", f.Name),
			)
		}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				return c
			}
		}
		if len(candidates) > 0 {
			return candidates[0]
		}
	}

	if f.Pid > 0 {
		if rel := helper.relPath(f.Pid); rel != "" {
			return filepath.Join(r.rootPath, rel, f.Name)
		}
	}
	return filepath.Join(r.rootPath, f.Name)
}

func (r *Reader) buildMediaMeta(
	fileID int64,
	img bfmodel.MaterialImage, hasImg bool,
	vid bfmodel.MaterialVideo, hasVid bool,
	aud bfmodel.MaterialAudio, hasAud bool,
	ud bfmodel.MaterialUserData,
) *domain.MediaMeta {
	rotation := ud.Rotation
	if hasImg && img.Rotation != 0 {
		rotation = img.Rotation
	}
	if hasVid && vid.Rotation != 0 {
		rotation = vid.Rotation
	}
	hflip := (hasImg && img.Hflip != 0) || ud.Hflip != 0
	vflip := (hasImg && img.Vflip != 0) || ud.Vflip != 0

	var bitrate int64
	var bpm float64
	if hasAud {
		bitrate = int64(aud.BitRate)
		bpm = float64(aud.Bpm)
	}

	thumbPath := r.findThumbnailPath(fileID)

	if rotation == 0 && !hflip && !vflip && bitrate == 0 && bpm == 0 && thumbPath == "" {
		return nil
	}

	return &domain.MediaMeta{
		Rotation:      rotation,
		HFlip:         hflip,
		VFlip:         vflip,
		Bitrate:       bitrate,
		BPM:           bpm,
		ThumbnailPath: thumbPath,
	}
}

func (r *Reader) findThumbnailPath(fileID int64) string {
	idStr := strconv.FormatInt(fileID, 10)
	dirs := []string{
		fmt.Sprintf("%02x", fileID&0xff),
		fmt.Sprintf("%02d", fileID%100),
	}
	if dirs[0] == dirs[1] {
		dirs = dirs[:1]
	}
	suffixes := []string{".hd.webp", ".small.webp"}
	for _, dir := range dirs {
		for _, suffix := range suffixes {
			candidate := filepath.Join(r.rootPath, ".bf", ".preview", dir, idStr+suffix)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func optionalFind(ctx context.Context, db *gorm.DB, tableName string, dest any, selectCols string) error {
	if !db.WithContext(ctx).Migrator().HasTable(tableName) {
		return nil
	}
	q := db.WithContext(ctx)
	if selectCols != "" {
		q = q.Select(selectCols)
	}
	return q.Find(dest).Error
}

type folderTreeHelper struct {
	byID     map[int64]bfmodel.Folder
	recycled map[int64]bool
	hidden   map[int64]bool
	relPaths map[int64]string
}

func newFolderTreeHelper(folders []bfmodel.Folder) *folderTreeHelper {
	byID := make(map[int64]bfmodel.Folder, len(folders))
	for _, f := range folders {
		byID[f.ID] = f
	}
	return &folderTreeHelper{
		byID:     byID,
		recycled: make(map[int64]bool, len(folders)),
		hidden:   make(map[int64]bool, len(folders)),
		relPaths: make(map[int64]string, len(folders)),
	}
}

func (h *folderTreeHelper) exists(id int64) bool {
	_, ok := h.byID[id]
	return ok
}

func (h *folderTreeHelper) isRecycled(id int64) bool {
	if id <= 0 {
		return false
	}
	if v, ok := h.recycled[id]; ok {
		return v
	}
	visited := make(map[int64]bool)
	cur := id
	for cur > 0 {
		if visited[cur] {
			break
		}
		visited[cur] = true
		f, ok := h.byID[cur]
		if !ok {
			break
		}
		if f.IsRecycle != 0 {
			h.recycled[id] = true
			return true
		}
		cur = f.Pid
	}
	h.recycled[id] = false
	return false
}

func (h *folderTreeHelper) isHidden(id int64) bool {
	if id <= 0 {
		return false
	}
	if v, ok := h.hidden[id]; ok {
		return v
	}
	visited := make(map[int64]bool)
	cur := id
	for cur > 0 {
		if visited[cur] {
			break
		}
		visited[cur] = true
		f, ok := h.byID[cur]
		if !ok {
			break
		}
		if f.Hide != 0 {
			h.hidden[id] = true
			return true
		}
		cur = f.Pid
	}
	h.hidden[id] = false
	return false
}

func (h *folderTreeHelper) relPath(id int64) string {
	if id <= 0 {
		return ""
	}
	if v, ok := h.relPaths[id]; ok {
		return v
	}
	var parts []string
	visited := make(map[int64]bool)
	cur := id
	for cur > 0 {
		if visited[cur] {
			break
		}
		visited[cur] = true
		f, ok := h.byID[cur]
		if !ok {
			break
		}
		if f.Name != "" {
			parts = append(parts, f.Name)
		}
		cur = f.Pid
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	rel := filepath.Join(parts...)
	h.relPaths[id] = rel
	return rel
}

func unixSecondsToTime(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

func intToHexColor(c int) string {
	if c == 0 {
		return ""
	}
	return fmt.Sprintf("#%06X", uint32(c)&0xFFFFFF)
}

func parseBillfishColors(raw string) []domain.PaletteColor {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	entries := strings.Split(raw, "|")
	colors := make([]domain.PaletteColor, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ",", 2)
		if len(parts) != 2 {
			continue
		}
		pct, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil || math.IsNaN(pct) || math.IsInf(pct, 0) {
			continue
		}
		rgbInt, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 32)
		if err != nil {
			continue
		}
		colors = append(colors, domain.PaletteColor{
			R:     uint8((rgbInt >> 16) & 0xFF),
			G:     uint8((rgbInt >> 8) & 0xFF),
			B:     uint8(rgbInt & 0xFF),
			Ratio: pct,
		})
	}
	if len(colors) == 0 {
		return nil
	}
	return colors
}

func classifyAssetKind(ext string, hasVid, hasImg, hasAud, hasFont, hasProject bool) domain.AssetKind {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	switch {
	case hasVid || isVideoExt(ext):
		return domain.KindVideo
	case hasAud || isAudioExt(ext):
		return domain.KindAudio
	case hasFont || isFontExt(ext):
		return domain.KindFont
	case hasProject || isProjectExt(ext):
		return domain.KindProject
	case hasImg || isImageExt(ext):
		return domain.KindImage
	default:
		return domain.KindOther
	}
}

func isImageExt(ext string) bool {
	switch ext {
	case "jpg", "jpeg", "png", "webp", "gif", "heic", "heif", "avif", "jxl",
		"bmp", "tif", "tiff", "svg", "ico", "icns", "raw", "cr2", "cr3",
		"nef", "arw", "dng", "orf", "rw2", "tga", "exr", "hdr":
		return true
	default:
		return false
	}
}

func isVideoExt(ext string) bool {
	switch ext {
	case "mp4", "mov", "mkv", "avi", "webm", "m4v", "wmv", "flv", "ts", "mts", "mpg", "mpeg", "3gp":
		return true
	default:
		return false
	}
}

func isAudioExt(ext string) bool {
	switch ext {
	case "mp3", "wav", "flac", "aac", "ogg", "m4a", "aiff", "aif", "wma", "opus", "alac":
		return true
	default:
		return false
	}
}

func isFontExt(ext string) bool {
	switch ext {
	case "ttf", "otf", "woff", "woff2", "ttc", "eot":
		return true
	default:
		return false
	}
}

func isProjectExt(ext string) bool {
	switch ext {
	case "psd", "psb", "ai", "sketch", "fig", "xd", "afdesign", "afphoto",
		"afpub", "cdr", "eps", "pdf", "blend", "c4d", "prproj", "aep", "indd":
		return true
	default:
		return false
	}
}
