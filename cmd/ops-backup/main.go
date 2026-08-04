package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/config"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := config.Load(); err != nil {
		fail("config_load", err)
	}
	if len(os.Args) < 2 {
		fail("arguments", errors.New("backup or restore subcommand is required"))
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "backup":
		err = runBackupCommand(ctx, os.Args[2:])
	case "restore":
		err = runRestoreCommand(ctx, os.Args[2:])
	default:
		err = errors.New("unknown subcommand")
	}
	if err != nil {
		fail(os.Args[1], err)
	}
}

func runBackupCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("backup", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	output := flags.String("output", "", "backup parent directory")
	skipObjects := flags.Bool("skip-objects", false, "back up database only")
	if err := flags.Parse(args); err != nil || strings.TrimSpace(*output) == "" {
		return errors.New("--output is required")
	}
	root, err := createBackupDirectory(*output)
	if err != nil {
		return err
	}
	completed := false
	defer func() {
		if !completed {
			_ = os.RemoveAll(root)
		}
	}()
	manifest := BackupManifest{FormatVersion: backupFormatVersion, CreatedAt: time.Now().UTC().Format(time.RFC3339), AppVersion: appVersion()}
	if !*skipObjects {
		manifest.ObjectStorage, err = backupObjects(ctx, root)
		if err != nil {
			return err
		}
		slog.Info("backup_stage_completed", "stage", "objects", "object_count", len(manifest.ObjectStorage.Objects))
	}
	manifest.Database, err = backupDatabase(ctx, root)
	if err != nil {
		return err
	}
	if err := validateManifest(root, manifest); err != nil {
		return err
	}
	if err := writeManifest(root, manifest); err != nil {
		return err
	}
	completed = true
	slog.Info("backup_completed", "directory", root, "database_bytes", manifest.Database.Size)
	return nil
}

func runRestoreCommand(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "backup directory")
	confirmation := flags.String("confirm", "", "must be RESTORE")
	skipObjects := flags.Bool("skip-objects", false, "restore database only")
	overwriteObjects := flags.Bool("overwrite-objects", false, "replace conflicting objects")
	if err := flags.Parse(args); err != nil || strings.TrimSpace(*input) == "" {
		return errors.New("--input is required")
	}
	if *confirmation != "RESTORE" {
		return errors.New("restore confirmation is required")
	}
	root, err := filepath.Abs(*input)
	if err != nil {
		return err
	}
	manifest, err := readManifest(root)
	if err != nil {
		return err
	}
	if err := verifyBackupFile(root, manifest.Database.File, manifest.Database.Size, manifest.Database.SHA256); err != nil {
		return err
	}
	var actions []objectRestoreAction
	if !*skipObjects && manifest.ObjectStorage != nil {
		actions, err = preflightObjects(root, *manifest.ObjectStorage, *overwriteObjects)
		if err != nil {
			return err
		}
	}
	if err := restoreObjects(ctx, root, actions); err != nil {
		return err
	}
	slog.Info("restore_stage_completed", "stage", "objects", "uploaded_count", len(actions))
	if err := restoreDatabase(ctx, root, manifest.Database); err != nil {
		return err
	}
	slog.Info("restore_completed", "directory", root)
	return nil
}

func createBackupDirectory(parent string) (string, error) {
	parent, err := filepath.Abs(parent)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return "", err
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	root := filepath.Join(parent, "backup-"+time.Now().UTC().Format("20060102T150405Z")+"-"+hex.EncodeToString(random))
	return root, os.Mkdir(root, 0700)
}

func appVersion() string {
	for _, filename := range []string{"/app/VERSION", "VERSION"} {
		if data, err := os.ReadFile(filename); err == nil && strings.TrimSpace(string(data)) != "" {
			return strings.TrimSpace(string(data))
		}
	}
	return "unknown"
}

func fail(operation string, err error) {
	slog.Error("operation_failed", "operation", operation, "error_type", fmt.Sprintf("%T", err))
	os.Exit(1)
}
