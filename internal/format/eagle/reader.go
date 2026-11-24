package eagle

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/format/eagle/model"
	"debris/internal/kit"
)

type reader struct {
	rootPath string
	opts     format.ReadOptions
}

func (r *reader) Close() error {
	return nil
}

func (r *reader) Inspect(ctx context.Context) (format.InspectResult, error) {
	scan, err := r.scan(ctx, false)
	if err != nil {
		return format.InspectResult{}, err
	}

	res := format.InspectResult{
		Format:        FormatID,
		Version:       scan.libMeta.ApplicationVersion,
		RootPath:      r.rootPath,
		Name:          libraryNameFromPath(r.rootPath),
		FolderCount:   countModelFolders(scan.libMeta.Folders),
		TagGroupCount: len(scan.libMeta.TagsGroups),
		TagCount:      len(scan.tags),
		AssetCount:    scan.assetCount,
		DeletedCount:  scan.deletedCount,
		TotalBytes:    scan.totalBytes,
		Warnings:      scan.warnings,
	}
	if scan.libMeta.ModificationTime > 0 {
		res.Extra = map[string]any{
			"modificationTime": scan.libMeta.ModificationTime,
		}
	}
	return res, nil
}

func (r *reader) Read(ctx context.Context) (*domain.Library, error) {
	scan, err := r.scan(ctx, true)
	if err != nil {
		return nil, err
	}

	lib := &domain.Library{
		Format:     FormatID,
		Version:    scan.libMeta.ApplicationVersion,
		RootPath:   r.rootPath,
		Name:       libraryNameFromPath(r.rootPath),
		ModifiedAt: msToTime(scan.libMeta.ModificationTime),
		Folders:    scan.folders,
		TagGroups:  scan.tagGroups,
		Tags:       scan.tags,
		Assets:     scan.assets,
	}
	return lib, nil
}

type scanResult struct {
	libMeta      model.LibraryMetadata
	folders      []*domain.Folder
	tagGroups    []*domain.TagGroup
	tags         []*domain.Tag
	assets       []*domain.Asset
	assetCount   int
	deletedCount int
	totalBytes   int64
	warnings     []string
}

