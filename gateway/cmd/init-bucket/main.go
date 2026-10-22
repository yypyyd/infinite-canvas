package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway/internal/storage"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	client := storage.New(env("RUSTFS_ENDPOINT"), env("RUSTFS_BUCKET"), env("RUSTFS_ACCESS_KEY"), env("RUSTFS_SECRET_KEY"))
	if !client.Configured() {
		log.Fatal("bucket initialization requires storage configuration")
	}
	for {
		probe, cancelProbe := context.WithTimeout(ctx, 5*time.Second)
		err := client.EnsureBucket(probe)
		cancelProbe()
		if err == nil {
			log.Print("bucket ready")
			return
		}
		select {
		case <-ctx.Done():
			// Upstream errors can include credentials or signed URLs.
			log.Fatal("bucket initialization failed; check storage availability and credentials")
		case <-time.After(2 * time.Second):
		}
	}
}

// env prefers GATEWAY_<key> and falls back to the bare key.
func env(key string) string {
	if v := os.Getenv("GATEWAY_" + key); v != "" {
		return v
	}
	return os.Getenv(key)
}
