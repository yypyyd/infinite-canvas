package repository

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestVideoRenderRequestHashStable(t *testing.T) {
	a := VideoRenderRequestHash("p1", 2, `{"shots":[]}`, `{"ratio":"16:9"}`)
	b := VideoRenderRequestHash("p1", 2, `{"shots":[]}`, `{"ratio":"16:9"}`)
	c := VideoRenderRequestHash("p1", 3, `{"shots":[]}`, `{"ratio":"16:9"}`)
	if a == "" || a != b {
		t.Fatalf("hash unstable: %s vs %s", a, b)
	}
	if a == c {
		t.Fatal("version should change hash")
	}
	_ = model.VideoRenderStatusQueued
}
