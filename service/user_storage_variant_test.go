package service

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestLocalImageVariantGeneration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "files")
	setting := model.StorageSetting{Driver: "local", LocalPath: root}
	objectKey := "org/a/image.png"
	source, err := localStorageObjectPath(setting, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1536, 1024))
	for y := 0; y < 1024; y++ {
		for x := 0; x < 1536; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 80, 255})
		}
	}
	file, _ := os.Create(source)
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	_ = file.Close()

	for variant, want := range map[string][2]int{"thumb": {160, 160}, "cover": {640, 426}, "preview": {1280, 853}} {
		base, err := localImageVariantCacheBase(setting, objectKey, variant)
		if err != nil {
			t.Fatal(err)
		}
		if rel, _ := filepath.Rel(root, base); filepath.IsLocal(rel) {
			t.Fatalf("variant cache must live outside the storage root: %s", base)
		}
		if err := writeLocalImageVariant(source, base, variant); err != nil {
			t.Fatal(err)
		}
		output, mimeType, ok := openLocalImageVariant(base)
		if !ok || mimeType != "image/jpeg" {
			t.Fatalf("%s: variant not cached as jpeg: %v %s", variant, ok, mimeType)
		}
		config, _, err := image.DecodeConfig(output)
		_ = output.Close()
		if err != nil || config.Width != want[0] || config.Height != want[1] {
			t.Fatalf("%s: got %dx%d, want %v (%v)", variant, config.Width, config.Height, want, err)
		}
	}
	removeLocalImageVariants(setting, objectKey)
	for _, variant := range []string{"thumb", "cover", "preview"} {
		base, _ := localImageVariantCacheBase(setting, objectKey, variant)
		if _, _, ok := openLocalImageVariant(base); ok {
			t.Fatalf("%s variant should be removed with the original", variant)
		}
	}
	if _, err := localImageVariantCacheBase(setting, "../escape.png", "thumb"); err == nil {
		t.Fatal("expected traversal key to be rejected")
	}
}
