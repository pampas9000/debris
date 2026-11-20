package model

import (
	"database/sql/driver"
	"encoding/json/v2"
	"fmt"
	"strings"
	"time"
)

const pixcallSQLiteTimeLayout = "2006-01-02 15:04:05.000"

var scanTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05-07:00",
	"2006-01-02 15:04:05",
	time.RFC3339Nano,
	time.RFC3339,
}

// Timestamp represents a UTC timestamp stored in Pixcall's SQLite databases
// using the native `YYYY-MM-DD HH:MM:SS.sss` format.
type Timestamp struct {
	time.Time
}

// Value implements database/sql/driver.Valuer.
func (t Timestamp) Value() (driver.Value, error) {
	v := t.Time
	if v.IsZero() {
		v = time.Now().UTC()
	}
	return v.UTC().Format(pixcallSQLiteTimeLayout), nil
}

// Scan implements database/sql.Scanner.
func (t *Timestamp) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		t.Time = time.Time{}
		return nil
	case time.Time:
		t.Time = v.UTC()
		return nil
	case string:
		return t.parseString(v)
	case []byte:
		return t.parseString(string(v))
	default:
		return fmt.Errorf("pixcall: cannot scan %T into Timestamp", src)
	}
}

func (t *Timestamp) parseString(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		t.Time = time.Time{}
		return nil
	}
	for _, layout := range scanTimeLayouts {
		if parsed, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return fmt.Errorf("pixcall: invalid timestamp %q", raw)
}

// MarshalJSON implements json.Marshaler.
func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Time)
}

// UnmarshalJSON implements json.Unmarshaler.
func (t *Timestamp) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &t.Time)
}
