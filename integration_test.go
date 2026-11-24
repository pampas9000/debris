package main_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"debris/cmd"
	"debris/internal/domain"
	"debris/internal/engine"
	"debris/internal/format"
	"debris/internal/format/billfish"
	"debris/internal/format/eagle"
	"debris/internal/format/pixcall"
	pixcallmodel "debris/internal/format/pixcall/model"
	"debris/internal/format/refto"
	"debris/internal/kit"
	"debris/pkg/debris"
)

func newIntegrationEngine() (*debris.Engine, map[string]format.Adapter) {
	adapters := []format.Adapter{
		billfish.New(),
		eagle.New(),
		pixcall.New(),
		refto.New(),
	}
	byID := make(map[string]format.Adapter, len(adapters))
	for _, a := range adapters {
		byID[a.ID()] = a
	}
	reg := engine.NewRegistry(adapters...)
	runner := engine.NewRunner(reg)
	return debris.NewEngine(debris.WithRunner(runner)), byID
}

func sampleLibraryPath(t *testing.T, name string) string {
	t.Helper()
	candidates := []string{
		filepath.Join("/Users/kazuha/dev/debris/resources/sample_libraries", name),
		filepath.Join("resources", "sample_libraries", name),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	t.Skipf("sample library %q not found in %v", name, candidates)
	return ""
}

func copySampleLibrary(t *testing.T, name string) string {
	t.Helper()
	src := sampleLibraryPath(t, name)
	dst := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
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
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()

		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatalf("copySampleLibrary(%s): %v", name, err)
	}
	return dst
}

func TestIntegration_ProbeAndInspectAllSamples(t *testing.T) {
	eng, _ := newIntegrationEngine()
	ctx := context.Background()

	specs := eng.Formats()
	if len(specs) != 4 {
		t.Fatalf("expected 4 registered formats, got %d", len(specs))
	}

	cases := []struct {
		dir           string
		wantFormat    string
		wantFolders   int
		wantTagGroups int
		wantTags      int
		wantAssets    int
	}{
		{
			dir:           "billfish.library",
			wantFormat:    "billfish",
			wantFolders:   2,
			wantTagGroups: 1,
			wantTags:      5,
			wantAssets:    4,
		},
		{
			dir:           "eagle.library",
			wantFormat:    "eagle",
			wantFolders:   2,
			wantTagGroups: 0,
			wantTags:      9,
			wantAssets:    5,
		},
		{
			dir:           "pixcall.library",
			wantFormat:    "pixcall",
			wantFolders:   0,
			wantTagGroups: 2,
			wantTags:      3,
			wantAssets:    6,
		},
	}

	for _, tc := range cases {
		t.Run(tc.wantFormat, func(t *testing.T) {
			libPath := copySampleLibrary(t, tc.dir)

			probeResp, err := eng.Probe(ctx, debris.ProbeRequest{Path: libPath})
			if err != nil {
				t.Fatalf("auto Probe(%s) failed: %v", tc.dir, err)
			}
			if !probeResp.Matched || probeResp.Format != tc.wantFormat {
				t.Fatalf("auto Probe(%s) = %+v, want matched format %q", tc.dir, probeResp, tc.wantFormat)
			}

			inspectResp, err := eng.Inspect(ctx, debris.InspectRequest{Path: libPath})
			if err != nil {
				t.Fatalf("auto Inspect(%s) failed: %v", tc.dir, err)
			}
			if inspectResp.Format != tc.wantFormat ||
				inspectResp.FolderCount != tc.wantFolders ||
				inspectResp.TagGroupCount != tc.wantTagGroups ||
				inspectResp.TagCount != tc.wantTags ||
				inspectResp.AssetCount != tc.wantAssets ||
				inspectResp.TotalBytes <= 0 {
				t.Fatalf("unexpected Inspect(%s): %+v", tc.dir, inspectResp)
			}
		})
	}
}

