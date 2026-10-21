package service

import (
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp"
)

// Local storage has no image-processing service like Qiniu's imageView2, so list thumbnails
// and previews used to download full-size originals. Variants are generated once and cached
// beside (not inside) the storage root so quota accounting and object keys stay untouched.
var localImageVariantSlots = make(chan struct{}, 2)

const localImageVariantMaxSourceBytes = 64 << 20

// OpenLocalUserWorkspaceImageVariant returns a cached thumb/preview for a local image file.
// ok=false means the caller should serve the original file instead.
func OpenLocalUserWorkspaceImageVariant(user model.AuthUser, storageKey, variant string) (*os.File, string, model.UserFile, bool) {
	file, mimeType, item, ok := openLocalUserWorkspaceImageVariant(user, storageKey, variant)
	return file, mimeType, item, ok
}

func openLocalUserWorkspaceImageVariant(user model.AuthUser, storageKey, variant string) (*os.File, string, model.UserFile, bool) {
	variant = strings.TrimSpace(variant)
	if variant != "thumb" && variant != "cover" && variant != "preview" {
		return nil, "", model.UserFile{}, false
	}
	item, ok, err := repository.GetUserFile(user.OrganizationID, strings.TrimSpace(storageKey))
	if err != nil || !ok || effectiveStorageDriver(item.StorageDriver) != "local" || !localImageVariantSupported(item.MimeType) || item.Size > localImageVariantMaxSourceBytes {
		return nil, "", model.UserFile{}, false
	}
	setting, err := currentUserStorageSetting()
	if err != nil {
		return nil, "", model.UserFile{}, false
	}
	source, err := localStorageObjectPath(setting, item.ObjectKey)
	if err != nil {
		return nil, "", model.UserFile{}, false
	}
	cacheBase, err := localImageVariantCacheBase(setting, item.ObjectKey, variant)
	if err != nil {
		return nil, "", model.UserFile{}, false
	}
	if file, mimeType, ok := openLocalImageVariant(cacheBase); ok {
		return file, mimeType, item, true
	}
	localImageVariantSlots <- struct{}{}
	defer func() { <-localImageVariantSlots }()
	if file, mimeType, ok := openLocalImageVariant(cacheBase); ok {
		return file, mimeType, item, true
	}
	if err := writeLocalImageVariant(source, cacheBase, variant); err != nil {
		return nil, "", model.UserFile{}, false
	}
	file, mimeType, ok := openLocalImageVariant(cacheBase)
	return file, mimeType, item, ok
}

func localImageVariantSupported(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	}
	return false
}

func localImageVariantCacheBase(setting model.StorageSetting, objectKey, variant string) (string, error) {
	root, err := filepath.Abs(setting.LocalPath)
	if err != nil || filepath.Dir(root) == root {
		return "", errors.New("local storage root is invalid")
	}
	if _, err := localStorageObjectPath(setting, objectKey); err != nil {
		return "", err
	}
	cleanKey := filepath.ToSlash(filepath.Clean(objectKey))
	return filepath.Join(root+"-variants", filepath.FromSlash(cleanKey)+"."+variant), nil
}

func openLocalImageVariant(base string) (*os.File, string, bool) {
	for _, candidate := range []struct{ ext, mimeType string }{{".jpg", "image/jpeg"}, {".png", "image/png"}} {
		if file, err := os.Open(base + candidate.ext); err == nil {
			return file, candidate.mimeType, true
		}
	}
	return nil, "", false
}

func writeLocalImageVariant(source, base, variant string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	config, _, err := image.DecodeConfig(input)
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width*config.Height > 60_000_000 {
		return errors.New("image variant source is unsupported")
	}
	if _, err := input.Seek(0, 0); err != nil {
		return err
	}
	img, err := imaging.Decode(input, imaging.AutoOrientation(true))
	if err != nil {
		return err
	}
	var output *image.NRGBA
	if variant == "thumb" {
		output = imaging.Fill(img, 160, 160, imaging.Center, imaging.Lanczos)
	} else if variant == "cover" {
		output = imaging.Fit(img, 640, 640, imaging.Lanczos)
	} else {
		output = imaging.Fit(img, 1280, 1280, imaging.Lanczos)
	}
	ext := ".jpg"
	if !output.Opaque() {
		ext = ".png"
	}
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(base), ".variant-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if ext == ".jpg" {
		err = jpeg.Encode(temporary, output, &jpeg.Options{Quality: 80})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(temporary, output)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, base+ext)
}

func removeLocalImageVariants(setting model.StorageSetting, objectKey string) {
	for _, variant := range []string{"thumb", "cover", "preview"} {
		base, err := localImageVariantCacheBase(setting, objectKey, variant)
		if err != nil {
			return
		}
		_ = os.Remove(base + ".jpg")
		_ = os.Remove(base + ".png")
	}
}
