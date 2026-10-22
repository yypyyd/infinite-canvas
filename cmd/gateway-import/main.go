// Command gateway-import copies data from a standalone Nexa (imageNexa)
// PostgreSQL database into the gateway schema embedded in 幻图.
//
// Usage:
//
//	GATEWAY_POSTGRES_DSN=... go run ./cmd/gateway-import -source "postgres://user:pass@host:5432/nexa?sslmode=disable" [-source-schema public] [-dry-run]
//
// The target is read from GATEWAY_POSTGRES_DSN / GATEWAY_DB_SCHEMA (or .env).
// The command is idempotent and safe to re-run; see docs/gateway.md.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/joho/godotenv"

	"github.com/yypyyd/infinite-canvas/gateway"
)

func main() {
	source := flag.String("source", os.Getenv("NEXA_SOURCE_DSN"), "standalone Nexa PostgreSQL DSN (or NEXA_SOURCE_DSN)")
	sourceSchema := flag.String("source-schema", "public", "schema holding the Nexa tables in the source database")
	dryRun := flag.Bool("dry-run", false, "only count source rows; write nothing")
	batchSize := flag.Int("batch-size", 500, "rows per insert batch")
	flag.Parse()
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := gateway.ImportStandalone(ctx, gateway.ImportOptions{
		SourceDSN:    *source,
		SourceSchema: *sourceSchema,
		DryRun:       *dryRun,
		BatchSize:    *batchSize,
		Log:          os.Stdout,
	})
	if err != nil {
		log.Fatalf("gateway import: %v", err)
	}
	log.Print("gateway import finished")
}