func (r *reader) scan(ctx context.Context, buildDomain bool) (*scanResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	metaPath := filepath.Join(r.rootPath, "metadata.json")
	rawMeta, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("eagle: read root metadata.json: %w", err)
	}

	var libMeta model.LibraryMetadata
	if err := json.Unmarshal(rawMeta, &libMeta); err != nil {
		return nil, fmt.Errorf("eagle: parse root metadata.json: %w", err)
	}

	res := &scanResult{
		libMeta: libMeta,
	}

	tagGroups, tagToGroupID := convertTagGroups(libMeta.TagsGroups)
	if buildDomain {
		res.folders = convertModelFolders(libMeta.Folders, "")
		res.tagGroups = tagGroups
	}

	tagSeen := make(map[string]struct{})
	starredSet := readStarredTags(r.rootPath)
	addTag := func(name string) string {
		name = strings.TrimSpace(name)
		if name == "" {
			return ""
		}
		if _, ok := tagSeen[name]; !ok {
			tagSeen[name] = struct{}{}
			res.tags = append(res.tags, &domain.Tag{
				ID:      name,
				GroupID: tagToGroupID[name],
				Name:    name,
				Starred: starredSet[name],
			})
		}
		return name
	}

	for _, tg := range libMeta.TagsGroups {
		for _, tagName := range tg.Tags {
			addTag(tagName)
		}
	}

	imagesDir := filepath.Join(r.rootPath, "images")
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, fmt.Errorf("eagle: read images directory: %w", err)
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), ".info") {
			continue
		}

		infoDir := filepath.Join(imagesDir, entry.Name())
		itemMetaPath := filepath.Join(infoDir, "metadata.json")
		rawItem, err := os.ReadFile(itemMetaPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				res.warnings = append(res.warnings, fmt.Sprintf("skipping %s: missing metadata.json", entry.Name()))
				continue
			}
			return nil, fmt.Errorf("eagle: read %s: %w", itemMetaPath, err)
		}

		var itemMeta model.ImageMetadata
		if err := json.Unmarshal(rawItem, &itemMeta); err != nil {
			res.warnings = append(res.warnings, fmt.Sprintf("skipping %s: invalid metadata.json: %v", entry.Name(), err))
			continue
		}

		if itemMeta.IsDeleted {
			res.deletedCount++
			if !r.opts.IncludeDeleted {
				continue
			}
		}

		if itemMeta.ID == "" {
			itemMeta.ID = strings.TrimSuffix(entry.Name(), ".info")
		}

		sourcePath, fileName := resolveAssetFile(infoDir, itemMeta)
		if sourcePath == "" {
			res.warnings = append(res.warnings, fmt.Sprintf("missing asset file in %s", entry.Name()))
		}

		size := itemMeta.Size
		if size <= 0 && sourcePath != "" {
			if st, statErr := os.Stat(sourcePath); statErr == nil {
				size = st.Size()
			}
		}

		res.assetCount++
		res.totalBytes += size

		assetTagIDs := make([]string, 0, len(itemMeta.Tags))
		itemTagSeen := make(map[string]struct{}, len(itemMeta.Tags))
		for _, rawTag := range itemMeta.Tags {
			id := addTag(rawTag)
			if id == "" {
				continue
			}
			if _, ok := itemTagSeen[id]; !ok {
				itemTagSeen[id] = struct{}{}
				assetTagIDs = append(assetTagIDs, id)
			}
		}

		if !buildDomain {
			continue
		}

		ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(itemMeta.Ext)), ".")
		if ext == "" && fileName != "" {
			_, ext = kit.SplitFilename(fileName)
		}
		name := itemMeta.Name
		if name == "" && fileName != "" {
			name, _ = kit.SplitFilename(fileName)
		}
		if fileName == "" {
			fileName = buildFileName(name, ext)
		}

		width := itemMeta.Width
		height := itemMeta.Height
		if width == 0 && itemMeta.ResolutionWidth > 0 {
			width = itemMeta.ResolutionWidth
		}
		if height == 0 && itemMeta.ResolutionHeight > 0 {
			height = itemMeta.ResolutionHeight
		}

		modifiedAt := msToTime(itemMeta.Mtime)
		if modifiedAt.IsZero() {
			modifiedAt = msToTime(itemMeta.ModificationTime)
		}
		if modifiedAt.IsZero() {
			modifiedAt = msToTime(itemMeta.LastModified)
		}

		colors := make([]domain.PaletteColor, 0, len(itemMeta.Palettes))
		for _, p := range itemMeta.Palettes {
			colors = append(colors, domain.PaletteColor{
				R:     p.Color[0],
				G:     p.Color[1],
				B:     p.Color[2],
				Ratio: p.Ratio,
			})
		}

		folderIDs := make([]string, 0, len(itemMeta.Folders))
		folderSeen := make(map[string]struct{}, len(itemMeta.Folders))
		for _, fid := range itemMeta.Folders {
			if fid == "" {
				continue
			}
			if _, ok := folderSeen[fid]; !ok {
				folderSeen[fid] = struct{}{}
				folderIDs = append(folderIDs, fid)
			}
		}

		var mediaMeta *domain.MediaMeta
		thumbPath := resolveThumbnailFile(infoDir, itemMeta)
		if thumbPath != "" || itemMeta.NoThumbnail {
			mediaMeta = &domain.MediaMeta{
				ThumbnailPath: thumbPath,
				NoThumbnail:   itemMeta.NoThumbnail,
			}
		}

		res.assets = append(res.assets, &domain.Asset{
			ID:          itemMeta.ID,
			FileName:    fileName,
			Name:        name,
			Ext:         ext,
			SourcePath:  sourcePath,
			Size:        size,
			Kind:        classifyAssetKind(ext),
			Width:       width,
			Height:      height,
			DurationSec: itemMeta.Duration,
			Rating:      itemMeta.Star,
			Annotation:  itemMeta.Annotation,
			OriginURL:   itemMeta.URL,
			Colors:      colors,
			FolderIDs:   folderIDs,
			TagIDs:      assetTagIDs,
			IsDeleted:   itemMeta.IsDeleted,
			CreatedAt:   msToTime(itemMeta.Btime),
			ModifiedAt:  modifiedAt,
			MediaMeta:   mediaMeta,
		})
	}

	return res, nil
}

func convertModelFolders(folders []model.Folder, parentID string) []*domain.Folder {
	if len(folders) == 0 {
		return nil
	}
	out := make([]*domain.Folder, 0, len(folders))
	for i, f := range folders {
		children := convertModelFolders(f.Children, f.ID)
		childIDs := make([]string, 0, len(children))
		for _, c := range children {
			childIDs = append(childIDs, c.ID)
		}

		var tagNames []string
		if len(f.Tags) > 0 {
			tagNames = append([]string(nil), f.Tags...)
		}

		var extra map[string]any
		if f.Password != "" || f.PasswordTips != "" {
			extra = map[string]any{}
			if f.Password != "" {
				extra["password"] = f.Password
			}
			if f.PasswordTips != "" {
				extra["passwordTips"] = f.PasswordTips
			}
		}

		out = append(out, &domain.Folder{
			ID:          f.ID,
			ParentID:    parentID,
			Name:        f.Name,
			Description: f.Description,
			Seq:         int64(i),
			Icon:        f.Icon,
			IconColor:   f.IconColor,
			ModifiedAt:  msToTime(f.ModificationTime),
			Children:    children,
			ChildIDs:    childIDs,
			TagNames:    tagNames,
			Extra:       extra,
		})
	}
	return out
}

