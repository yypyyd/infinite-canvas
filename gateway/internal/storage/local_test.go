package storage

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestLocalStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewLocal(t.TempDir())
	if !store.Configured() || store.EnsureBucket(ctx) != nil || store.Check(ctx) != nil {
		t.Fatal("local store must be usable without configuration")
	}
	if err := store.Put(ctx, "img/a/b.png", []byte("0123456789"), "image/png"); err != nil {
		t.Fatal(err)
	}
	resp, err := store.Get(ctx, "img/a/b.png", "")
	if err != nil || resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("get: %v %v", err, resp)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "0123456789" {
		t.Fatalf("body %q", body)
	}
	resp, err = store.Get(ctx, "img/a/b.png", "bytes=2-4")
	if err != nil || resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("range: %v %v", err, resp)
	}
	body, _ = io.ReadAll(resp.Body)
	if string(body) != "234" {
		t.Fatalf("range body %q", body)
	}
	if objects, err := store.List(ctx, "img/"); err != nil || len(objects) != 1 || objects[0].Key != "img/a/b.png" {
		t.Fatalf("list %v %v", objects, err)
	}
	if err := store.Delete(ctx, "img/a/b.png"); err != nil {
		t.Fatal(err)
	}
	if resp, _ := store.Get(ctx, "img/a/b.png", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted object status %d", resp.StatusCode)
	}
	for _, key := range []string{"", "../x", "a/../../x", "a\\b"} {
		if err := store.Put(ctx, key, []byte("x"), ""); err == nil {
			t.Fatalf("key %q must be rejected", key)
		}
	}
}
