package eagle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/eagle/model"
	"debris/internal/kit"
)

type writer struct {
	rootPath string
	opts     format.WriteOptions
}

func (w *writer) Close() error {
	return nil
}

func (w *writer) Write(ctx context.Context, lib *domain.Library, emit format.ProgressCallback) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if lib == nil {
		return errors.New("eagle: library must not be nil")
	}

	nowMs := time.Now().UnixMilli()
	usedIDs := make(map[string]struct{})

	emitEvent(emit, format.ProgressEvent{
		Kind:    "stage",
		Stage:   "prepare",
		Message: "preparing eagle metadata",
	})

	eagleFolders, folderIDMap := buildEagleFolders(lib.Folders, usedIDs, nowMs)
	tagNameByID, historyTags, historySeen, tagsGroups := buildEagleTags(lib, usedIDs)

	addHistoryTag := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := historySeen[name]; !ok {
			historySeen[name] = struct{}{}
			historyTags = append(historyTags, name)
		}
	}
	var addFolderTags func(folders []model.Folder)
	addFolderTags = func(folders []model.Folder) {
		for _, f := range folders {
			for _, tn := range f.Tags {
				addHistoryTag(tn)
			}
			addFolderTags(f.Children)
		}
	}
	addFolderTags(eagleFolders)

	if err := os.MkdirAll(filepath.Join(w.rootPath, "images"), 0o755); err != nil {
		return fmt.Errorf("eagle: create images directory: %w", err)
	}

	totalAssets := int64(len(lib.Assets))
	mtimeIndex := make(map[string]int64, len(lib.Assets)+1)
	var activeCount int64
	emitEvent(emit, format.ProgressEvent{
		Kind:    "stage",
		Stage:   "assets",
		Total:   totalAssets,
		Message: "transferring assets",
	})

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

		eagleID := newUniqueEagleID(usedIDs)
		baseName, ext := resolveWriteNameAndExt(asset, eagleID)
		targetFileName := buildFileName(baseName, ext)

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

		fileSize := asset.Size
		if fileSize <= 0 {
			fileSize = srcInfo.Size()
		}

		assetTags := make([]string, 0, len(asset.TagIDs))
		assetTagSeen := make(map[string]struct{}, len(asset.TagIDs))
		for _, tid := range asset.TagIDs {
			name, ok := tagNameByID[tid]
			if !ok || name == "" {
				continue
			}
			if _, dup := assetTagSeen[name]; !dup {
				assetTagSeen[name] = struct{}{}
				assetTags = append(assetTags, name)
				addHistoryTag(name)
			}
		}

		assetFolders := make([]string, 0, len(asset.FolderIDs))
		assetFolderSeen := make(map[string]struct{}, len(asset.FolderIDs))
		for _, fid := range asset.FolderIDs {
			eagleFID, ok := folderIDMap[fid]
			if !ok || eagleFID == "" {
				continue
			}
			if _, dup := assetFolderSeen[eagleFID]; !dup {
				assetFolderSeen[eagleFID] = struct{}{}
				assetFolders = append(assetFolders, eagleFID)
			}
		}

		infoDir := filepath.Join(w.rootPath, "images", eagleID+".info")
		if err := os.MkdirAll(infoDir, 0o755); err != nil {
			return fmt.Errorf("eagle: create item dir %q: %w", infoDir, err)
		}

		dstAssetPath := filepath.Join(infoDir, targetFileName)
		usedMode, err := kit.TransferFile(ctx, asset.SourcePath, dstAssetPath, string(w.opts.CopyMode), nil)
		if err != nil {
			return fmt.Errorf("eagle: transfer asset %q: %w", asset.ID, err)
		}

		if asset.MediaMeta != nil && strings.TrimSpace(asset.MediaMeta.ThumbnailPath) != "" {
			if thumbInfo, err := os.Stat(asset.MediaMeta.ThumbnailPath); err == nil && !thumbInfo.IsDir() {
				thumbDst := filepath.Join(infoDir, baseName+"_thumbnail.png")
				if _, thumbErr := kit.TransferFile(ctx, asset.MediaMeta.ThumbnailPath, thumbDst, string(w.opts.CopyMode), nil); thumbErr != nil {
					if ctx.Err() != nil {
						return thumbErr
					}
					emitEvent(emit, format.ProgressEvent{
						Kind:    "warning",
						Stage:   "assets",
						AssetID: asset.ID,
						Message: fmt.Sprintf("failed to copy thumbnail for %q: %v", asset.ID, thumbErr),
					})
				}
			}
		}

		btime := timeToMs(asset.CreatedAt)
		mtime := timeToMs(asset.ModifiedAt)
		if btime == 0 {
			if mtime > 0 {
				btime = mtime
			} else {
				btime = nowMs
			}
		}
		if mtime == 0 {
			mtime = btime
		}

		palettes := make([]model.Palette, 0, len(asset.Colors))
		for _, c := range asset.Colors {
			palettes = append(palettes, model.Palette{
				Color: [3]uint8{c.R, c.G, c.B},
				Ratio: c.Ratio,
			})
		}

		resWidth, resHeight := 0, 0
		if (asset.Kind == domain.KindVideo || asset.DurationSec > 0) && asset.Width > 0 && asset.Height > 0 {
			resWidth = asset.Width
			resHeight = asset.Height
		}

		noThumb := asset.MediaMeta != nil && asset.MediaMeta.NoThumbnail

		itemMeta := model.ImageMetadata{
			ID:               eagleID,
			Name:             baseName,
			Size:             fileSize,
			Btime:            btime,
			Mtime:            mtime,
			Ext:              ext,
			Tags:             assetTags,
			Folders:          assetFolders,
			IsDeleted:        asset.IsDeleted,
			URL:              asset.OriginURL,
			Annotation:       asset.Annotation,
			ModificationTime: mtime,
			Width:            asset.Width,
			Height:           asset.Height,
			ResolutionWidth:  resWidth,
			ResolutionHeight: resHeight,
			Duration:         asset.DurationSec,
			NoThumbnail:      noThumb,
			LastModified:     mtime,
			Star:             asset.Rating,
			Palettes:         palettes,
		}

		if err := kit.WriteJSON(filepath.Join(infoDir, "metadata.json"), itemMeta); err != nil {
			return fmt.Errorf("eagle: write item metadata for %q: %w", asset.ID, err)
		}

		mtimeIndex[eagleID] = mtime
		if !asset.IsDeleted {
			activeCount++
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
		Message: "writing root metadata",
	})

	libModTime := timeToMs(lib.ModifiedAt)
	if libModTime == 0 {
		libModTime = nowMs
	}

	libMeta := model.LibraryMetadata{
		Folders:            eagleFolders,
		SmartFolders:       make([]any, 0),
		QuickAccess:        make([]any, 0),
		TagsGroups:         tagsGroups,
		ModificationTime:   libModTime,
		ApplicationVersion: DefaultAppVersion,
	}
	if err := kit.WriteJSON(filepath.Join(w.rootPath, "metadata.json"), libMeta); err != nil {
		return fmt.Errorf("eagle: write root metadata.json: %w", err)
	}

	tagsFile := model.TagsFile{
		HistoryTags: historyTags,
		StarredTags: make([]string, 0),
	}
	if err := kit.WriteJSON(filepath.Join(w.rootPath, "tags.json"), tagsFile); err != nil {
		return fmt.Errorf("eagle: write tags.json: %w", err)
	}

	mtimeIndex["all"] = activeCount
	if err := kit.WriteJSON(filepath.Join(w.rootPath, "mtime.json"), mtimeIndex); err != nil {
		return fmt.Errorf("eagle: write mtime.json: %w", err)
	}

	if err := kit.WriteJSON(filepath.Join(w.rootPath, "saved-filters.json"), []any{}); err != nil {
		return fmt.Errorf("eagle: write saved-filters.json: %w", err)
	}

	if err := kit.WriteJSON(filepath.Join(w.rootPath, "actions.json"), []any{}); err != nil {
		return fmt.Errorf("eagle: write actions.json: %w", err)
	}

	return nil
}

