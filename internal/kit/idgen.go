package kit

import (
	"cmp"
	"crypto/rand"
	"os"
	"slices"
	"sync"
	"time"
)

const (
	eagleIDLength = 13
	eagleCharset  = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	eagleCharsetN = byte(len(eagleCharset)) // 36
	eagleMaxValid = 252                     // largest multiple of 36 <= 255 for rejection sampling

	snowflakeEpochMs int64 = 1636629071000 // 2021-11-11T11:11:11Z (Pixcall Snowflake epoch)
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

// AssignSnowflakeIDs assigns unique, positive 64-bit Snowflake IDs for each
// timestamp in times such that ID order matches chronological timestamp order.
// Timestamps at or before Pixcall's Snowflake epoch are ordered in [1000, 1<<22)
// so they remain above reserved root IDs (1, 2) and strictly below all post-epoch IDs.
func AssignSnowflakeIDs(times []time.Time) []int64 {
	if len(times) == 0 {
		return nil
	}

	type indexedTime struct {
		idx int
		ms  int64
	}
	items := make([]indexedTime, len(times))
	for i, t := range times {
		items[i] = indexedTime{idx: i, ms: t.UnixMilli()}
	}
	slices.SortFunc(items, func(a, b indexedTime) int {
		if c := cmp.Compare(a.ms, b.ms); c != 0 {
			return c
		}
		return cmp.Compare(a.idx, b.idx)
	})

	// Flip bit 0 so workerID is disjoint from defaultSnowflake.workerID used by NewSnowflakeID.
	workerID := defaultSnowflake.workerID ^ 1
	const preEpochLimit = int64(1) << (workerBits + seqBits)
	preEpochID := int64(1000)

	ids := make([]int64, len(times))
	var lastMs int64
	var seq int64

	for _, item := range items {
		offsetMs := item.ms - snowflakeEpochMs
		if offsetMs <= 0 && preEpochID < preEpochLimit {
			ids[item.idx] = preEpochID
			preEpochID++
			continue
		}
		if offsetMs < 1 {
			offsetMs = 1
		}
		if offsetMs > lastMs {
			lastMs = offsetMs
			seq = 0
		} else {
			seq++
			if seq > maxSeq {
				lastMs++
				seq = 0
			}
		}
		ids[item.idx] = (lastMs << (workerBits + seqBits)) | (workerID << seqBits) | seq
	}

	return ids
}
