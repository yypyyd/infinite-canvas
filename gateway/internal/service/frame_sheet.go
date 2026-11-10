package service

import (
	"bytes"
	"errors"
	"image"
	"math"

	"golang.org/x/image/draw"
)

// ChatGPT Images answers a GIF request with one picture that holds the frames in
// a grid, read left to right and top to bottom; its own page plays that sheet as
// an animation. The layout matches the request added by chatgpt.injectOutputHint.
// The gateway only checks the sheet and describes it; the caller cuts the frames
// and assembles the animation.
const (
	frameSheetColumns = 3
	frameSheetRows    = 3
	// frameSheetDurationMS is how long each frame shows, the Image API default.
	frameSheetDurationMS = 200
)

var errNotFrameSheet = errors.New("image is not an animation frame sheet")

// imageDataItem builds the one-element data list of an image response. A gif
// request served as a frame sheet also carries the layout to cut it with.
func imageDataItem(item map[string]any, outputFormat, mimeType string) []map[string]any {
	if outputFormat == "gif" && mimeType != "image/gif" {
		item["frame_sheet"] = map[string]any{"columns": frameSheetColumns, "rows": frameSheetRows, "frame_duration_ms": frameSheetDurationMS}
	}
	return []map[string]any{item}
}

// checkFrameSheet tells a frame sheet from a single picture: consecutive frames of
// an animation show the same scene, while neighbouring cells of one picture do not.
func checkFrameSheet(data []byte) error {
	sheet, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	bounds := sheet.Bounds()
	cell := image.Rect(0, 0, bounds.Dx()/frameSheetColumns, bounds.Dy()/frameSheetRows)
	if cell.Dx() < 16 || cell.Dy() < 16 {
		return errNotFrameSheet
	}
	const side = 16
	var previous []float64
	for index := 0; index < frameSheetColumns*frameSheetRows; index++ {
		origin := bounds.Min.Add(image.Pt(index%frameSheetColumns*cell.Dx(), index/frameSheetColumns*cell.Dy()))
		small := image.NewRGBA(image.Rect(0, 0, side, side))
		draw.BiLinear.Scale(small, small.Bounds(), sheet, cell.Add(origin), draw.Src, nil)
		current := make([]float64, 0, side*side*3)
		for i, value := range small.Pix {
			if i%4 != 3 {
				current = append(current, float64(value)/255)
			}
		}
		if previous != nil && !thumbnailsAlike(previous, current) {
			return errNotFrameSheet
		}
		previous = current
	}
	return nil
}

// thumbnailsAlike accepts two frames that barely differ (a small subject moving
// on a plain background) or that share the same layout (correlated colours).
func thumbnailsAlike(a, b []float64) bool {
	var difference, meanA, meanB float64
	for i := range a {
		difference += math.Abs(a[i] - b[i])
		meanA += a[i]
		meanB += b[i]
	}
	count := float64(len(a))
	if difference/count <= 0.05 {
		return true
	}
	meanA, meanB = meanA/count, meanB/count
	var covariance, varianceA, varianceB float64
	for i := range a {
		deltaA, deltaB := a[i]-meanA, b[i]-meanB
		covariance += deltaA * deltaB
		varianceA += deltaA * deltaA
		varianceB += deltaB * deltaB
	}
	return varianceA > 0 && varianceB > 0 && covariance/math.Sqrt(varianceA*varianceB) >= 0.7
}
