package kit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestSplitFilename(t *testing.T) {
	cases := []struct {
		in       string
		wantBase string
		wantExt  string
	}{
		{in: "", wantBase: "", wantExt: ""},
		{in: "README", wantBase: "README", wantExt: ""},
		{in: ".DS_Store", wantBase: ".DS_Store", wantExt: ""},
		{in: ".gitignore", wantBase: ".gitignore", wantExt: ""},
		{in: ".env.local", wantBase: ".env", wantExt: "local"},
		{in: "photo.PNG", wantBase: "photo", wantExt: "png"},
		{in: "archive.tar.GZ", wantBase: "archive.tar", wantExt: "gz"},
		{in: "file.", wantBase: "file.", wantExt: ""},
		{in: "dir.v1/README", wantBase: "dir.v1/README", wantExt: ""},
		{in: "dir.v1/.DS_Store", wantBase: "dir.v1/.DS_Store", wantExt: ""},
		{in: "dir.v1/sub/Image.HEIC", wantBase: "dir.v1/sub/Image", wantExt: "heic"},
		{in: `C:\dir.v1\photo.JPG`, wantBase: `C:\dir.v1\photo`, wantExt: "jpg"},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			gotBase, gotExt := SplitFilename(tc.in)
			if gotBase != tc.wantBase || gotExt != tc.wantExt {
				t.Fatalf("SplitFilename(%q) = (%q, %q), want (%q, %q)", tc.in, gotBase, gotExt, tc.wantBase, tc.wantExt)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "nested", "metadata.json")
	payload := map[string]any{
		"name":  "test",
		"count": 42,
	}

	if err := WriteJSON(outPath, payload); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	raw, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded["name"] != "test" || decoded["count"] != float64(42) {
		t.Fatalf("unexpected decoded JSON: %#v", decoded)
	}

	if err := WriteJSON("", payload); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestTransferFile_CopyHardlinkAuto(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	content := bytes.Repeat([]byte("debris-data-"), 40000) // ~480 KiB (multi-chunk)
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	t.Run("copy mode", func(t *testing.T) {
		dst := filepath.Join(dir, "sub", "copy.bin")
		var transferred atomic.Int64
		mode, err := TransferFile(context.Background(), src, dst, "copy", func(n int64) {
			transferred.Add(n)
		})
		if err != nil {
			t.Fatalf("TransferFile copy failed: %v", err)
		}
		if mode != "copy" {
			t.Fatalf("usedMode = %q, want copy", mode)
		}
		if transferred.Load() != int64(len(content)) {
			t.Fatalf("transferred = %d, want %d", transferred.Load(), len(content))
		}
		got, err := os.ReadFile(dst)
		if err != nil || !bytes.Equal(got, content) {
			t.Fatalf("copied file content mismatch (err=%v)", err)
		}
		srcInfo, _ := os.Stat(src)
		dstInfo, _ := os.Stat(dst)
		if os.SameFile(srcInfo, dstInfo) {
			t.Fatal("copy mode should create distinct inode, not hardlink")
		}
	})

	t.Run("hardlink mode", func(t *testing.T) {
		dst := filepath.Join(dir, "sub", "link.bin")
		var transferred atomic.Int64
		mode, err := TransferFile(context.Background(), src, dst, "hardlink", func(n int64) {
			transferred.Add(n)
		})
		if err != nil {
			t.Fatalf("TransferFile hardlink failed: %v", err)
		}
		if mode != "hardlink" {
			t.Fatalf("usedMode = %q, want hardlink", mode)
		}
		if transferred.Load() != int64(len(content)) {
			t.Fatalf("transferred = %d, want %d", transferred.Load(), len(content))
		}
		srcInfo, _ := os.Stat(src)
		dstInfo, _ := os.Stat(dst)
		if !os.SameFile(srcInfo, dstInfo) {
			t.Fatal("hardlink mode did not link to same file")
		}
	})

	t.Run("auto mode uses hardlink on same fs and falls back to copy on link error", func(t *testing.T) {
		dst1 := filepath.Join(dir, "auto1.bin")
		mode, err := TransferFile(context.Background(), src, dst1, "auto", nil)
		if err != nil {
			t.Fatalf("auto mode failed: %v", err)
		}
		if mode != "hardlink" {
			t.Fatalf("expected auto mode to use hardlink on same fs, got %q", mode)
		}

		// When dst2 already exists as a distinct file, os.Link fails with EEXIST,
		// so auto mode falls back to copy and replaces it cleanly.
		dst2 := filepath.Join(dir, "auto2.bin")
		if err := os.WriteFile(dst2, []byte("old"), 0o644); err != nil {
			t.Fatalf("WriteFile dst2 failed: %v", err)
		}
		mode2, err := TransferFile(context.Background(), src, dst2, "auto", nil)
		if err != nil {
			t.Fatalf("auto fallback failed: %v", err)
		}
		if mode2 != "copy" {
			t.Fatalf("expected auto mode fallback to copy, got %q", mode2)
		}
		got, _ := os.ReadFile(dst2)
		if !bytes.Equal(got, content) {
			t.Fatal("auto fallback did not write full content")
		}
	})

	t.Run("invalid mode and missing src", func(t *testing.T) {
		if _, err := TransferFile(context.Background(), src, filepath.Join(dir, "bad.bin"), "symlink", nil); err == nil {
			t.Fatal("expected error for invalid mode")
		}
		if _, err := TransferFile(context.Background(), filepath.Join(dir, "missing.bin"), filepath.Join(dir, "out.bin"), "copy", nil); err == nil {
			t.Fatal("expected error for missing source")
		}
	})
}

func TestTransferFile_Cancel(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "big.bin")
	// 1 MiB so multiple 128 KiB chunks are read in copy mode.
	content := bytes.Repeat([]byte("x"), 1024*1024)
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	t.Run("pre-canceled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		dst := filepath.Join(dir, "precancel.bin")
		if _, err := TransferFile(ctx, src, dst, "copy", nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if _, err := os.Stat(dst); !os.IsNotExist(err) {
			t.Fatalf("expected no dst file on pre-canceled transfer, stat err=%v", err)
		}
	})

	t.Run("canceled mid-copy cleans up partial temp file", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		dstDir := filepath.Join(dir, "mid")
		dst := filepath.Join(dstDir, "midcancel.bin")
		_, err := TransferFile(ctx, src, dst, "copy", func(n int64) {
			cancel()
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
		if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
			t.Fatalf("expected no dst file after canceled copy, stat err=%v", statErr)
		}
		entries, _ := os.ReadDir(dstDir)
		if len(entries) != 0 {
			t.Fatalf("expected no leftover temp files in %q, found %d", dstDir, len(entries))
		}
	})
}
