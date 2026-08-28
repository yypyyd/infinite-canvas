package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObjectBackupFileDoesNotUseObjectKeyAsPath(t *testing.T) {
	first := objectBackupFile("../../outside")
	second := objectBackupFile("C:\\outside")
	for _, value := range []string{first, second} {
		if !strings.HasPrefix(filepath.ToSlash(value), "objects/") {
			t.Fatalf("unexpected object path %q", value)
		}
		if filepath.Base(value) == "outside" {
			t.Fatalf("object key leaked into path %q", value)
		}
	}
}

func TestValidateManifestRejectsUnsafeDatabasePath(t *testing.T) {
	root := t.TempDir()
	manifest := BackupManifest{FormatVersion: backupFormatVersion, Database: DatabaseBackup{Driver: "sqlite", File: "database/../../outside", SHA256: strings.Repeat("a", 64)}}
	if err := validateManifest(root, manifest); err == nil {
		t.Fatal("expected unsafe database path to be rejected")
	}
}

func TestValidateManifestRejectsDuplicateObjects(t *testing.T) {
	root := t.TempDir()
	key := "organizations/org/uploads/file.png"
	object := ObjectBackup{Key: key, File: objectBackupFile(key), SHA256: strings.Repeat("a", 64), QiniuHash: "etag"}
	manifest := BackupManifest{
		FormatVersion: backupFormatVersion,
		Database:      DatabaseBackup{Driver: "sqlite", File: "database/database.db", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		ObjectStorage: &ObjectStorageBackup{Provider: "qiniu-kodo", Bucket: "bucket", Objects: []ObjectBackup{object, object}},
	}
	if err := validateManifest(root, manifest); err == nil {
		t.Fatal("expected duplicate object to be rejected")
	}
}

func TestVerifyBackupFileRejectsChangedContent(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "database", "database.db")
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackupFile(root, "database/database.db", 7, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
