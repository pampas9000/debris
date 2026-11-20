package kit

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const transferBufSize = 128 * 1024

var copyBufPool = sync.Pool{
	New: func() any {
		return make([]byte, transferBufSize)
	},
}

// SplitFilename splits filename into base and normalized lowercase extension
// (without leading dot). It never panics on empty strings, extensionless files,
// or dotfiles such as ".DS_Store".
func SplitFilename(filename string) (base, ext string) {
	if filename == "" {
		return "", ""
	}
	sep := strings.LastIndexAny(filename, "/\\")
	namePart := filename[sep+1:]
	if namePart == "" {
		return filename, ""
	}

	dot := strings.LastIndexByte(namePart, '.')
	// dot <= 0 covers no extension (-1) and dotfiles without a secondary extension (.DS_Store).
	// dot == len(namePart)-1 covers trailing dots like "file.".
	if dot <= 0 || dot == len(namePart)-1 {
		return filename, ""
	}

	return filename[:sep+1+dot], strings.ToLower(namePart[dot+1:])
}

// WriteJSON serializes v as indented JSON and atomically writes it to path,
// creating parent directories as needed and closing file descriptors promptly.
func WriteJSON(path string, v any) error {
	if path == "" {
		return errors.New("kit: empty json output path")
	}

	data, err := json.Marshal(v, jsontext.WithIndent("    "))
	if err != nil {
		return fmt.Errorf("kit: marshal json: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("kit: create directory %q: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".debris-json-*.tmp")
	if err != nil {
		return fmt.Errorf("kit: create temp json file: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("kit: write temp json file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("kit: close temp json file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("kit: rename json file to %q: %w", path, err)
	}
	cleanup = false
	return nil
}

// TransferFile transfers src to dst using mode ("copy", "hardlink", or "auto").
// Empty mode defaults to "auto", which attempts os.Link first and falls back to
// streaming io.CopyBuffer on error. onBytes (when non-nil) is invoked with byte
// counts as data is transferred.
func TransferFile(ctx context.Context, src, dst string, mode string, onBytes func(int64)) (usedMode string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if normalizedMode == "" {
		normalizedMode = "auto"
	}
	switch normalizedMode {
	case "copy", "hardlink", "auto":
	default:
		return "", fmt.Errorf("kit: unsupported transfer mode %q", mode)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("kit: stat source %q: %w", src, err)
	}
	if srcInfo.IsDir() {
		return "", fmt.Errorf("kit: source %q is a directory", src)
	}

	if dstInfo, statErr := os.Stat(dst); statErr == nil {
		if os.SameFile(srcInfo, dstInfo) {
			if normalizedMode == "hardlink" || normalizedMode == "auto" {
				if onBytes != nil && srcInfo.Size() > 0 {
					onBytes(srcInfo.Size())
				}
				return "hardlink", nil
			}
			// In copy mode, verify src and dst are not literally the exact same path before replacing.
			srcAbs, _ := filepath.Abs(src)
			dstAbs, _ := filepath.Abs(dst)
			if srcAbs == dstAbs {
				return "", fmt.Errorf("kit: source and destination are identical (%q)", src)
			}
		}
	}

	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return "", fmt.Errorf("kit: create destination dir %q: %w", dstDir, err)
	}

	if normalizedMode == "hardlink" || normalizedMode == "auto" {
		if linkErr := os.Link(src, dst); linkErr == nil {
			if onBytes != nil && srcInfo.Size() > 0 {
				onBytes(srcInfo.Size())
			}
			return "hardlink", nil
		} else if normalizedMode == "hardlink" {
			return "", fmt.Errorf("kit: hardlink %q -> %q: %w", src, dst, linkErr)
		}
	}

	if err := copyFileStreaming(ctx, src, dst, srcInfo.Mode().Perm(), onBytes); err != nil {
		return "", err
	}
	return "copy", nil
}

func copyFileStreaming(ctx context.Context, src, dst string, perm os.FileMode, onBytes func(int64)) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("kit: open source %q: %w", src, err)
	}
	defer srcFile.Close()

	dstDir := filepath.Dir(dst)
	tmpFile, err := os.CreateTemp(dstDir, ".debris-copy-*.tmp")
	if err != nil {
		return fmt.Errorf("kit: create temp file in %q: %w", dstDir, err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	buf := copyBufPool.Get().([]byte)
	defer copyBufPool.Put(buf)

	r := &ctxReader{ctx: ctx, r: srcFile}
	var w io.Writer = tmpFile
	if onBytes != nil {
		w = &progressWriter{w: tmpFile, onBytes: onBytes}
	}

	if _, err := io.CopyBuffer(w, r, buf); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("kit: close copied file %q: %w", dst, err)
	}

	if perm != 0 {
		_ = os.Chmod(tmpPath, perm)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("kit: finalize copied file %q: %w", dst, err)
	}
	cleanup = false
	return nil
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

type progressWriter struct {
	w       io.Writer
	onBytes func(int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	if n > 0 && p.onBytes != nil {
		p.onBytes(int64(n))
	}
	return n, err
}