func buildEagleFolders(folders []*domain.Folder, usedIDs map[string]struct{}, nowMs int64) ([]model.Folder, map[string]string) {
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
		idMap[oldID] = newUniqueEagleID(usedIDs)
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
		sort.SliceStable(ids, func(i, j int) bool {
			return byID[ids[i]].Seq < byID[ids[j]].Seq
		})
	}
	sortBySeq(rootIDs)
	for pid, cids := range childrenOf {
		sortBySeq(cids)
		childrenOf[pid] = cids
	}

	visited := make(map[string]bool, len(byID))
	var buildTree func(ids []string) []model.Folder
	buildTree = func(ids []string) []model.Folder {
		out := make([]model.Folder, 0, len(ids))
		for _, oldID := range ids {
			if visited[oldID] {
				continue
			}
			visited[oldID] = true
			src := byID[oldID]

			modTime := timeToMs(src.ModifiedAt)
			if modTime == 0 {
				modTime = timeToMs(src.CreatedAt)
			}
			if modTime == 0 {
				modTime = nowMs
			}

			tags := make([]string, 0, len(src.TagNames))
			for _, tn := range src.TagNames {
				if trimmed := strings.TrimSpace(tn); trimmed != "" {
					tags = append(tags, trimmed)
				}
			}

			var password, passwordTips string
			if src.Extra != nil {
				if p, ok := src.Extra["password"].(string); ok {
					password = p
				}
				if pt, ok := src.Extra["passwordTips"].(string); ok {
					passwordTips = pt
				}
			}

			out = append(out, model.Folder{
				ID:               idMap[oldID],
				Name:             src.Name,
				Description:      src.Description,
				Children:         buildTree(childrenOf[oldID]),
				ModificationTime: modTime,
				Tags:             tags,
				Password:         password,
				PasswordTips:     passwordTips,
				Icon:             src.Icon,
				IconColor:        src.IconColor,
			})
		}
		return out
	}

	eagleFolders := buildTree(rootIDs)
	// Handle any disconnected cycle nodes gracefully as root folders.
	if len(visited) < len(orderedIDs) {
		var remaining []string
		for _, oldID := range orderedIDs {
			if !visited[oldID] {
				remaining = append(remaining, oldID)
			}
		}
		eagleFolders = append(eagleFolders, buildTree(remaining)...)
	}

	return eagleFolders, idMap
}

