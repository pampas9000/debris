package billfish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
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

const dbInsertBatchSize = 200

// Writer materializes a domain.Library into a Billfish v41 library directory.
type Writer struct {
	targetPath string
	opts       format.WriteOptions

	mu     sync.Mutex
	closer io.Closer
	closed bool
}

func newWriter(ctx context.Context, path string, opts format.WriteOptions) (*Writer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("billfish: empty target library path")
	}

	mode, err := format.ParseCopyMode(string(opts.CopyMode))
	if err != nil {
		return nil, err
	}
	opts.CopyMode = mode

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("billfish: resolve target path %q: %w", path, err)
	}

	return &Writer{
		targetPath: absPath,
		opts:       opts,
	}, nil
}

// Close closes any open database handle held by Writer.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	if w.closer != nil {
		err := w.closer.Close()
		w.closer = nil
		return err
	}
	return nil
}

// Write creates the Billfish directory layout, transfers assets, and populates
// .bf/billfish.db in batched transactions.
func (w *Writer) Write(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lib == nil {
		return errors.New("billfish: nil library")
	}

	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return errors.New("billfish: writer is closed")
	}
	w.mu.Unlock()

	dbPath := billfishDBPath(w.targetPath)
	if !w.opts.DryRun {
		if _, err := os.Stat(dbPath); err == nil {
			if !w.opts.Overwrite {
				return fmt.Errorf("billfish: target database %q already exists (set overwrite=true to replace)", dbPath)
			}
			for _, p := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
				_ = os.Remove(p)
			}
		}
	}

	emitEvent := func(ev format.ProgressEvent) {
		if emit != nil {
			emit(ev)
		}
	}

	nowSec := time.Now().UTC().Unix()

	// 1. Plan tag groups & tags (strictly mapped by ID).
	tagGroupIDMap, tagGroupRows := planTagGroups(lib.TagGroups, nowSec)
	tagIDMap, tagRows, tagJoinGroupRows, tagIDByName := planTags(lib.Tags, tagGroupIDMap, nowSec)

	// 2. Plan folders topologically and compute relative filesystem paths.
	folderPlan := planFolders(lib.Folders, w.opts.IncludeDeleted, w.opts.IncludeHidden, nowSec, tagIDByName)

	if !w.opts.DryRun {
		if err := ensureBillfishDirs(w.targetPath); err != nil {
			return err
		}
		for _, f := range folderPlan.rows {
			if f.IsRecycle != 0 {
				continue
			}
			rel := folderPlan.relPathByNewID[f.ID]
			if rel == "" {
				continue
			}
			if err := os.MkdirAll(filepath.Join(w.targetPath, rel), 0o755); err != nil {
				return fmt.Errorf("billfish: create folder dir %q: %w", rel, err)
			}
		}
	}

	// 3. Filter assets & transfer physical files.
	assets := make([]*domain.Asset, 0, len(lib.Assets))
	for _, a := range lib.Assets {
		if a == nil {
			continue
		}
		if !w.opts.IncludeDeleted && a.IsDeleted {
			continue
		}
		if !w.opts.IncludeHidden && a.IsHidden {
			continue
		}
		assets = append(assets, a)
	}

	emitEvent(format.ProgressEvent{
		Kind:  "stage",
		Stage: "files",
		Total: int64(len(assets)),
	})

	usedNamesByDir := make(map[string]map[string]bool)
	var (
		fileRows        []bfmodel.File
		materialRows    []bfmodel.Material
		userdataRows    []bfmodel.MaterialUserData
		videoRows       []bfmodel.MaterialVideo
		imageRows       []bfmodel.MaterialImage
		audioRows       []bfmodel.MaterialAudio
		fontRows        []bfmodel.MaterialFont
		projectRows     []bfmodel.MaterialProject
		tagJoinFileRows []bfmodel.TagJoinFile
		recycleRows     []bfmodel.Recycle
	)

	var nextFileID int64 = 1
	totalAssets := int64(len(assets))

	for i, asset := range assets {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Determine parent folder ID (Billfish supports a single parent folder per file).
		var folderNewID int64
		for _, fid := range asset.FolderIDs {
			if mapped, ok := folderPlan.idMap[fid]; ok {
				folderNewID = mapped
				break
			}
		}

		isRecycled := asset.IsDeleted || folderPlan.recycledByNewID[folderNewID]
		isHidden := asset.IsHidden || folderPlan.hiddenByNewID[folderNewID]

		rawName := strings.TrimSpace(asset.EffectiveFileName())
		if rawName == "" {
			rawName = sanitizeName(asset.ID, "asset")
		} else {
			rawName = sanitizeName(filepath.Base(rawName), "asset")
		}

		var dstDir string
		if isRecycled {
			dstDir = filepath.Join(w.targetPath, ".bf", ".recycle")
		} else if folderNewID > 0 && folderPlan.relPathByNewID[folderNewID] != "" {
			dstDir = filepath.Join(w.targetPath, folderPlan.relPathByNewID[folderNewID])
		} else {
			dstDir = w.targetPath
		}

		used := usedNamesByDir[dstDir]
		if used == nil {
			used = make(map[string]bool)
			usedNamesByDir[dstDir] = used
		}
		finalName := disambiguateFileName(used, rawName)
		dstPath := filepath.Join(dstDir, finalName)

		srcInfo, statErr := statRegularFile(asset.SourcePath)
		if statErr != nil {
			emitEvent(format.ProgressEvent{
				Kind:    "warning",
				Stage:   "files",
				AssetID: asset.ID,
				Message: fmt.Sprintf("skipping asset %q: %v", asset.ID, statErr),
			})
			emitEvent(format.ProgressEvent{
				Kind:     "progress",
				Stage:    "files",
				Current:  int64(i + 1),
				Total:    totalAssets,
				AssetID:  asset.ID,
				CopyMode: "skip",
			})
			continue
		}

		if w.opts.DryRun {
			emitEvent(format.ProgressEvent{
				Kind:     "progress",
				Stage:    "files",
				Current:  int64(i + 1),
				Total:    totalAssets,
				Bytes:    srcInfo.Size(),
				AssetID:  asset.ID,
				CopyMode: "skip",
			})
			continue
		}

		if w.opts.Overwrite && (w.opts.CopyMode == format.CopyModeHardlink || w.opts.CopyMode == format.CopyModeAuto) {
			if dstInfo, dstErr := os.Stat(dstPath); dstErr == nil && !os.SameFile(srcInfo, dstInfo) {
				_ = os.Remove(dstPath)
			}
		}

		usedMode, err := kit.TransferFile(ctx, asset.SourcePath, dstPath, string(w.opts.CopyMode), nil)
		if err != nil {
			return fmt.Errorf("billfish: transfer asset %q (%q -> %q): %w", asset.ID, asset.SourcePath, dstPath, err)
		}

		emitEvent(format.ProgressEvent{
			Kind:     "progress",
			Stage:    "files",
			Current:  int64(i + 1),
			Total:    totalAssets,
			Bytes:    srcInfo.Size(),
			AssetID:  asset.ID,
			CopyMode: usedMode,
		})

		fileID := nextFileID
		nextFileID++

		born := timeToUnixSeconds(asset.CreatedAt)
		if born == 0 {
			born = timeToUnixSeconds(asset.ModifiedAt)
		}
		if born == 0 {
			born = srcInfo.ModTime().UTC().Unix()
		}
		mtime := timeToUnixSeconds(asset.ModifiedAt)
		if mtime == 0 {
			mtime = born
		}

		fileSize := srcInfo.Size()
		if fileSize == 0 && asset.Size > 0 {
			fileSize = asset.Size
		}

		pid := folderNewID
		if isRecycled && !folderPlan.recycledByNewID[folderNewID] {
			pid = -1
		}

		fileRows = append(fileRows, bfmodel.File{
			ID:       fileID,
			Name:     finalName,
			Pid:      pid,
			IsHide:   boolToInt(isHidden),
			FileSize: fileSize,
			Ctime:    born,
			Mtime:    mtime,
			Md5:      asset.Hash,
			Born:     born,
		})

		var thumbTid int64
		if asset.MediaMeta != nil && asset.MediaMeta.ThumbnailPath != "" {
			if copiedThumb := copyThumbnailIfWebP(ctx, asset.MediaMeta.ThumbnailPath, w.targetPath, fileID); copiedThumb {
				thumbTid = 1
			}
		}

		materialRows = append(materialRows, bfmodel.Material{
			FileID:    fileID,
			Status:    1,
			ThumbTid:  thumbTid,
			W:         asset.Width,
			H:         asset.Height,
			IsRecycle: boolToInt(isRecycled),
			Colors:    formatBillfishColors(asset.Colors),
		})

		var rotation, hflip, vflip int
		var bitrate, bpm int
		if asset.MediaMeta != nil {
			rotation = asset.MediaMeta.Rotation
			hflip = boolToInt(asset.MediaMeta.HFlip)
			vflip = boolToInt(asset.MediaMeta.VFlip)
			bitrate = int(asset.MediaMeta.Bitrate)
			bpm = int(math.Round(asset.MediaMeta.BPM))
		}

		if asset.Annotation != "" || asset.OriginURL != "" || asset.Rating != 0 || rotation != 0 || hflip != 0 || vflip != 0 {
			userdataRows = append(userdataRows, bfmodel.MaterialUserData{
				FileID:   fileID,
				Note:     asset.Annotation,
				Origin:   asset.OriginURL,
				Score:    asset.Rating,
				Rotation: rotation,
				Hflip:    hflip,
				Vflip:    vflip,
			})
		}

		switch asset.Kind {
		case domain.KindVideo:
			videoRows = append(videoRows, bfmodel.MaterialVideo{
				FileID:   fileID,
				Duration: int(math.Round(asset.DurationSec * 1000.0)),
				Rotation: rotation,
			})
		case domain.KindImage:
			imageRows = append(imageRows, bfmodel.MaterialImage{
				FileID:   fileID,
				Rotation: rotation,
				Hflip:    hflip,
				Vflip:    vflip,
			})
		case domain.KindAudio:
			audioRows = append(audioRows, bfmodel.MaterialAudio{
				FileID:   fileID,
				Duration: int(math.Round(asset.DurationSec * 1000.0)),
				BitRate:  bitrate,
				Bpm:      bpm,
			})
		case domain.KindFont:
			fontRows = append(fontRows, bfmodel.MaterialFont{
				FileID: fileID,
			})
		case domain.KindProject:
			projectRows = append(projectRows, bfmodel.MaterialProject{
				FileID: fileID,
			})
		}

		seenTagIDs := make(map[int64]bool, len(asset.TagIDs))
		for _, oldTagID := range asset.TagIDs {
			newTagID, ok := tagIDMap[oldTagID]
			if !ok || seenTagIDs[newTagID] {
				continue
			}
			seenTagIDs[newTagID] = true
			tagJoinFileRows = append(tagJoinFileRows, bfmodel.TagJoinFile{
				FileID: fileID,
				TagID:  newTagID,
				Born:   born,
			})
		}

		if isRecycled {
			recycleRows = append(recycleRows, bfmodel.Recycle{
				FileID:       fileID,
				FolderID:     folderNewID,
				Born:         born,
				OriginalName: rawName,
				RecycleName:  finalName,
			})
		}
	}

	if w.opts.DryRun {
		return nil
	}

	emitEvent(format.ProgressEvent{
		Kind:  "stage",
		Stage: "database",
	})

	libName := strings.TrimSpace(w.opts.LibraryName)
	if libName == "" {
		libName = strings.TrimSpace(lib.Name)
	}
	if libName == "" {
		libName = filepath.Base(w.targetPath)
	}
	if err := writeBillfishUIConfig(w.targetPath, libName); err != nil {
		return err
	}

	db, closer, err := kit.OpenReadWrite(dbPath)
	if err != nil {
		return fmt.Errorf("billfish: open target database: %w", err)
	}
	w.mu.Lock()
	w.closer = closer
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		c := w.closer
		w.closer = nil
		w.mu.Unlock()
		if c != nil {
			_ = c.Close()
		}
	}()

	if err := migrateBillfishSchema(ctx, db); err != nil {
		return fmt.Errorf("billfish: migrate schema: %w", err)
	}

	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		libRow := bfmodel.Library{
			ID:       1,
			Version:  LibraryVersion,
			Platform: billfishPlatform(),
		}
		if err := tx.Create(&libRow).Error; err != nil {
			return fmt.Errorf("insert library row: %w", err)
		}

		if err := insertBatched(tx, folderPlan.rows); err != nil {
			return fmt.Errorf("insert bf_folder: %w", err)
		}
		if err := insertBatched(tx, tagGroupRows); err != nil {
			return fmt.Errorf("insert bf_tag_group: %w", err)
		}
		if err := insertBatched(tx, tagRows); err != nil {
			return fmt.Errorf("insert bf_tag_v2: %w", err)
		}
		if err := insertBatched(tx, tagJoinGroupRows); err != nil {
			return fmt.Errorf("insert bf_tag_join_group: %w", err)
		}
		if err := insertBatched(tx, folderPlan.tagJoinRows); err != nil {
			return fmt.Errorf("insert bf_tag_join_folder: %w", err)
		}
		if err := insertBatched(tx, fileRows); err != nil {
			return fmt.Errorf("insert bf_file: %w", err)
		}
		if err := insertBatched(tx, materialRows); err != nil {
			return fmt.Errorf("insert bf_material_v2: %w", err)
		}
		if err := insertBatched(tx, userdataRows); err != nil {
			return fmt.Errorf("insert bf_material_userdata: %w", err)
		}
		if err := insertBatched(tx, videoRows); err != nil {
			return fmt.Errorf("insert bf_material_video: %w", err)
		}
		if err := insertBatched(tx, imageRows); err != nil {
			return fmt.Errorf("insert bf_material_image: %w", err)
		}
		if err := insertBatched(tx, audioRows); err != nil {
			return fmt.Errorf("insert bf_material_audio: %w", err)
		}
		if err := insertBatched(tx, fontRows); err != nil {
			return fmt.Errorf("insert bf_material_font: %w", err)
		}
		if err := insertBatched(tx, projectRows); err != nil {
			return fmt.Errorf("insert bf_material_project: %w", err)
		}
		if err := insertBatched(tx, tagJoinFileRows); err != nil {
			return fmt.Errorf("insert bf_tag_join_file: %w", err)
		}
		if err := insertBatched(tx, recycleRows); err != nil {
			return fmt.Errorf("insert bf_recycle: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("billfish: write database transaction: %w", err)
	}

	return nil
}

