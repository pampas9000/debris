package kit

import (
	"crypto/rand"
	"os"
	"sync"
	"time"
)

const (
	eagleIDLength = 13
	eagleCharset  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	eagleCharsetN = byte(len(eagleCharset)) // 36
	eagleMaxValid = 252                     // largest multiple of 36 <= 255 for rejection sampling

	snowflakeEpochMs int64 = 1577836800000 // 2020-01-01T00:00:00Z
	workerBits       uint8 = 10
	seqBits          uint8 = 12
	maxWorkerID      int64 = (1 << workerBits) - 1
	maxSeq           int64 = (1 << seqBits) - 1
)

// NewEagleID generates a concurrency-safe 13-character uppercase alphanumeric
// ID (A-Z0-9) suitable for Eagle items and folders. Any lowercase ASCII letters
// in prefix are uppercased, non-alphanumeric characters are omitted, and the
// remainder is filled using unbiased cryptographic randomness.
func NewEagleID(prefix string) string {
	var buf [eagleIDLength]byte
	n := 0
	for i := 0; i < len(prefix) && n < eagleIDLength; i++ {
		c := prefix[i]
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'Z':
			buf[n] = c
			n++
		case c >= 'a' && c <= 'z':
			buf[n] = c - ('a' - 'A')
			n++
		}
	}

	if n < eagleIDLength {
		fillRandomBase36(buf[n:])
	}
	return string(buf[:])
}

func fillRandomBase36(dst []byte) {
	var rnd [32]byte
	i := 0
	for i < len(dst) {
		if _, err := rand.Read(rnd[:]); err != nil {
			// Fallback should never happen on supported OSes, but keeps total-failure safe.
			panic("kit: crypto/rand failed: " + err.Error())
		}
		for _, b := range rnd {
			if b < eagleMaxValid {
				dst[i] = eagleCharset[b%eagleCharsetN]
				i++
				if i == len(dst) {
					return
				}
			}
		}
	}
}

type snowflakeGenerator struct {
	mu       sync.Mutex
	lastMs   int64
	workerID int64
	seq      int64
}

var defaultSnowflake = newSnowflakeGenerator()

func newSnowflakeGenerator() *snowflakeGenerator {
	var rnd [2]byte
	_, _ = rand.Read(rnd[:])
	worker := (int64(os.Getpid()) ^ (int64(rnd[0])<<8 | int64(rnd[1]))) & maxWorkerID
	return &snowflakeGenerator{
		workerID: worker,
	}
}

// NewSnowflakeID generates a concurrency-safe, strictly monotonic positive
// 64-bit integer ID suitable for Pixcall entries, folders, tags, and boards.
func NewSnowflakeID() int64 {
	return defaultSnowflake.Next()
}

func (g *snowflakeGenerator) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := max(time.Now().UnixMilli()-snowflakeEpochMs, 0)

	if now > g.lastMs {
		g.lastMs = now
		g.seq = 0
	} else {
		g.seq++
		if g.seq > maxSeq {
			g.lastMs++
			g.seq = 0
		}
	}

	return (g.lastMs << (workerBits + seqBits)) | (g.workerID << seqBits) | g.seq
}