func convertTagGroups(groups []model.TagGroup) ([]*domain.TagGroup, map[string]string) {
	tagToGroup := make(map[string]string)
	if len(groups) == 0 {
		return nil, tagToGroup
	}
	out := make([]*domain.TagGroup, 0, len(groups))
	for i, g := range groups {
		id := g.ID
		if id == "" {
			id = g.Name
		}
		out = append(out, &domain.TagGroup{
			ID:       id,
			ParentID: g.ParentID,
			Name:     g.Name,
			Color:    g.Color,
			Ranking:  int64(i),
		})
		for _, tagName := range g.Tags {
			trimmed := strings.TrimSpace(tagName)
			if trimmed != "" {
				if _, exists := tagToGroup[trimmed]; !exists {
					tagToGroup[trimmed] = id
				}
			}
		}
	}
	return out, tagToGroup
}

func resolveAssetFile(infoDir string, meta model.ImageMetadata) (fullPath, fileName string) {
	ext := strings.TrimPrefix(strings.TrimSpace(meta.Ext), ".")
	candidates := make([]string, 0, 3)

	if meta.Name != "" {
		if ext != "" {
			candidates = append(candidates, meta.Name+"."+ext)
			lowerExt := strings.ToLower(ext)
			if lowerExt != ext {
				candidates = append(candidates, meta.Name+"."+lowerExt)
			}
		} else {
			candidates = append(candidates, meta.Name)
		}
	}

	for _, cand := range candidates {
		p := filepath.Join(infoDir, cand)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, cand
		}
	}

	// Fallback: scan infoDir for a non-metadata, non-thumbnail regular file.
	entries, err := os.ReadDir(infoDir)
	if err != nil {
		return "", ""
	}
	var fallback string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "metadata.json" || name == ".DS_Store" || strings.HasSuffix(name, "_thumbnail.png") {
			continue
		}
		if len(candidates) > 0 && strings.EqualFold(name, candidates[0]) {
			return filepath.Join(infoDir, name), name
		}
		if fallback == "" {
			fallback = name
		} else {
			// Multiple ambiguous files; don't guess blindly.
			fallback = ""
			break
		}
	}
	if fallback != "" {
		return filepath.Join(infoDir, fallback), fallback
	}
	return "", ""
}

func readStarredTags(rootPath string) map[string]bool {
	raw, err := os.ReadFile(filepath.Join(rootPath, "tags.json"))
	if err != nil {
		return nil
	}
	var tf model.TagsFile
	if err := json.Unmarshal(raw, &tf); err != nil {
		return nil
	}
	if len(tf.StarredTags) == 0 {
		return nil
	}
	out := make(map[string]bool, len(tf.StarredTags))
	for _, name := range tf.StarredTags {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			out[trimmed] = true
		}
	}
	return out
}

func resolveThumbnailFile(infoDir string, meta model.ImageMetadata) string {
	if meta.Name != "" {
		cand := filepath.Join(infoDir, meta.Name+"_thumbnail.png")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return ""
}

func buildFileName(name, ext string) string {
	ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
	if ext == "" {
		return name
	}
	if name == "" {
		return ""
	}
	return name + "." + ext
}

func msToTime(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

func classifyAssetKind(ext string) domain.AssetKind {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".") {
	case "jpg", "jpeg", "png", "gif", "webp", "bmp", "tiff", "tif",
		"heic", "heif", "avif", "jxl", "svg", "ico", "raw", "cr2",
		"nef", "arw", "dng", "tga", "exr", "hdr":
		return domain.KindImage
	case "mp4", "mov", "m4v", "mkv", "avi", "webm", "flv", "wmv",
		"ts", "mts", "m2ts", "3gp", "ogv":
		return domain.KindVideo
	case "mp3", "wav", "flac", "aac", "ogg", "m4a", "wma", "aiff", "opus":
		return domain.KindAudio
	case "ttf", "otf", "woff", "woff2", "ttc", "eot":
		return domain.KindFont
	case "psd", "psb", "ai", "sketch", "fig", "xd", "eps", "cdr",
		"afdesign", "afphoto", "clip", "kra", "blend", "c4d", "prproj", "aep":
		return domain.KindProject
	default:
		return domain.KindOther
	}
}
