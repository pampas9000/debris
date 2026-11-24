package kit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSQLiteOpenReadWriteAndReadOnly(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "test.db")

	// OpenReadOnly on missing file must error and must NOT create the file.
	missingPath := filepath.Join(dir, "does-not-exist.db")
	if _, _, err := OpenReadOnly(missingPath); err == nil {
		t.Fatal("expected error when opening nonexistent DB read-only")
	}
	if _, err := os.Stat(missingPath); !os.IsNotExist(err) {
		t.Fatalf("OpenReadOnly created missing file %q", missingPath)
	}

	// OpenReadWrite creates DB and allows writes.
	rwDB, rwCloser, err := OpenReadWrite(dbPath)
	if err != nil {
		t.Fatalf("OpenReadWrite failed: %v", err)
	}
	if err := rwDB.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT)").Error; err != nil {
		_ = rwCloser.Close()
		t.Fatalf("CREATE TABLE failed: %v", err)
	}
	if err := rwDB.Exec("INSERT INTO items (id, name) VALUES (1, 'ok')").Error; err != nil {
		_ = rwCloser.Close()
		t.Fatalf("INSERT failed: %v", err)
	}
	if err := rwCloser.Close(); err != nil {
		t.Fatalf("close rwDB failed: %v", err)
	}

	// OpenReadOnly reads existing data and rejects mutations.
	roDB, roCloser, err := OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly failed: %v", err)
	}
	defer roCloser.Close()

	var name string
	if err := roDB.Raw("SELECT name FROM items WHERE id = 1").Scan(&name).Error; err != nil {
		t.Fatalf("SELECT on roDB failed: %v", err)
	}
	if name != "ok" {
		t.Fatalf("got name %q, want ok", name)
	}

	if err := roDB.Exec("INSERT INTO items (id, name) VALUES (2, 'mutate')").Error; err == nil {
		t.Fatal("expected write on OpenReadOnly DB to fail")
	}
}

func TestSQLiteSupportsFTS5ContentlessDelete(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fts5.db")
	db, closer, err := OpenReadWrite(dbPath)
	if err != nil {
		t.Fatalf("OpenReadWrite failed: %v", err)
	}
	defer closer.Close()

	var version string
	if err := db.Raw("SELECT sqlite_version()").Scan(&version).Error; err != nil {
		t.Fatalf("SELECT sqlite_version() failed: %v", err)
	}

	if err := db.Exec(`CREATE VIRTUAL TABLE t_fts USING fts5(name, body, content='', contentless_delete=1)`).Error; err != nil {
		t.Fatalf("CREATE VIRTUAL TABLE contentless_delete=1 failed (sqlite %s): %v", version, err)
	}
	if err := db.Exec(`INSERT INTO t_fts(rowid, name, body) VALUES (1, 'hello', 'world')`).Error; err != nil {
		t.Fatalf("INSERT into FTS5 failed: %v", err)
	}
	if err := db.Exec(`DELETE FROM t_fts WHERE rowid = 1`).Error; err != nil {
		t.Fatalf("DELETE from contentless_delete FTS5 failed: %v", err)
	}
}
