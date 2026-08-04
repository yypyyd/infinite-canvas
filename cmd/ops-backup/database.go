package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/repository"
	mysqldriver "github.com/go-sql-driver/mysql"
)

type commandSpec struct {
	Name string
	Args []string
	Env  []string
}

func normalizedDatabaseDriver() string {
	driver := strings.ToLower(strings.TrimSpace(config.Cfg.StorageDriver))
	if driver == "" {
		return "sqlite"
	}
	if driver == "postgresql" {
		return "postgres"
	}
	return driver
}

func backupDatabase(ctx context.Context, root string) (DatabaseBackup, error) {
	driver := normalizedDatabaseDriver()
	name := map[string]string{"sqlite": "database.db", "mysql": "database.sql", "postgres": "database.dump"}[driver]
	if name == "" {
		return DatabaseBackup{}, errors.New("unsupported database driver")
	}
	relative := filepath.ToSlash(filepath.Join("database", name))
	filename, err := safeBackupPath(root, relative)
	if err != nil {
		return DatabaseBackup{}, err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return DatabaseBackup{}, err
	}
	switch driver {
	case "sqlite":
		db, dbErr := repository.DB()
		if dbErr != nil {
			return DatabaseBackup{}, dbErr
		}
		err = db.WithContext(ctx).Exec("VACUUM INTO ?", filename).Error
	case "mysql":
		var spec commandSpec
		spec, err = mysqlCommand("mysqldump", config.Cfg.DatabaseDSN)
		if err == nil {
			databaseName := spec.Args[len(spec.Args)-1]
			spec.Args = append(spec.Args[:len(spec.Args)-1], "--single-transaction", "--quick", "--routines", "--triggers", "--events", "--hex-blob", "--default-character-set=utf8mb4", "--result-file="+filename, databaseName)
			err = runCommand(ctx, spec, nil)
		}
	case "postgres":
		spec := postgresCommand("pg_dump", config.Cfg.DatabaseDSN)
		spec.Args = []string{"--format=custom", "--file=" + filename}
		err = runCommand(ctx, spec, nil)
	}
	if err != nil {
		return DatabaseBackup{}, err
	}
	size, digest, err := fileDigest(filename)
	return DatabaseBackup{Driver: driver, File: relative, Size: size, SHA256: digest}, err
}

func restoreDatabase(ctx context.Context, root string, backup DatabaseBackup) error {
	if backup.Driver != normalizedDatabaseDriver() {
		return errors.New("backup database driver does not match configuration")
	}
	filename, err := safeBackupPath(root, backup.File)
	if err != nil {
		return err
	}
	switch backup.Driver {
	case "sqlite":
		return restoreSQLite(filename, config.Cfg.DatabaseDSN)
	case "mysql":
		spec, err := mysqlCommand("mysql", config.Cfg.DatabaseDSN)
		if err != nil {
			return err
		}
		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()
		return runCommand(ctx, spec, file)
	case "postgres":
		spec := postgresCommand("pg_restore", config.Cfg.DatabaseDSN)
		spec.Args = []string{"--clean", "--if-exists", "--no-owner", "--no-privileges", filename}
		return runCommand(ctx, spec, nil)
	default:
		return errors.New("unsupported database driver")
	}
}

func mysqlCommand(name, dsn string) (commandSpec, error) {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return commandSpec{}, err
	}
	if strings.TrimSpace(cfg.DBName) == "" {
		return commandSpec{}, errors.New("mysql database name is required")
	}
	args := []string{"--user=" + cfg.User}
	switch cfg.Net {
	case "unix":
		args = append(args, "--socket="+cfg.Addr)
	default:
		host, port := splitHostPort(cfg.Addr, "3306")
		args = append(args, "--host="+host, "--port="+port)
	}
	args = append(args, cfg.DBName)
	return commandSpec{Name: name, Args: args, Env: []string{"MYSQL_PWD=" + cfg.Passwd}}, nil
}

func splitHostPort(address, defaultPort string) (string, string) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "127.0.0.1", defaultPort
	}
	index := strings.LastIndex(address, ":")
	if index <= 0 || index == len(address)-1 {
		return address, defaultPort
	}
	return strings.Trim(address[:index], "[]"), address[index+1:]
}

func postgresCommand(name, dsn string) commandSpec {
	return commandSpec{Name: name, Env: []string{"PGDATABASE=" + dsn}}
}

func runCommand(ctx context.Context, spec commandSpec, stdin io.Reader) error {
	command := exec.CommandContext(ctx, spec.Name, spec.Args...)
	command.Env = append(os.Environ(), spec.Env...)
	command.Stdin = stdin
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return fmt.Errorf("database client failed: %w", err)
	}
	return nil
}

func sqliteDatabasePath(dsn string) (string, error) {
	value := strings.TrimSpace(dsn)
	if value == "" || value == ":memory:" || strings.Contains(value, "mode=memory") {
		return "", errors.New("file-based sqlite database is required")
	}
	if strings.HasPrefix(value, "file:") {
		value = strings.TrimPrefix(value, "file:")
	}
	value, _, _ = strings.Cut(value, "?")
	if value == "" {
		return "", errors.New("invalid sqlite database path")
	}
	return filepath.Abs(filepath.FromSlash(value))
}

func restoreSQLite(source, dsn string) error {
	if err := checkSQLiteBackup(source); err != nil {
		return err
	}
	target, err := sqliteDatabasePath(dsn)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	temporary := target + ".restore.tmp"
	if err := copyFile(source, temporary); err != nil {
		return err
	}
	defer os.Remove(temporary)

	suffix := ".pre-restore-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	originals := []string{target, target + "-wal", target + "-shm"}
	renamed := make([][2]string, 0, len(originals))
	rollback := func() {
		for index := len(renamed) - 1; index >= 0; index-- {
			_ = os.Rename(renamed[index][1], renamed[index][0])
		}
	}
	for _, original := range originals {
		if _, statErr := os.Stat(original); errors.Is(statErr, os.ErrNotExist) {
			continue
		} else if statErr != nil {
			rollback()
			return statErr
		}
		backup := original + suffix
		if err := os.Rename(original, backup); err != nil {
			rollback()
			return err
		}
		renamed = append(renamed, [2]string{original, backup})
	}
	if err := os.Rename(temporary, target); err != nil {
		rollback()
		return err
	}
	return nil
}

func checkSQLiteBackup(filename string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filename)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var result string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("sqlite backup integrity check failed")
	}
	return nil
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}
