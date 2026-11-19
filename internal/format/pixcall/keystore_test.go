package pixcall

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"debris/internal/domain"
	"debris/internal/format"
	"debris/internal/kit"
)

func TestOpenSampleKeystoreAndRoundTrip(t *testing.T) {
	sampleKS := "/Users/kazuha/dev/debris/resources/sample_libraries/pixcall.library/.pixcall/keystore"
	raw, err := os.ReadFile(sampleKS)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("sample keystore not present at %s", sampleKS)
		}
		t.Fatal(err)
	}

	pt, err := openKeystore(raw)
	if err != nil {
		t.Fatalf("openKeystore(sample) failed: %v", err)
	}

	var parsed keystorePayload
	if err := json.Unmarshal(pt, &parsed); err != nil {
		t.Fatalf("unmarshal sample keystore plaintext: %v (%q)", err, string(pt))
	}
	if !parsed.IsLocal || parsed.LibraryID != "51b27792-e645-419c-992b-bcaf0390f673" || parsed.Status.Status != "ready" {
		t.Fatalf("unexpected sample keystore payload: %+v", parsed)
	}

	sealed, err := buildKeystoreBytes("aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", 1795079509)
	if err != nil {
		t.Fatalf("buildKeystoreBytes error: %v", err)
	}
	if len(sealed) < keystoreTagSize+keystoreNonceSize+10 {
		t.Fatalf("unexpected sealed keystore length: %d", len(sealed))
	}
	opened, err := openKeystore(sealed)
	if err != nil {
		t.Fatalf("openKeystore(sealed) failed: %v", err)
	}
	var roundTrip keystorePayload
	if err := json.Unmarshal(opened, &roundTrip); err != nil {
		t.Fatalf("unmarshal roundTrip keystore: %v", err)
	}
	if !roundTrip.IsLocal || roundTrip.LastActive != 1795079509 ||
		roundTrip.LibraryID != "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee" ||
		roundTrip.Status.Status != "ready" {
		t.Fatalf("unexpected roundTrip payload: %+v", roundTrip)
	}

	// Tampering with ciphertext must fail authentication.
	tampered := append([]byte(nil), sealed...)
	tampered[0] ^= 0xff
	if _, err := openKeystore(tampered); err == nil {
		t.Fatal("expected authentication failure for tampered keystore")
	}
}

func TestWriterGeneratesPixcallCompatibleMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	md5AssetPath := filepath.Join(srcDir, "a.png")
	b3AssetPath := filepath.Join(srcDir, "b.png")
	for _, p := range []string{md5AssetPath, b3AssetPath} {
		if err := os.WriteFile(p, []byte("test:"+filepath.Base(p)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	blake3Hex := strings.Repeat("ab", 32)
	created := time.Date(2026, 5, 1, 18, 22, 28, 123000000, time.UTC)

	lib := &domain.Library{
		Name:       "CompatTest",
		CreatedAt:  created,
		ModifiedAt: created,
		Assets: []*domain.Asset{
			{
				ID:         "1",
				FileName:   "a.png",
				SourcePath: md5AssetPath,
				Hash:       "42348E44802AB47B6F914828CCBFB61E", // Billfish MD5 -> should not be written as content_hash
				Kind:       domain.KindImage,
				CreatedAt:  created,
				ModifiedAt: created,
			},
			{
				ID:         "2",
				FileName:   "b.png",
				SourcePath: b3AssetPath,
				Hash:       strings.ToUpper(blake3Hex), // 64-hex -> normalized lowercase
				Kind:       domain.KindImage,
				CreatedAt:  created,
				ModifiedAt: created,
			},
		},
	}

	outDir := filepath.Join(tmpDir, "out.pixcall")
	adapter := New()
	w, err := adapter.OpenWriter(context.Background(), outDir, format.WriteOptions{CopyMode: format.CopyModeCopy})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(context.Background(), lib, nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// 1. Keystore + keystore.backup + matching .id marker
	ksRaw, err := os.ReadFile(filepath.Join(outDir, ".pixcall", "keystore"))
	if err != nil {
		t.Fatalf("missing keystore: %v", err)
	}
	ksBakRaw, err := os.ReadFile(filepath.Join(outDir, ".pixcall", "keystore.backup"))
	if err != nil {
		t.Fatalf("missing keystore.backup: %v", err)
	}
	if string(ksRaw) != string(ksBakRaw) {
		t.Fatal("keystore and keystore.backup differ")
	}
	ksPlain, err := openKeystore(ksRaw)
	if err != nil {
		t.Fatalf("openKeystore(written) failed: %v", err)
	}
	var ks keystorePayload
	if err := json.Unmarshal(ksPlain, &ks); err != nil {
		t.Fatal(err)
	}
	idMarker := filepath.Join(outDir, ".pixcall", ks.LibraryID+".id")
	if _, err := os.Stat(idMarker); err != nil {
		t.Fatalf("missing matching .id marker %q: %v", idMarker, err)
	}

	// 2. thumbs.db exists and has schema_version = '2'
	thumbsDB, thumbsCloser, err := kit.OpenReadOnly(filepath.Join(outDir, ".pixcall", "database", "thumbs.db"))
	if err != nil {
		t.Fatalf("open thumbs.db: %v", err)
	}
	var thumbsVer string
	if err := thumbsDB.Raw("SELECT v FROM kvs WHERE k = 'schema_version'").Scan(&thumbsVer).Error; err != nil {
		_ = thumbsCloser.Close()
		t.Fatalf("query thumbs.db schema_version: %v", err)
	}
	var thumbTableCount int64
	if err := thumbsDB.Raw("SELECT count(*) FROM thumbnails").Scan(&thumbTableCount).Error; err != nil {
		_ = thumbsCloser.Close()
		t.Fatalf("query thumbnails table: %v", err)
	}
	_ = thumbsCloser.Close()
	if thumbsVer != "2" {
		t.Fatalf("thumbs.db schema_version = %q, want 2", thumbsVer)
	}

	// 3. main.db timestamps, content_hash, and status
	mainDB, mainCloser, err := kit.OpenReadOnly(filepath.Join(outDir, ".pixcall", "database", "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mainCloser.Close()

	type rowCheck struct {
		Name        string
		ContentHash *string
		Status      int64
		CreatedAt   string
		UpdatedAt   string
	}
	var rows []rowCheck
	if err := mainDB.Raw("SELECT name, content_hash, status, CAST(created_at AS TEXT) AS created_at, CAST(updated_at AS TEXT) AS updated_at FROM entries WHERE kind = 1 ORDER BY name ASC").Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d file entries, want 2", len(rows))
	}
	if rows[0].ContentHash != nil {
		t.Fatalf("a.png content_hash = %q, want nil (MD5 must not be written)", *rows[0].ContentHash)
	}
	if rows[1].ContentHash == nil || *rows[1].ContentHash != blake3Hex {
		t.Fatalf("b.png content_hash = %v, want %q", rows[1].ContentHash, blake3Hex)
	}
	for _, r := range rows {
		if r.Status != 0 {
			t.Fatalf("%s status = %d, want 0", r.Name, r.Status)
		}
		if r.CreatedAt != "2026-05-01 18:22:28.123" || r.UpdatedAt != "2026-05-01 18:22:28.123" {
			t.Fatalf("%s timestamps = (%q, %q), want 2026-05-01 18:22:28.123", r.Name, r.CreatedAt, r.UpdatedAt)
		}
	}
}

func TestWriterNormalizesNFDEntryNamesToNFC(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()

	// NFD forms typical of macOS APFS filenames ("ナヒーダ" = タ+\u3099, "フォーダ" = タ+\u3099).
	nfdFileName := "#女の子 ナヒータ\u3099誕生祭2023 - misyune的插画.png"
	nfcFileName := "#女の子 ナヒーダ誕生祭2023 - misyune的插画.png"
	nfdFolderName := "おめて\u3099とう"
	nfcFolderName := "おめでとう"

	srcImg := filepath.Join(srcDir, nfdFileName)
	if err := os.WriteFile(srcImg, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatal(err)
	}

	lib := &domain.Library{
		Name: "NFDTest",
		Folders: []*domain.Folder{
			{ID: "f1", Name: nfdFolderName},
		},
		Assets: []*domain.Asset{
			{
				ID:         "a1",
				Name:       strings.TrimSuffix(nfdFileName, ".png"),
				FileName:   nfdFileName,
				Ext:        "png",
				Kind:       domain.KindImage,
				SourcePath: srcImg,
				FolderIDs:  []string{"f1"},
			},
			{
				ID:         "a2",
				Name:       strings.TrimSuffix(nfcFileName, ".png"),
				FileName:   nfcFileName,
				Ext:        "png",
				Kind:       domain.KindImage,
				SourcePath: srcImg,
				FolderIDs:  []string{"f1"},
			},
		},
	}

	outDir := filepath.Join(t.TempDir(), "nfd.pixcall")
	var adapter Adapter
	w, err := adapter.OpenWriter(ctx, outDir, format.WriteOptions{
		CopyMode:  format.CopyModeCopy,
		Overwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(ctx, lib, nil); err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	mainDB, closer, err := kit.OpenReadOnly(filepath.Join(outDir, ".pixcall", "database", "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	var folderNames []string
	if err := mainDB.Raw("SELECT name FROM entries WHERE kind = 0 AND parent_id > 0 ORDER BY id ASC").Scan(&folderNames).Error; err != nil {
		t.Fatal(err)
	}
	if len(folderNames) != 1 || folderNames[0] != nfcFolderName {
		t.Fatalf("folder names = %q, want [%q]", folderNames, nfcFolderName)
	}

	var fileNames []string
	if err := mainDB.Raw("SELECT name FROM entries WHERE kind = 1 ORDER BY ranking DESC").Scan(&fileNames).Error; err != nil {
		t.Fatal(err)
	}
	wantSecond := strings.TrimSuffix(nfcFileName, ".png") + " (2).png"
	if len(fileNames) != 2 || fileNames[0] != nfcFileName || fileNames[1] != wantSecond {
		t.Fatalf("file names = %q, want [%q, %q]", fileNames, nfcFileName, wantSecond)
	}
}
