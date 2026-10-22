package gateway

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/yypyyd/infinite-canvas/gateway/internal/bootstrap"
	"github.com/yypyyd/infinite-canvas/gateway/internal/config"
	"github.com/yypyyd/infinite-canvas/gateway/internal/migrations"
)

// ImportOptions controls ImportStandalone.
type ImportOptions struct {
	// SourceDSN points at the standalone Nexa PostgreSQL database.
	SourceDSN string
	// SourceSchema is the schema holding Nexa tables there (default "public").
	SourceSchema string
	// DryRun only reports row counts; nothing is written.
	DryRun bool
	// BatchSize is the number of rows per INSERT batch (default 500).
	BatchSize int
	Log       io.Writer
}

// importSkipTables are never copied: migration bookkeeping belongs to the
// target, and the standalone admin login is replaced by 幻图's super admin.
var importSkipTables = map[string]bool{"schema_migrations": true, "admins": true}

// ImportStandalone copies data from a standalone Nexa database into the
// embedded gateway schema (GATEWAY_POSTGRES_DSN / GATEWAY_DB_SCHEMA).
//
// It is idempotent: the target schema is migrated first, tables are copied in
// foreign-key order, rows are upserted by primary key (source wins), tables
// without a primary key use ON CONFLICT DO NOTHING, and serial sequences are
// advanced past the imported ids. Running it again re-applies the same rows.
func ImportStandalone(ctx context.Context, opts ImportOptions) error {
	out := opts.Log
	if out == nil {
		out = io.Discard
	}
	if strings.TrimSpace(opts.SourceDSN) == "" {
		return errors.New("source DSN is required")
	}
	sourceSchema := strings.TrimSpace(opts.SourceSchema)
	if sourceSchema == "" {
		sourceSchema = "public"
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = 500
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	targetSchema := strings.TrimSpace(cfg.DBSchema)
	if targetSchema == "" {
		targetSchema = "gateway"
	}

	targetDB, err := bootstrap.OpenDB(ctx, cfg.PostgresDSN, targetSchema, nil)
	if err != nil {
		return fmt.Errorf("open target: %w", err)
	}
	targetSQL, _ := targetDB.DB()
	defer targetSQL.Close()
	if err := bootstrap.Migrate(ctx, targetDB); err != nil {
		return fmt.Errorf("migrate target: %w", err)
	}

	source, err := pgx.Connect(ctx, opts.SourceDSN)
	if err != nil {
		return fmt.Errorf("connect source: %w", err)
	}
	defer source.Close(ctx)

	targetCfg, err := pgx.ParseConfig(cfg.PostgresDSN)
	if err != nil {
		return err
	}
	target, err := pgx.ConnectConfig(ctx, targetCfg)
	if err != nil {
		return fmt.Errorf("connect target: %w", err)
	}
	defer target.Close(ctx)

	if err := checkSourceMigrations(ctx, source, sourceSchema, out); err != nil {
		return err
	}

	targetTables, err := listTables(ctx, target, targetSchema)
	if err != nil {
		return err
	}
	sourceTables, err := listTables(ctx, source, sourceSchema)
	if err != nil {
		return err
	}
	sourceSet := map[string]bool{}
	for _, name := range sourceTables {
		sourceSet[name] = true
	}
	ordered, err := orderByForeignKeys(ctx, target, targetSchema, targetTables)
	if err != nil {
		return err
	}

	var failures []string
	for _, table := range ordered {
		if importSkipTables[table] {
			continue
		}
		if !sourceSet[table] {
			fmt.Fprintf(out, "skip %-36s (not in source)\n", table)
			continue
		}
		copied, err := importTable(ctx, source, target, sourceSchema, targetSchema, table, batchSize, opts.DryRun)
		if err != nil {
			fmt.Fprintf(out, "FAIL %-36s %v\n", table, err)
			failures = append(failures, table)
			continue
		}
		verb := "copied"
		if opts.DryRun {
			verb = "would copy"
		}
		fmt.Fprintf(out, "ok   %-36s %s %d rows\n", table, verb, copied)
	}
	if !opts.DryRun {
		if err := resetSequences(ctx, target, targetSchema, out); err != nil {
			return err
		}
		invalidateSettingsCache(ctx, target, targetSchema, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, out)
	}
	if len(failures) > 0 {
		return fmt.Errorf("import failed for %d table(s): %s", len(failures), strings.Join(failures, ", "))
	}
	return nil
}

func checkSourceMigrations(ctx context.Context, source *pgx.Conn, schema string, out io.Writer) error {
	known, err := migrations.Load()
	if err != nil {
		return err
	}
	latest := known[len(known)-1].Version
	var sourceVersion *int64
	err = source.QueryRow(ctx, "SELECT MAX(version) FROM "+pgx.Identifier{schema, "schema_migrations"}.Sanitize()).Scan(&sourceVersion)
	if err != nil {
		return fmt.Errorf("read source schema_migrations (is this a Nexa database?): %w", err)
	}
	if sourceVersion == nil {
		return errors.New("source has no applied Nexa migrations")
	}
	fmt.Fprintf(out, "source migration version %06d, gateway version %06d\n", *sourceVersion, latest)
	if *sourceVersion > latest {
		return fmt.Errorf("source is newer (%06d) than the embedded gateway (%06d); upgrade 幻图 first", *sourceVersion, latest)
	}
	return nil
}

func listTables(ctx context.Context, conn *pgx.Conn, schema string) ([]string, error) {
	rows, err := conn.Query(ctx, `SELECT table_name FROM information_schema.tables WHERE table_schema = $1 AND table_type = 'BASE TABLE' ORDER BY table_name`, schema)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func listColumns(ctx context.Context, conn *pgx.Conn, schema, table string) ([]string, error) {
	rows, err := conn.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 AND is_generated = 'NEVER' ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func primaryKey(ctx context.Context, conn *pgx.Conn, schema, table string) ([]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(i.indkey)
		WHERE i.indisprimary AND n.nspname = $1 AND c.relname = $2
		ORDER BY array_position(i.indkey, a.attnum)`, schema, table)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// orderByForeignKeys returns tables so that referenced tables come first.
func orderByForeignKeys(ctx context.Context, conn *pgx.Conn, schema string, tables []string) ([]string, error) {
	rows, err := conn.Query(ctx, `
		SELECT child.relname, parent.relname
		FROM pg_constraint con
		JOIN pg_class child ON child.oid = con.conrelid
		JOIN pg_class parent ON parent.oid = con.confrelid
		JOIN pg_namespace n ON n.oid = child.relnamespace
		WHERE con.contype = 'f' AND n.nspname = $1 AND child.oid <> parent.oid`, schema)
	if err != nil {
		return nil, err
	}
	deps := map[string]map[string]bool{}
	for rows.Next() {
		var child, parent string
		if err := rows.Scan(&child, &parent); err != nil {
			rows.Close()
			return nil, err
		}
		if deps[child] == nil {
			deps[child] = map[string]bool{}
		}
		deps[child][parent] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sorted := append([]string(nil), tables...)
	sort.Strings(sorted)
	done := map[string]bool{}
	visiting := map[string]bool{}
	var ordered []string
	var visit func(string)
	visit = func(name string) {
		if done[name] || visiting[name] {
			return
		}
		visiting[name] = true
		parents := make([]string, 0, len(deps[name]))
		for parent := range deps[name] {
			parents = append(parents, parent)
		}
		sort.Strings(parents)
		for _, parent := range parents {
			visit(parent)
		}
		visiting[name] = false
		done[name] = true
		ordered = append(ordered, name)
	}
	for _, name := range sorted {
		visit(name)
	}
	return ordered, nil
}

func importTable(ctx context.Context, source, target *pgx.Conn, sourceSchema, targetSchema, table string, batchSize int, dryRun bool) (int64, error) {
	targetColumns, err := listColumns(ctx, target, targetSchema, table)
	if err != nil {
		return 0, err
	}
	sourceColumns, err := listColumns(ctx, source, sourceSchema, table)
	if err != nil {
		return 0, err
	}
	sourceColumnSet := map[string]bool{}
	for _, name := range sourceColumns {
		sourceColumnSet[name] = true
	}
	var columns []string
	for _, name := range targetColumns {
		if sourceColumnSet[name] {
			columns = append(columns, name)
		}
	}
	if len(columns) == 0 {
		return 0, errors.New("no common columns")
	}
	if dryRun {
		var count int64
		err := source.QueryRow(ctx, "SELECT COUNT(*) FROM "+pgx.Identifier{sourceSchema, table}.Sanitize()).Scan(&count)
		return count, err
	}
	pk, err := primaryKey(ctx, target, targetSchema, table)
	if err != nil {
		return 0, err
	}

	quoted := make([]string, len(columns))
	for i, name := range columns {
		quoted[i] = pgx.Identifier{name}.Sanitize()
	}
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	insert := "INSERT INTO " + pgx.Identifier{targetSchema, table}.Sanitize() + " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	pkSet := map[string]bool{}
	for _, name := range pk {
		pkSet[name] = true
	}
	if len(pk) > 0 {
		quotedPK := make([]string, len(pk))
		for i, name := range pk {
			quotedPK[i] = pgx.Identifier{name}.Sanitize()
		}
		var updates []string
		for i, name := range columns {
			if !pkSet[name] {
				updates = append(updates, quoted[i]+" = EXCLUDED."+quoted[i])
			}
		}
		if len(updates) > 0 {
			insert += " ON CONFLICT (" + strings.Join(quotedPK, ", ") + ") DO UPDATE SET " + strings.Join(updates, ", ")
		} else {
			insert += " ON CONFLICT DO NOTHING"
		}
	} else {
		insert += " ON CONFLICT DO NOTHING"
	}

	selectSQL := "SELECT " + strings.Join(quoted, ", ") + " FROM " + pgx.Identifier{sourceSchema, table}.Sanitize()
	if len(pk) > 0 {
		quotedPK := make([]string, len(pk))
		for i, name := range pk {
			quotedPK[i] = pgx.Identifier{name}.Sanitize()
		}
		selectSQL += " ORDER BY " + strings.Join(quotedPK, ", ")
	}
	rows, err := source.Query(ctx, selectSQL)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	tx, err := target.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var copied int64
	batch := &pgx.Batch{}
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		results := tx.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return err
			}
		}
		batch = &pgx.Batch{}
		return results.Close()
	}
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return copied, err
		}
		batch.Queue(insert, values...)
		copied++
		if batch.Len() >= batchSize {
			if err := flush(); err != nil {
				return copied, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return copied, err
	}
	if err := flush(); err != nil {
		return copied, err
	}
	return copied, tx.Commit(ctx)
}

func resetSequences(ctx context.Context, conn *pgx.Conn, schema string, out io.Writer) error {
	rows, err := conn.Query(ctx, `
		SELECT c.table_name, c.column_name, pg_get_serial_sequence(quote_ident(c.table_schema) || '.' || quote_ident(c.table_name), c.column_name)
		FROM information_schema.columns c
		WHERE c.table_schema = $1
		  AND pg_get_serial_sequence(quote_ident(c.table_schema) || '.' || quote_ident(c.table_name), c.column_name) IS NOT NULL`, schema)
	if err != nil {
		return err
	}
	type seq struct{ table, column, name string }
	var seqs []seq
	for rows.Next() {
		var item seq
		if err := rows.Scan(&item.table, &item.column, &item.name); err != nil {
			rows.Close()
			return err
		}
		seqs = append(seqs, item)
	}
	rows.Close()
	for _, item := range seqs {
		statement := fmt.Sprintf("SELECT setval($1, GREATEST(COALESCE((SELECT MAX(%s) FROM %s), 0), 1), (SELECT MAX(%s) FROM %s) IS NOT NULL)",
			pgx.Identifier{item.column}.Sanitize(), pgx.Identifier{schema, item.table}.Sanitize(),
			pgx.Identifier{item.column}.Sanitize(), pgx.Identifier{schema, item.table}.Sanitize())
		if _, err := conn.Exec(ctx, statement, item.name); err != nil {
			return fmt.Errorf("reset sequence %s: %w", item.name, err)
		}
		fmt.Fprintf(out, "seq  %s.%s -> %s\n", item.table, item.column, item.name)
	}
	return nil
}

// invalidateSettingsCache drops the gateway's Redis copies of site settings
// ("setting:<key>") so a running 幻图 picks up imported values immediately.
func invalidateSettingsCache(ctx context.Context, conn *pgx.Conn, schema, addr, password string, db int, out io.Writer) {
	rows, err := conn.Query(ctx, "SELECT key FROM "+pgx.Identifier{schema, "site_settings"}.Sanitize())
	if err != nil {
		fmt.Fprintf(out, "warn settings cache not invalidated: %v\n", err)
		return
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(keys) == 0 {
		return
	}
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	defer client.Close()
	cacheKeys := make([]string, len(keys))
	for i, key := range keys {
		cacheKeys[i] = "setting:" + key
	}
	if err := client.Del(ctx, cacheKeys...).Err(); err != nil {
		fmt.Fprintf(out, "warn settings cache not invalidated (restart 幻图 or wait 5 min): %v\n", err)
		return
	}
	fmt.Fprintf(out, "cache invalidated %d setting keys\n", len(cacheKeys))
}