func buildEagleTags(lib *domain.Library, usedIDs map[string]struct{}) (
	tagNameByID map[string]string,
	historyTags []string,
	historySeen map[string]struct{},
	tagsGroups []model.TagGroup,
) {
	tagNameByID = make(map[string]string, len(lib.Tags))
	historyTags = make([]string, 0, len(lib.Tags))
	historySeen = make(map[string]struct{}, len(lib.Tags))
	tagsByGroup := make(map[string][]string)

	for _, t := range lib.Tags {
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
		if t.ID != "" {
			tagNameByID[t.ID] = name
		}
		if _, ok := historySeen[name]; !ok {
			historySeen[name] = struct{}{}
			historyTags = append(historyTags, name)
		}
		if t.GroupID != "" {
			tagsByGroup[t.GroupID] = append(tagsByGroup[t.GroupID], name)
		}
	}

	tagsGroups = make([]model.TagGroup, 0, len(lib.TagGroups))
	groupIDMap := make(map[string]string, len(lib.TagGroups))
	for _, g := range lib.TagGroups {
		if g == nil {
			continue
		}
		groupIDMap[g.ID] = newUniqueEagleID(usedIDs)
	}
	for _, g := range lib.TagGroups {
		if g == nil {
			continue
		}
		rawNames := tagsByGroup[g.ID]
		deduped := make([]string, 0, len(rawNames))
		seen := make(map[string]struct{}, len(rawNames))
		for _, n := range rawNames {
			if _, ok := seen[n]; !ok {
				seen[n] = struct{}{}
				deduped = append(deduped, n)
			}
		}
		tagsGroups = append(tagsGroups, model.TagGroup{
			ID:       groupIDMap[g.ID],
			Name:     g.Name,
			Tags:     deduped,
			Color:    g.Color,
			ParentID: groupIDMap[g.ParentID],
		})
	}

	return tagNameByID, historyTags, historySeen, tagsGroups
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

	baseName = sanitizeFilenamePart(baseName)
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

func newUniqueEagleID(used map[string]struct{}) string {
	for {
		id := kit.NewEagleID("M")
		if _, exists := used[id]; !exists {
			used[id] = struct{}{}
			return id
		}
	}
}

func timeToMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

func emitEvent(emit format.ProgressCallback, ev format.ProgressEvent) {
	if emit != nil {
		emit(ev)
	}
}
