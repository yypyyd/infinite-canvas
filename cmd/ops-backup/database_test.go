package main

import (
	"slices"
	"strings"
	"testing"
)

func TestMySQLPasswordIsOnlyInEnvironment(t *testing.T) {
	spec, err := mysqlCommand("mysqldump", "backup_user:very-secret@tcp(db.internal:3307)/canvas")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(spec.Args, " "), "very-secret") {
		t.Fatal("password must not be present in command arguments")
	}
	if !slices.Contains(spec.Env, "MYSQL_PWD=very-secret") {
		t.Fatal("password environment variable is missing")
	}
}

func TestPostgresDSNIsOnlyInEnvironment(t *testing.T) {
	dsn := "postgres://backup:very-secret@db.internal/canvas"
	spec := postgresCommand("pg_dump", dsn)
	if strings.Contains(strings.Join(spec.Args, " "), dsn) {
		t.Fatal("dsn must not be present in command arguments")
	}
	if !slices.Contains(spec.Env, "PGDATABASE="+dsn) {
		t.Fatal("dsn environment variable is missing")
	}
}

func TestSQLiteDatabasePathRejectsMemoryDatabase(t *testing.T) {
	for _, dsn := range []string{":memory:", "file:memory?mode=memory"} {
		if _, err := sqliteDatabasePath(dsn); err == nil {
			t.Fatalf("expected %q to be rejected", dsn)
		}
	}
}
