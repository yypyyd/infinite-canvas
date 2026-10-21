package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestUserWorkspaceFileETag(t *testing.T) {
	first := UserWorkspaceFileETag(model.UserFile{Hash: "abc", ObjectKey: "o/1", Size: 1}, "")
	if first != `"abc"` {
		t.Fatalf("unexpected etag %s", first)
	}
	if UserWorkspaceFileETag(model.UserFile{Hash: "abc"}, "thumb") == first {
		t.Fatal("variants must not share the original etag")
	}
	if UserWorkspaceFileETag(model.UserFile{Hash: "def"}, "") == first {
		t.Fatal("re-uploaded content must change the etag")
	}
	if got := UserWorkspaceFileETag(model.UserFile{ObjectKey: "o/2 x", Size: 9}, ""); got != `"o_2_x-9"` {
		t.Fatalf("fallback etag %s", got)
	}
}
