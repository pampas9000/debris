package kit

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenReadOnly opens an existing SQLite database in strict read-only mode using
// SQLite URI parameters (?mode=ro) so source library databases are never created
// or mutated. The caller must close the returned io.Closer when finished.
func OpenReadOnly(path string) (*gorm.DB, io.Closer, error) {
	if path == "" {
		return nil, nil, errors.New("kit: empty sqlite database path")
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("kit: stat sqlite database %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("kit: sqlite path %q is a directory", path)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, fmt.Errorf("kit: resolve sqlite path %q: %w", path, err)
	}

	dsn := buildSQLiteURI(absPath, "mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	return openSQLite(dsn, path)
}

// OpenReadWrite opens (or creates) a SQLite database in read-write mode,
// creating parent directories as needed. The caller must close the returned
// io.Closer when finished.
func OpenReadWrite(path string) (*gorm.DB, io.Closer, error) {
	if path == "" {
		return nil, nil, errors.New("kit: empty sqlite database path")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("kit: create sqlite directory %q: %w", dir, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, fmt.Errorf("kit: resolve sqlite path %q: %w", path, err)
	}

	dsn := buildSQLiteURI(absPath, "_pragma=busy_timeout(5000)")
	return openSQLite(dsn, path)
}

func buildSQLiteURI(absPath, rawQuery string) string {
	slashPath := filepath.ToSlash(absPath)
	if !stringsHasLeadingSlash(slashPath) {
		slashPath = "/" + slashPath
	}
	u := url.URL{
		Scheme:   "file",
		Path:     slashPath,
		RawQuery: rawQuery,
	}
	return u.String()
}

func stringsHasLeadingSlash(s string) bool {
	return len(s) > 0 && s[0] == '/'
}

func openSQLite(dsn, displayPath string) (*gorm.DB, io.Closer, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("kit: open sqlite %q: %w", displayPath, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("kit: get sql.DB for %q: %w", displayPath, err)
	}

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("kit: ping sqlite %q: %w", displayPath, err)
	}

	return db, sqlDB, nil
}
