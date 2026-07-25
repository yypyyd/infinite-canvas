package main

import "testing"

func TestPlanObjectRestore(t *testing.T) {
	object := ObjectBackup{Key: "object", Size: 12, QiniuHash: "etag"}
	if action, err := planObjectRestore(true, "etag", 12, object, false); err != nil || action != nil {
		t.Fatal("same object should be skipped")
	}
	if _, err := planObjectRestore(true, "different", 12, object, false); err == nil {
		t.Fatal("conflicting object should be rejected by default")
	}
	action, err := planObjectRestore(true, "different", 12, object, true)
	if err != nil || action == nil || !action.Overwrite {
		t.Fatal("explicit overwrite should replace conflicting object")
	}
	action, err = planObjectRestore(false, "", 0, object, false)
	if err != nil || action == nil || action.Overwrite {
		t.Fatal("missing object should use insert-only upload")
	}
}
