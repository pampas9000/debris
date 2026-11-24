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

const (
	ulidAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
)

type ulidGenerator struct {
	mu     sync.Mutex
	lastMs int64
	lastR  [10]byte
}

var defaultULID = &ulidGenerator{}

// NewULID generates a 26-character uppercase Crockford Base32 ULID for t.
// If t is zero, time.Now() is used. Calls sharing the same millisecond
// increment the random suffix monotonically to preserve ordering.
func NewULID(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	ms := max(t.UnixMilli(), 0)
	return defaultULID.next(ms)
}

func (g *ulidGenerator) next(ms int64) string {
	g.mu.Lock()
	defer g.mu.Unlock()

	var rnd [10]byte
	if ms == g.lastMs {
		rnd = g.lastR
		if !increment80(rnd[:]) {
			ms++
			fillRandomULID(rnd[:])
		}
	} else {
		fillRandomULID(rnd[:])
	}
	if ms > g.lastMs {
		g.lastMs = ms
	}
	g.lastR = rnd
	return encodeULID(uint64(ms), rnd)
}

// AssignULIDs assigns strictly increasing 26-character ULIDs matching the
// chronological order of times (preserving input order on equal timestamps).
func AssignULIDs(times []time.Time) []string {
	if len(times) == 0 {
		return nil
	}
	type indexedTime struct {
		idx int
		ms  int64
	}
	items := make([]indexedTime, len(times))
	nowMs := time.Now().UnixMilli()
	for i, t := range times {
		ms := nowMs
		if !t.IsZero() {
			ms = max(t.UnixMilli(), 0)
		}
		items[i] = indexedTime{idx: i, ms: ms}
	}
	slices.SortFunc(items, func(a, b indexedTime) int {
		if c := cmp.Compare(a.ms, b.ms); c != 0 {
			return c
		}
		return cmp.Compare(a.idx, b.idx)
	})

	ids := make([]string, len(times))
	var lastMs int64 = -1
	var lastR [10]byte
	for _, item := range items {
		ms := item.ms
		var rnd [10]byte
		if ms <= lastMs {
			ms = lastMs
			rnd = lastR
			if !increment80(rnd[:]) {
				ms++
				fillRandomULID(rnd[:])
			}
		} else {
			fillRandomULID(rnd[:])
			// Keep high random bits bounded below 0x80 so subsequent ties never wrap.
			rnd[0] &= 0x7F
		}
		lastMs = ms
		lastR = rnd
		ids[item.idx] = encodeULID(uint64(ms), rnd)
	}
	return ids
}

func fillRandomULID(dst []byte) {
	if _, err := rand.Read(dst); err != nil {
		panic("kit: crypto/rand failed: " + err.Error())
	}
}

func increment80(b []byte) bool {
	for i := len(b) - 1; i >= 0; i-- {
		b[i]++
		if b[i] != 0 {
			return true
		}
	}
	return false
}

func encodeULID(ms uint64, rnd [10]byte) string {
	var raw [16]byte
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	copy(raw[6:], rnd[:])

	var out [26]byte
	// 128 bits -> 26 base32 chars (top 2 bits zero-padded in char 0)
	out[0] = ulidAlphabet[(raw[0]&0xE0)>>5]
	out[1] = ulidAlphabet[raw[0]&0x1F]
	out[2] = ulidAlphabet[(raw[1]&0xF8)>>3]
	out[3] = ulidAlphabet[((raw[1]&0x07)<<2)|((raw[2]&0xC0)>>6)]
	out[4] = ulidAlphabet[(raw[2]&0x3E)>>1]
	out[5] = ulidAlphabet[((raw[2]&0x01)<<4)|((raw[3]&0xF0)>>4)]
	out[6] = ulidAlphabet[((raw[3]&0x0F)<<1)|((raw[4]&0x80)>>7)]
	out[7] = ulidAlphabet[(raw[4]&0x7C)>>2]
	out[8] = ulidAlphabet[((raw[4]&0x03)<<3)|((raw[5]&0xE0)>>5)]
	out[9] = ulidAlphabet[raw[5]&0x1F]

	out[10] = ulidAlphabet[(raw[6]&0xF8)>>3]
	out[11] = ulidAlphabet[((raw[6]&0x07)<<2)|((raw[7]&0xC0)>>6)]
	out[12] = ulidAlphabet[(raw[7]&0x3E)>>1]
	out[13] = ulidAlphabet[((raw[7]&0x01)<<4)|((raw[8]&0xF0)>>4)]
	out[14] = ulidAlphabet[((raw[8]&0x0F)<<1)|((raw[9]&0x80)>>7)]
	out[15] = ulidAlphabet[(raw[9]&0x7C)>>2]
	out[16] = ulidAlphabet[((raw[9]&0x03)<<3)|((raw[10]&0xE0)>>5)]
	out[17] = ulidAlphabet[raw[10]&0x1F]
	out[18] = ulidAlphabet[(raw[11]&0xF8)>>3]
	out[19] = ulidAlphabet[((raw[11]&0x07)<<2)|((raw[12]&0xC0)>>6)]
	out[20] = ulidAlphabet[(raw[12]&0x3E)>>1]
	out[21] = ulidAlphabet[((raw[12]&0x01)<<4)|((raw[13]&0xF0)>>4)]
	out[22] = ulidAlphabet[((raw[13]&0x0F)<<1)|((raw[14]&0x80)>>7)]
	out[23] = ulidAlphabet[(raw[14]&0x7C)>>2]
	out[24] = ulidAlphabet[((raw[14]&0x03)<<3)|((raw[15]&0xE0)>>5)]
	out[25] = ulidAlphabet[raw[15]&0x1F]
	return string(out[:])
}