func TestIntegration_PlanAcrossPairs(t *testing.T) {
	eng, _ := newIntegrationEngine()
	ctx := context.Background()

	samples := map[string]string{
		"billfish": copySampleLibrary(t, "billfish.library"),
		"eagle":    copySampleLibrary(t, "eagle.library"),
		"pixcall":  copySampleLibrary(t, "pixcall.library"),
	}

	pairs := []struct {
		from       string
		to         string
		wantAssets int
	}{
		{from: "billfish", to: "eagle", wantAssets: 4},
		{from: "billfish", to: "pixcall", wantAssets: 4},
		{from: "eagle", to: "billfish", wantAssets: 5},
		{from: "eagle", to: "pixcall", wantAssets: 5},
		{from: "pixcall", to: "billfish", wantAssets: 6},
		{from: "pixcall", to: "eagle", wantAssets: 6},
	}

	for _, p := range pairs {
		t.Run(p.from+"->"+p.to, func(t *testing.T) {
			outPath := filepath.Join(t.TempDir(), "planned_"+p.to)
			planResp, err := eng.Plan(ctx, debris.PlanRequest{
				To:         p.to,
				InputPath:  samples[p.from],
				OutputPath: outPath,
				CopyMode:   debris.CopyModeAuto,
			})
			if err != nil {
				t.Fatalf("Plan(%s -> %s) failed: %v", p.from, p.to, err)
			}
			if planResp.SourceFormat != p.from || planResp.TargetFormat != p.to {
				t.Fatalf("unexpected formats: %+v", planResp)
			}
			if planResp.AssetCount != p.wantAssets || planResp.TotalBytes <= 0 {
				t.Fatalf("unexpected plan counts: %+v", planResp)
			}
			if _, err := os.Stat(outPath); !os.IsNotExist(err) {
				t.Fatalf("Plan created output path %q unexpectedly", outPath)
			}
		})
	}
}

