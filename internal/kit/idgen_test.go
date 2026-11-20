package kit

import (
	"sync"
	"testing"
)

func isEagleChar(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'Z')
}

func TestNewEagleID_FormatAndPrefix(t *testing.T) {
	cases := []struct {
		name       string
		prefix     string
		wantPrefix string
	}{
		{name: "empty prefix", prefix: "", wantPrefix: ""},
		{name: "single upper", prefix: "M", wantPrefix: "M"},
		{name: "lowercase normalized", prefix: "ab1", wantPrefix: "AB1"},
		{name: "non alnum stripped", prefix: "m-1_a", wantPrefix: "M1A"},
		{name: "exact 13", prefix: "ABCDEFGHIJKLM", wantPrefix: "ABCDEFGHIJKLM"},
		{name: "longer than 13 truncated", prefix: "ABCDEFGHIJKLMNOP", wantPrefix: "ABCDEFGHIJKLM"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := NewEagleID(tc.prefix)
			if len(id) != 13 {
				t.Fatalf("len(%q) = %d, want 13", id, len(id))
			}
			if tc.wantPrefix != "" && id[:len(tc.wantPrefix)] != tc.wantPrefix {
				t.Fatalf("id %q does not start with %q", id, tc.wantPrefix)
			}
			for i := 0; i < len(id); i++ {
				if !isEagleChar(id[i]) {
					t.Fatalf("id %q contains invalid char %q at index %d", id, id[i], i)
				}
			}
		})
	}
}

func TestNewEagleID_ConcurrencyUniqueness(t *testing.T) {
	const workers = 16
	const perWorker = 500

	var wg sync.WaitGroup
	results := make([][]string, workers)
	for w := range workers {
		wg.Go(func() {
			ids := make([]string, perWorker)
			for i := range perWorker {
				ids[i] = NewEagleID("M")
			}
			results[w] = ids
		})
	}
	wg.Wait()

	seen := make(map[string]struct{}, workers*perWorker)
	for _, batch := range results {
		for _, id := range batch {
			if len(id) != 13 || id[0] != 'M' {
				t.Fatalf("invalid Eagle ID %q", id)
			}
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate Eagle ID generated: %q", id)
			}
			seen[id] = struct{}{}
		}
	}
}

func TestNewSnowflakeID_MonotonicAndUniqueUnderConcurrency(t *testing.T) {
	// Sequential strict monotonicity check (exceeding 4096/ms burst limit too).
	prev := int64(0)
	for i := range 6000 {
		id := NewSnowflakeID()
		if id <= 0 {
			t.Fatalf("expected positive snowflake ID, got %d", id)
		}
		if id <= prev {
			t.Fatalf("snowflake ID not strictly monotonic at %d: prev=%d cur=%d", i, prev, id)
		}
		prev = id
	}

	const workers = 16
	const perWorker = 1000
	var wg sync.WaitGroup
	results := make([][]int64, workers)
	for w := range workers {
		wg.Go(func() {
			ids := make([]int64, perWorker)
			for i := range perWorker {
				ids[i] = NewSnowflakeID()
			}
			results[w] = ids
		})
	}
	wg.Wait()

	seen := make(map[int64]struct{}, workers*perWorker)
	for _, batch := range results {
		for _, id := range batch {
			if id <= 0 {
				t.Fatalf("non-positive ID: %d", id)
			}
			if _, dup := seen[id]; dup {
				t.Fatalf("duplicate snowflake ID generated: %d", id)
			}
			seen[id] = struct{}{}
		}
	}
}