func migrateBillfishSchema(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).AutoMigrate(
		&bfmodel.Library{},
		&bfmodel.Type{},
		&bfmodel.Folder{},
		&bfmodel.File{},
		&bfmodel.Material{},
		&bfmodel.MaterialImage{},
		&bfmodel.MaterialVideo{},
		&bfmodel.VideoUserdata{},
		&bfmodel.MaterialAudio{},
		&bfmodel.MaterialFont{},
		&bfmodel.MaterialProject{},
		&bfmodel.MaterialUserData{},
		&bfmodel.TagGroup{},
		&bfmodel.LegacyTag{},
		&bfmodel.Tag{},
		&bfmodel.HiddenTag{},
		&bfmodel.TagJoinFolder{},
		&bfmodel.TagJoinFile{},
		&bfmodel.HiddenTagJoinFile{},
		&bfmodel.TagJoinGroup{},
		&bfmodel.Recycle{},
	)
}

func insertBatched[T any](tx *gorm.DB, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	return tx.CreateInBatches(rows, dbInsertBatchSize).Error
}

func ensureBillfishDirs(root string) error {
	dirs := []string{
		filepath.Join(root, ".bf"),
		filepath.Join(root, ".bf", ".preview"),
		filepath.Join(root, ".bf", ".recycle"),
		filepath.Join(root, ".bf", ".recycle2"),
		filepath.Join(root, ".bf", ".ui_config"),
		filepath.Join(root, ".bf", ".backup"),
		filepath.Join(root, ".bf", ".temp"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("billfish: create directory %q: %w", d, err)
		}
	}
	return nil
}