func TestIntegration_AllSixConversions(t *testing.T) {
	eng, adapters := newIntegrationEngine()
	ctx := context.Background()

	type checkAsset struct {
		baseName   string
		rating     int
		annotation string
		originURL  string
		minTags    int
	}

	cases := []struct {
		fromDir     string
		from        string
		to          string
		wantAssets  int
		wantFolders int
		minTags     int
		checkAssets []checkAsset
	}{
		{
			fromDir:     "billfish.library",
			from:        "billfish",
			to:          "eagle",
			wantAssets:  4,
			wantFolders: 2,
			minTags:     4,
			checkAssets: []checkAsset{
				{
					baseName:   "IMG_6480 Medium",
					rating:     5,
					annotation: "兔子",
					originURL:  "https://www.rabbit.com",
					minTags:    1,
				},
			},
		},
		{
			fromDir:     "billfish.library",
			from:        "billfish",
			to:          "pixcall",
			wantAssets:  4,
			wantFolders: 2,
			minTags:     4,
			checkAssets: []checkAsset{
				{
					baseName:   "IMG_6480 Medium",
					rating:     5,
					annotation: "兔子",
					originURL:  "https://www.rabbit.com",
					minTags:    1,
				},
			},
		},
		{
			fromDir:     "eagle.library",
			from:        "eagle",
			to:          "billfish",
			wantAssets:  5,
			wantFolders: 2,
			minTags:     9,
			checkAssets: []checkAsset{
				{
					baseName:   "IMG_6480 Medium",
					rating:     5,
					annotation: "兔子",
					originURL:  "https://www.rabbit.com",
					minTags:    1,
				},
			},
		},
		{
			fromDir:     "eagle.library",
			from:        "eagle",
			to:          "pixcall",
			wantAssets:  5,
			wantFolders: 2,
			minTags:     9,
			checkAssets: []checkAsset{
				{
					baseName:   "IMG_6480 Medium",
					rating:     5,
					annotation: "兔子",
					originURL:  "https://www.rabbit.com",
					minTags:    1,
				},
			},
		},
		{
			fromDir:     "pixcall.library",
			from:        "pixcall",
			to:          "billfish",
			wantAssets:  6,
			wantFolders: 0,
			minTags:     3,
			checkAssets: []checkAsset{
				{
					baseName:   "小黄人",
					rating:     5,
					annotation: "测试小黄人",
				},
				{
					baseName:  "2_q80",
					rating:    2,
					originURL: "https://www.xiaohongshu.com",
					minTags:   1,
				},
			},
		},
		{
			fromDir:     "pixcall.library",
			from:        "pixcall",
			to:          "eagle",
			wantAssets:  6,
			wantFolders: 0,
			minTags:     3,
			checkAssets: []checkAsset{
				{
					baseName:   "小黄人",
					rating:     5,
					annotation: "测试小黄人",
				},
				{
					baseName:  "2_q80",
					rating:    2,
					originURL: "https://www.xiaohongshu.com",
					minTags:   1,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			srcCopy := copySampleLibrary(t, tc.fromDir)

			// Copy and Hardlink real conversions
			modes := []debris.CopyMode{debris.CopyModeCopy, debris.CopyModeHardlink}
			for _, mode := range modes {
				t.Run(string(mode), func(t *testing.T) {
					outDir := filepath.Join(t.TempDir(), "out_"+tc.to+"_"+string(mode))
					var events []debris.Event
					resp, err := eng.Convert(ctx, debris.ConvertRequest{
						To:          tc.to,
						InputPath:   srcCopy,
						OutputPath:  outDir,
						CopyMode:    mode,
						LibraryName: "Converted_" + tc.from + "_to_" + tc.to,
					}, func(ev debris.Event) {
						events = append(events, ev)
					})
					if err != nil {
						t.Fatalf("Convert(%s -> %s, %s) failed: %v", tc.from, tc.to, mode, err)
					}
					if resp.SourceFormat != tc.from || resp.TargetFormat != tc.to ||
						resp.AssetCount != tc.wantAssets || resp.FolderCount != tc.wantFolders ||
						resp.SkippedAssets != 0 {
						t.Fatalf("unexpected Convert response: %+v", resp)
					}
					switch mode {
					case debris.CopyModeCopy:
						if resp.CopiedAssets != tc.wantAssets || resp.HardlinkedAssets != 0 {
							t.Fatalf("expected %d copied assets, got copied=%d hardlinked=%d",
								tc.wantAssets, resp.CopiedAssets, resp.HardlinkedAssets)
						}
					case debris.CopyModeHardlink:
						if resp.HardlinkedAssets != tc.wantAssets || resp.CopiedAssets != 0 {
							t.Fatalf("expected %d hardlinked assets, got hardlinked=%d copied=%d",
								tc.wantAssets, resp.HardlinkedAssets, resp.CopiedAssets)
						}
					}
					if len(events) == 0 {
						t.Fatal("expected conversion progress events")
					}

					// Auto Probe converted output
					probeResp, err := eng.Probe(ctx, debris.ProbeRequest{Path: outDir})
					if err != nil {
						t.Fatalf("Probe converted output failed: %v", err)
					}
					if !probeResp.Matched || probeResp.Format != tc.to {
						t.Fatalf("converted output Probe = %+v, want format %q", probeResp, tc.to)
					}

					// Auto Inspect converted output
					inspectResp, err := eng.Inspect(ctx, debris.InspectRequest{Path: outDir})
					if err != nil {
						t.Fatalf("Inspect converted output failed: %v", err)
					}
					if inspectResp.Format != tc.to ||
						inspectResp.AssetCount != tc.wantAssets ||
						inspectResp.FolderCount != tc.wantFolders ||
						inspectResp.TagCount < tc.minTags ||
						inspectResp.TotalBytes <= 0 {
						t.Fatalf("unexpected converted Inspect: %+v", inspectResp)
					}

					// Read back converted output via target adapter
					srcReader, err := adapters[tc.from].OpenReader(ctx, srcCopy, format.ReadOptions{})
					if err != nil {
						t.Fatalf("OpenReader source: %v", err)
					}
					srcLib, err := srcReader.Read(ctx)
					_ = srcReader.Close()
					if err != nil {
						t.Fatalf("Read source: %v", err)
					}

					dstReader, err := adapters[tc.to].OpenReader(ctx, outDir, format.ReadOptions{})
					if err != nil {
						t.Fatalf("OpenReader converted output: %v", err)
					}
					dstLib, err := dstReader.Read(ctx)
					_ = dstReader.Close()
					if err != nil {
						t.Fatalf("Read converted output: %v", err)
					}

					if len(dstLib.Assets) != tc.wantAssets {
						t.Fatalf("Read converted assets = %d, want %d", len(dstLib.Assets), tc.wantAssets)
					}
					if countFoldersFlat(dstLib.Folders) != tc.wantFolders {
						t.Fatalf("Read converted folders = %d, want %d", countFoldersFlat(dstLib.Folders), tc.wantFolders)
					}
					if len(dstLib.Tags) < tc.minTags {
						t.Fatalf("Read converted tags = %d, want >= %d", len(dstLib.Tags), tc.minTags)
					}

					srcByBase := indexAssetsByBaseName(srcLib.Assets)
					dstByBase := indexAssetsByBaseName(dstLib.Assets)

					srcFolderNames := folderNameMap(srcLib.Folders)
					dstFolderNames := folderNameMap(dstLib.Folders)
					for _, name := range srcFolderNames {
						if !containsValue(dstFolderNames, name) {
							t.Errorf("missing converted folder %q in %s -> %s", name, tc.from, tc.to)
						}
					}

					srcTagNames := tagNameMap(srcLib.Tags)
					dstTagNames := tagNameMap(dstLib.Tags)

					for _, a := range dstLib.Assets {
						if a.SourcePath == "" {
							t.Fatalf("converted asset %q has empty SourcePath", a.FileName)
						}
						info, err := os.Stat(a.SourcePath)
						if err != nil || info.Size() <= 0 {
							t.Fatalf("converted asset %q physical file invalid at %q: info=%v err=%v", a.FileName, a.SourcePath, info, err)
						}
						if mode == debris.CopyModeHardlink {
							if srcAsset := srcByBase[a.Name]; srcAsset != nil {
								srcInfo, sErr := os.Stat(srcAsset.SourcePath)
								if sErr == nil && !os.SameFile(srcInfo, info) {
									t.Fatalf("expected hardlinked asset %q to share inode with %q", a.SourcePath, srcAsset.SourcePath)
								}
							}
						}
					}

					for baseName, srcAsset := range srcByBase {
						dstAsset := dstByBase[baseName]
						if dstAsset == nil {
							t.Fatalf("missing converted asset %q (have keys: %v)", baseName, mapKeys(dstByBase))
						}
						if len(srcAsset.FolderIDs) > 0 {
							if len(dstAsset.FolderIDs) == 0 {
								t.Errorf("asset %q lost folder assignment in %s -> %s", baseName, tc.from, tc.to)
							} else {
								wantFolder := srcFolderNames[srcAsset.FolderIDs[0]]
								gotFolder := dstFolderNames[dstAsset.FolderIDs[0]]
								if wantFolder != "" && gotFolder != wantFolder {
									t.Errorf("asset %q folder = %q, want %q", baseName, gotFolder, wantFolder)
								}
							}
						}
						gotTags := make(map[string]bool, len(dstAsset.TagIDs))
						for _, tid := range dstAsset.TagIDs {
							if name := dstTagNames[tid]; name != "" {
								gotTags[name] = true
							}
						}
						for _, tid := range srcAsset.TagIDs {
							if wantTag := srcTagNames[tid]; wantTag != "" && !gotTags[wantTag] {
								t.Errorf("asset %q missing tag %q (got tags: %v)", baseName, wantTag, gotTags)
							}
						}
					}

					for _, want := range tc.checkAssets {
						got := dstByBase[want.baseName]
						if got == nil {
							t.Fatalf("missing converted asset %q (have keys: %v)", want.baseName, mapKeys(dstByBase))
						}
						if got.Rating != want.rating {
							t.Errorf("asset %q Rating = %d, want %d", want.baseName, got.Rating, want.rating)
						}
						if want.annotation != "" && got.Annotation != want.annotation {
							t.Errorf("asset %q Annotation = %q, want %q", want.baseName, got.Annotation, want.annotation)
						}
						if want.originURL != "" && got.OriginURL != want.originURL {
							t.Errorf("asset %q OriginURL = %q, want %q", want.baseName, got.OriginURL, want.originURL)
						}
						if len(got.TagIDs) < want.minTags {
							t.Errorf("asset %q TagIDs = %v, want >= %d", want.baseName, got.TagIDs, want.minTags)
						}
					}
				})
			}
		})
	}
}

func TestIntegration_CLIEndToEndWithWiredDefaultEngine(t *testing.T) {
	srcCopy := copySampleLibrary(t, "pixcall.library")
	outDir := filepath.Join(t.TempDir(), "cli_billfish.library")

	// Use nil engine so NewRootCmd uses cmd's wired defaultEngine.
	run := func(args ...string) (string, error) {
		root := cmd.NewRootCmd(nil)
		var outBuf, errBuf bytes.Buffer
		root.SetOut(&outBuf)
		root.SetErr(&errBuf)
		root.SetArgs(args)
		err := root.Execute()
		return outBuf.String(), err
	}

	inspectOut, err := run("inspect", "--path", srcCopy, "--json")
	if err != nil {
		t.Fatalf("CLI inspect failed: %v", err)
	}
	var inspectResp debris.InspectResponse
	if err := json.Unmarshal([]byte(inspectOut), &inspectResp); err != nil {
		t.Fatalf("unmarshal CLI inspect: %v\n%s", err, inspectOut)
	}
	if inspectResp.Format != "pixcall" || inspectResp.AssetCount != 6 {
		t.Fatalf("unexpected CLI inspect: %+v", inspectResp)
	}

	convOut, err := run("convert", "-t", "billfish", "-i", srcCopy, "-o", outDir, "--json")
	if err != nil {
		t.Fatalf("CLI convert failed: %v", err)
	}
	if !strings.Contains(convOut, `"type":"result"`) {
		t.Fatalf("expected NDJSON result line, got:\n%s", convOut)
	}

	outInspect, err := run("inspect", "-p", outDir, "--json")
	if err != nil {
		t.Fatalf("CLI inspect output failed: %v", err)
	}
	var outResp debris.InspectResponse
	if err := json.Unmarshal([]byte(outInspect), &outResp); err != nil {
		t.Fatalf("unmarshal output inspect: %v", err)
	}
	if outResp.Format != "billfish" || outResp.AssetCount != 6 {
		t.Fatalf("unexpected output CLI inspect: %+v", outResp)
	}
}

func countFoldersFlat(folders []*domain.Folder) int {
	total := 0
	for _, f := range folders {
		if f == nil {
			continue
		}
		total++
		total += countFoldersFlat(f.Children)
	}
	return total
}

func indexAssetsByBaseName(assets []*domain.Asset) map[string]*domain.Asset {
	m := make(map[string]*domain.Asset, len(assets))
	for _, a := range assets {
		if a == nil {
			continue
		}
		name := a.Name
		if name == "" {
			name = strings.TrimSuffix(a.FileName, filepath.Ext(a.FileName))
		}
		m[name] = a
	}
	return m
}

func mapKeys(m map[string]*domain.Asset) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func folderNameMap(folders []*domain.Folder) map[string]string {
	m := make(map[string]string)
	var walk func([]*domain.Folder)
	walk = func(list []*domain.Folder) {
		for _, f := range list {
			if f == nil {
				continue
			}
			m[f.ID] = f.Name
			walk(f.Children)
		}
	}
	walk(folders)
	return m
}

func tagNameMap(tags []*domain.Tag) map[string]string {
	m := make(map[string]string, len(tags))
	for _, tg := range tags {
		if tg != nil {
			m[tg.ID] = tg.Name
		}
	}
	return m
}

func containsValue(m map[string]string, want string) bool {
	for _, v := range m {
		if v == want {
			return true
		}
	}
	return false
}

func TestIntegration_ReftoCrossFormatConversions(t *testing.T) {
	eng, adapters := newIntegrationEngine()
	ctx := context.Background()

	type roundTripCase struct {
		name        string
		sampleDir   string
		srcFormat   string
		wantAssets  int
		wantFolders int
		minTags     int
	}

	cases := []roundTripCase{
		{
			name:        "eagle->refto->eagle",
			sampleDir:   "eagle.library",
			srcFormat:   "eagle",
			wantAssets:  5,
			wantFolders: 2,
			minTags:     9,
		},
		{
			name:        "pixcall->refto->pixcall",
			sampleDir:   "pixcall.library",
			srcFormat:   "pixcall",
			wantAssets:  6,
			wantFolders: 0,
			minTags:     3,
		},
		{
			name:        "billfish->refto",
			sampleDir:   "billfish.library",
			srcFormat:   "billfish",
			wantAssets:  4,
			wantFolders: 2,
			minTags:     4,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srcCopy := copySampleLibrary(t, tc.sampleDir)
			for _, mode := range []debris.CopyMode{debris.CopyModeCopy, debris.CopyModeHardlink} {
				t.Run(string(mode), func(t *testing.T) {
					reftoDir := filepath.Join(t.TempDir(), "mid.reflib")
					resp1, err := eng.Convert(ctx, debris.ConvertRequest{
						To:          "refto",
						InputPath:   srcCopy,
						OutputPath:  reftoDir,
						CopyMode:    mode,
						LibraryName: "MidRefto",
					}, nil)
					if err != nil {
						t.Fatalf("Convert(%s -> refto, %s) failed: %v", tc.srcFormat, mode, err)
					}
					if resp1.SourceFormat != tc.srcFormat || resp1.TargetFormat != "refto" ||
						resp1.AssetCount != tc.wantAssets || resp1.FolderCount != tc.wantFolders ||
						resp1.SkippedAssets != 0 {
						t.Fatalf("unexpected Convert->refto response: %+v", resp1)
					}

					probeResp, err := eng.Probe(ctx, debris.ProbeRequest{Path: reftoDir})
					if err != nil || !probeResp.Matched || probeResp.Format != "refto" {
						t.Fatalf("Probe(refto) = (%+v, %v)", probeResp, err)
					}

					inspectResp, err := eng.Inspect(ctx, debris.InspectRequest{Path: reftoDir})
					if err != nil {
						t.Fatalf("Inspect(refto) failed: %v", err)
					}
					if inspectResp.AssetCount != tc.wantAssets || inspectResp.FolderCount != tc.wantFolders || inspectResp.TagCount < tc.minTags {
						t.Fatalf("unexpected Inspect(refto): %+v", inspectResp)
					}

					// If billfish->refto, verify read-back directly; otherwise also convert back to srcFormat.
					reftoReader, err := adapters["refto"].OpenReader(ctx, reftoDir, format.ReadOptions{})
					if err != nil {
						t.Fatal(err)
					}
					reftoLib, err := reftoReader.Read(ctx)
					_ = reftoReader.Close()
					if err != nil {
						t.Fatal(err)
					}
					if len(reftoLib.Assets) != tc.wantAssets {
						t.Fatalf("read refto assets = %d, want %d", len(reftoLib.Assets), tc.wantAssets)
					}

					if tc.srcFormat == "billfish" {
						return
					}

					backDir := filepath.Join(t.TempDir(), "back."+tc.srcFormat)
					resp2, err := eng.Convert(ctx, debris.ConvertRequest{
						To:         tc.srcFormat,
						InputPath:  reftoDir,
						OutputPath: backDir,
						CopyMode:   mode,
					}, nil)
					if err != nil {
						t.Fatalf("Convert(refto -> %s, %s) failed: %v", tc.srcFormat, mode, err)
					}
					if resp2.SourceFormat != "refto" || resp2.TargetFormat != tc.srcFormat ||
						resp2.AssetCount != tc.wantAssets || resp2.FolderCount != tc.wantFolders ||
						resp2.SkippedAssets != 0 {
						t.Fatalf("unexpected Convert(refto->%s) response: %+v", tc.srcFormat, resp2)
					}

					if tc.srcFormat == "pixcall" {
						db, closer, err := kit.OpenReadOnly(filepath.Join(backDir, ".pixcall", "database", "main.db"))
						if err != nil {
							t.Fatalf("open round-trip pixcall main.db: %v", err)
						}
						type mediaEntry struct {
							Name        string
							ContentType string
						}
						var mediaRows []mediaEntry
						err = db.Table("media").
							Select("entries.name AS name, entries.content_type AS content_type").
							Joins("JOIN entries ON entries.id = media.entry_id").
							Find(&mediaRows).Error
						var nonNullIcons int64
						err2 := db.Model(&pixcallmodel.Folders{}).Where("icon IS NOT NULL").Count(&nonNullIcons).Error
						_ = closer.Close()
						if err != nil || err2 != nil {
							t.Fatalf("query round-trip pixcall main.db: err=%v err2=%v", err, err2)
						}
						if nonNullIcons != 0 {
							t.Fatalf("refto->pixcall folders.icon non-null count = %d, want 0", nonNullIcons)
						}
						for _, row := range mediaRows {
							if !strings.HasPrefix(row.ContentType, "video/") && !strings.HasPrefix(row.ContentType, "audio/") {
								t.Fatalf("refto->pixcall unexpectedly wrote media row for non-AV file %+v", row)
							}
						}
					}
				})
			}
		})
	}
}
