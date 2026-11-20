package pixcall

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// pixcallHashChunkSize is the 4 MiB block size used by Pixcall's file_hasher crate.
const pixcallHashChunkSize = 4 * 1024 * 1024

// pixcallContentHashBytes computes Pixcall's content_hash for an in-memory buffer:
// SHA-256 over the concatenation of 4 MiB chunk SHA-256 digests.
func pixcallContentHashBytes(data []byte) string {
	outer := sha256.New()
	for offset := 0; offset < len(data); offset += pixcallHashChunkSize {
		end := min(offset+pixcallHashChunkSize, len(data))
		chunkSum := sha256.Sum256(data[offset:end])
		_, _ = outer.Write(chunkSum[:])
	}
	return hex.EncodeToString(outer.Sum(nil))
}

// pixcallContentHashFile streams path in 4 MiB chunks and returns Pixcall's 64-char
// lowercase hex content_hash (SHA-256 of chunk SHA-256 digests).
func pixcallContentHashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	outer := sha256.New()
	buf := make([]byte, pixcallHashChunkSize)
	for {
		n, err := io.ReadFull(f, buf)
		if n > 0 {
			chunkSum := sha256.Sum256(buf[:n])
			_, _ = outer.Write(chunkSum[:])
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(outer.Sum(nil)), nil
}
