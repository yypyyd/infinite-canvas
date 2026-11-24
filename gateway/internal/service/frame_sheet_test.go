package service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestFrameSheetPoseChanges(t *testing.T) {
	for _, tc := range []struct {
		name  string
		poses [9]int
		valid bool
	}{
		{"different poses with an intermediate pose", [9]int{0, 1, 0, 1, 2, 0, 1, 0, 0}, true},
		{"one unrelated cell", [9]int{0, 0, 0, 0, 1, 0, 0, 0, 0}, false},
		{"identical frames", [9]int{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const cell = 16
			sheet := image.NewGray(image.Rect(0, 0, 3*cell, 3*cell))
			for i, pose := range tc.poses {
				for y := 0; y < cell; y++ {
					for x := 0; x < cell; x++ {
						horizontal, vertical := 32+(y/4%2)*192, 32+(x/4%2)*192
						intensity := []int{horizontal, vertical, (horizontal + vertical) / 2}[pose]
						sheet.SetGray(i%3*cell+x, i/3*cell+y, color.Gray{Y: uint8(intensity)})
					}
				}
			}
			var encoded bytes.Buffer
			if err := png.Encode(&encoded, sheet); err != nil {
				t.Fatal(err)
			}
			if err := checkFrameSheet(encoded.Bytes()); (err == nil) != tc.valid {
				t.Fatalf("valid=%t error=%v", tc.valid, err)
			}
		})
	}
}
