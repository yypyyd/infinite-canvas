package service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func deliveryTestImage(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil { t.Fatal(err) }
	return output.Bytes()
}

func TestApplyProductionDeliverySpecCropsToExactDimensions(t *testing.T) {
	for _, test := range []struct { name string; sourceWidth, sourceHeight, width, height int }{
		{name: "landscape to portrait", sourceWidth: 320, sourceHeight: 160, width: 90, height: 120},
		{name: "portrait to landscape", sourceWidth: 160, sourceHeight: 320, width: 120, height: 90},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := deliveryTestImage(t, test.sourceWidth, test.sourceHeight)
			result, err := applyProductionDeliverySpec(BatchProductionResult{Data: data, Size: int64(len(data)), MimeType: "image/png", ResultURL: "https://example.com/source.png"}, model.ProductionDeliverySpec{Width: test.width, Height: test.height, Format: "jpeg", Quality: 90})
			if err != nil { t.Fatal(err) }
			decoded, format, err := image.Decode(bytes.NewReader(result.Data))
			if err != nil { t.Fatal(err) }
			if decoded.Bounds().Dx() != test.width || decoded.Bounds().Dy() != test.height { t.Fatalf("delivery dimensions = %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy()) }
			if format != "jpeg" || result.MimeType != "image/jpeg" || http.DetectContentType(result.Data) != "image/jpeg" { t.Fatalf("unexpected JPEG result: format=%s mime=%s", format, result.MimeType) }
			if result.Size != int64(len(result.Data)) || result.Size == 0 || result.ResultURL != "" { t.Fatalf("unexpected delivery metadata: %#v", result) }
		})
	}
}

func TestApplyProductionDeliverySpecKeepsOriginalResult(t *testing.T) {
	data := deliveryTestImage(t, 32, 24)
	original := BatchProductionResult{Data: data, Size: int64(len(data)), MimeType: "image/png", ResultURL: "https://example.com/source.png"}
	result, err := applyProductionDeliverySpec(original, model.ProductionDeliverySpec{ID: "original", Format: "original"})
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(result.Data, original.Data) || result.Size != original.Size || result.MimeType != original.MimeType || result.ResultURL != original.ResultURL { t.Fatalf("original delivery result changed: %#v", result) }
}