func writeBillfishUIConfig(root, libraryName string) error {
	info := libInfoFile{
		CloseLibraryType:    "5",
		CloseLibraryID:      "0",
		Folders:             []any{},
		LibraryName:         libraryName,
		OpenedColorOptimize: true,
		TagIDs:              []any{},
	}
	if err := kit.WriteJSON(filepath.Join(root, ".bf", ".ui_config", "lib_info.json"), info); err != nil {
		return fmt.Errorf("billfish: write lib_info.json: %w", err)
	}

	iniPath := filepath.Join(root, ".bf", ".ui_config", "library.ini")
	iniContent := "[library_info]\nall_res=0\nres=0\nall_tags=0\nsize=0\ncover=\nname=\n"
	if err := os.WriteFile(iniPath, []byte(iniContent), 0o644); err != nil {
		return fmt.Errorf("billfish: write library.ini: %w", err)
	}
	return nil
}

func planTagGroups(groups []*domain.TagGroup, defaultBorn int64) (map[string]int64, []bfmodel.TagGroup) {
	valid := make([]*domain.TagGroup, 0, len(groups))
	byID := make(map[string]*domain.TagGroup, len(groups))
	for _, g := range groups {
		if g == nil || g.ID == "" {
			continue
		}
		valid = append(valid, g)
		byID[g.ID] = g
	}

	ordered := topoSort(valid,
		func(g *domain.TagGroup) string { return g.ID },
		func(g *domain.TagGroup) string { return g.ParentID },
		func(a, b *domain.TagGroup) bool {
			if a.Ranking != b.Ranking {
				return a.Ranking < b.Ranking
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.ID < b.ID
		},
	)

	idMap := make(map[string]int64, len(ordered))
	rows := make([]bfmodel.TagGroup, 0, len(ordered))
	for i, g := range ordered {
		newID := int64(i + 1)
		idMap[g.ID] = newID
		var pid int64
		if g.ParentID != "" {
			if mapped, ok := idMap[g.ParentID]; ok && byID[g.ParentID] != nil {
				pid = mapped
			}
		}
		rows = append(rows, bfmodel.TagGroup{
			ID:    newID,
			Name:  g.Name,
			Color: hexColorToInt(g.Color),
			Born:  defaultBorn,
			Pid:   pid,
		})
	}
	return idMap, rows
}

func planTags(tags []*domain.Tag, groupIDMap map[string]int64, defaultBorn int64) (
	map[string]int64,
	[]bfmodel.Tag,
	[]bfmodel.TagJoinGroup,
	map[string]int64,
) {
	valid := make([]*domain.Tag, 0, len(tags))
	for _, t := range tags {
		if t == nil || t.ID == "" {
			continue
		}
		valid = append(valid, t)
	}

	ordered := topoSort(valid,
		func(t *domain.Tag) string { return t.ID },
		func(t *domain.Tag) string { return t.ParentID },
		func(a, b *domain.Tag) bool {
			if a.Seq != b.Seq {
				return a.Seq < b.Seq
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.ID < b.ID
		},
	)

	idMap := make(map[string]int64, len(ordered))
	idByName := make(map[string]int64, len(ordered))
	rows := make([]bfmodel.Tag, 0, len(ordered))
	var joinGroups []bfmodel.TagJoinGroup
	seenJoinGroup := make(map[[2]int64]bool)

	for i, t := range ordered {
		newID := int64(i + 1)
		idMap[t.ID] = newID
		if _, exists := idByName[t.Name]; !exists {
			idByName[t.Name] = newID
		}

		var pid int64
		if t.ParentID != "" {
			if mapped, ok := idMap[t.ParentID]; ok {
				pid = mapped
			}
		}

		seq := 0.9 + float64(i+1)*0.000001
		if t.Seq > 0 {
			seq = float64(t.Seq) / seqScale
		}

		born := timeToUnixSeconds(t.CreatedAt)
		if born == 0 {
			born = defaultBorn
		}

		icon, _ := strconv.Atoi(t.Icon)

		rows = append(rows, bfmodel.Tag{
			ID:    newID,
			Name:  t.Name,
			Pid:   pid,
			Seq:   seq,
			Icon:  icon,
			Color: hexColorToInt(t.Color),
			Born:  born,
		})

		if t.GroupID != "" {
			if gid, ok := groupIDMap[t.GroupID]; ok {
				pair := [2]int64{gid, newID}
				if !seenJoinGroup[pair] {
					seenJoinGroup[pair] = true
					joinGroups = append(joinGroups, bfmodel.TagJoinGroup{
						Gid:   gid,
						TagID: newID,
						Born:  born,
					})
				}
			}
		}
	}

	return idMap, rows, joinGroups, idByName
}

type folderPlanResult struct {
	idMap           map[string]int64
	rows            []bfmodel.Folder
	relPathByNewID  map[int64]string
	recycledByNewID map[int64]bool
	hiddenByNewID   map[int64]bool
	tagJoinRows     []bfmodel.TagJoinFolder
}

func planFolders(
	folders []*domain.Folder,
	includeDeleted, includeHidden bool,
	defaultBorn int64,
	tagIDByName map[string]int64,
) folderPlanResult {
	res := folderPlanResult{
		idMap:           make(map[string]int64, len(folders)),
		relPathByNewID:  make(map[int64]string, len(folders)),
		recycledByNewID: make(map[int64]bool, len(folders)),
		hiddenByNewID:   make(map[int64]bool, len(folders)),
	}

	valid := make([]*domain.Folder, 0, len(folders))
	for _, f := range folders {
		if f == nil || f.ID == "" {
			continue
		}
		if !includeDeleted && f.IsRecycle {
			continue
		}
		if !includeHidden && f.IsHidden {
			continue
		}
		valid = append(valid, f)
	}

	ordered := topoSort(valid,
		func(f *domain.Folder) string { return f.ID },
		func(f *domain.Folder) string { return f.ParentID },
		func(a, b *domain.Folder) bool {
			if a.Seq != b.Seq {
				return a.Seq < b.Seq
			}
			if a.Name != b.Name {
				return a.Name < b.Name
			}
			return a.ID < b.ID
		},
	)

	siblingNames := make(map[int64]map[string]bool)
	seenFolderTag := make(map[[2]int64]bool)

	for i, f := range ordered {
		newID := int64(i + 1)
		res.idMap[f.ID] = newID

		var pid int64
		if f.ParentID != "" {
			if mapped, ok := res.idMap[f.ParentID]; ok {
				pid = mapped
			}
		}

		recycled := f.IsRecycle || res.recycledByNewID[pid]
		hidden := f.IsHidden || res.hiddenByNewID[pid]
		res.recycledByNewID[newID] = recycled
		res.hiddenByNewID[newID] = hidden

		used := siblingNames[pid]
		if used == nil {
			used = make(map[string]bool)
			siblingNames[pid] = used
		}
		safeName := sanitizeName(f.Name, "Untitled")
		dirName := disambiguateDirName(used, safeName)

		if pid > 0 && res.relPathByNewID[pid] != "" {
			res.relPathByNewID[newID] = filepath.Join(res.relPathByNewID[pid], dirName)
		} else {
			res.relPathByNewID[newID] = dirName
		}

		seq := 0.9 + float64(i+1)*0.000001
		if f.Seq > 0 {
			seq = float64(f.Seq) / seqScale
		}

		born := timeToUnixSeconds(f.CreatedAt)
		if born == 0 {
			born = defaultBorn
		}

		res.rows = append(res.rows, bfmodel.Folder{
			ID:        newID,
			Born:      born,
			Pid:       pid,
			Name:      dirName,
			Desc:      f.Description,
			Hide:      boolToInt(hidden),
			Seq:       seq,
			Color:     hexColorToInt(f.Color),
			IsRecycle: boolToInt(recycled),
		})

		for _, tagName := range f.TagNames {
			if tid, ok := tagIDByName[tagName]; ok {
				pair := [2]int64{newID, tid}
				if !seenFolderTag[pair] {
					seenFolderTag[pair] = true
					res.tagJoinRows = append(res.tagJoinRows, bfmodel.TagJoinFolder{
						FolderID: newID,
						TagID:    tid,
					})
				}
			}
		}
	}

	return res
}

// topoSort orders items so parents always precede children while preserving
// deterministic sibling order and breaking cycles safely.
func topoSort[T any](items []T, getID, getParentID func(T) string, less func(a, b T) bool) []T {
	byID := make(map[string]T, len(items))
	children := make(map[string][]T, len(items))
	var roots []T

	for _, item := range items {
		byID[getID(item)] = item
	}
	for _, item := range items {
		pid := getParentID(item)
		if pid == "" || pid == getID(item) {
			roots = append(roots, item)
			continue
		}
		if _, exists := byID[pid]; !exists {
			roots = append(roots, item)
			continue
		}
		children[pid] = append(children[pid], item)
	}

	sortSlice := func(s []T) {
		sort.SliceStable(s, func(i, j int) bool {
			return less(s[i], s[j])
		})
	}
	sortSlice(roots)

	result := make([]T, 0, len(items))
	visited := make(map[string]bool, len(items))

	var walk func(T)
	walk = func(node T) {
		id := getID(node)
		if visited[id] {
			return
		}
		visited[id] = true
		result = append(result, node)
		kids := children[id]
		if len(kids) > 1 {
			sortSlice(kids)
		}
		for _, child := range kids {
			walk(child)
		}
	}

	for _, r := range roots {
		walk(r)
	}

	// Append any items locked in pure cycles deterministically.
	if len(result) < len(items) {
		var remaining []T
		for _, item := range items {
			if !visited[getID(item)] {
				remaining = append(remaining, item)
			}
		}
		sortSlice(remaining)
		for _, item := range remaining {
			walk(item)
		}
	}

	return result
}

func statRegularFile(path string) (os.FileInfo, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("empty source path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("source path %q is a directory", path)
	}
	return info, nil
}

func copyThumbnailIfWebP(ctx context.Context, srcThumb, targetRoot string, fileID int64) bool {
	if strings.ToLower(filepath.Ext(srcThumb)) != ".webp" {
		return false
	}
	if _, err := statRegularFile(srcThumb); err != nil {
		return false
	}
	dir := fmt.Sprintf("%02d", fileID%100)
	dst := filepath.Join(targetRoot, ".bf", ".preview", dir, strconv.FormatInt(fileID, 10)+".hd.webp")
	_, err := kit.TransferFile(ctx, srcThumb, dst, string(format.CopyModeCopy), nil)
	return err == nil
}

func sanitizeName(name, fallback string) string {
	cleaned := strings.TrimSpace(name)
	cleaned = strings.ReplaceAll(cleaned, "/", "_")
	cleaned = strings.ReplaceAll(cleaned, "\\", "_")
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return fallback
	}
	return cleaned
}

func disambiguateDirName(used map[string]bool, name string) string {
	key := strings.ToLower(name)
	if !used[key] {
		used[key] = true
		return name
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (%d)", name, n)
		candKey := strings.ToLower(candidate)
		if !used[candKey] {
			used[candKey] = true
			return candidate
		}
	}
}

func disambiguateFileName(used map[string]bool, fileName string) string {
	key := strings.ToLower(fileName)
	if !used[key] {
		used[key] = true
		return fileName
	}
	base, ext := kit.SplitFilename(fileName)
	for n := 2; ; n++ {
		var candidate string
		if ext != "" {
			candidate = fmt.Sprintf("%s (%d).%s", base, n, ext)
		} else {
			candidate = fmt.Sprintf("%s (%d)", base, n)
		}
		candKey := strings.ToLower(candidate)
		if !used[candKey] {
			used[candKey] = true
			return candidate
		}
	}
}

func formatBillfishColors(colors []domain.PaletteColor) string {
	if len(colors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(colors))
	for _, c := range colors {
		rgbInt := (uint32(c.R) << 16) | (uint32(c.G) << 8) | uint32(c.B)
		parts = append(parts, fmt.Sprintf("%.2f,%d", c.Ratio, rgbInt))
	}
	return strings.Join(parts, "|")
}

func hexColorToInt(raw string) int {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0
	}
	if strings.HasPrefix(s, "#") || strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = strings.TrimPrefix(s, "#")
		s = strings.TrimPrefix(s, "0x")
		s = strings.TrimPrefix(s, "0X")
		v, err := strconv.ParseInt(s, 16, 64)
		if err != nil {
			return 0
		}
		return int(v)
	}
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	v, err := strconv.ParseInt(s, 16, 64)
	if err != nil {
		return 0
	}
	return int(v)
}

func timeToUnixSeconds(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().Unix()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func billfishPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "mac"
	case "windows":
		return "win"
	default:
		return runtime.GOOS
	}
}
