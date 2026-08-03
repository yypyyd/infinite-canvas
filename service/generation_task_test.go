package service

import (
	"errors"
	"testing"
)

func TestGenerationTaskWriteConflictDetection(t *testing.T) {
	for _, message := range []string{"database is locked (5) (SQLITE_BUSY)", "database table is locked", "SQLITE_LOCKED"} {
		if !isGenerationTaskWriteConflict(errors.New(message)) {
			t.Fatalf("%q should be retryable", message)
		}
	}
	if isGenerationTaskWriteConflict(errors.New("validation failed")) {
		t.Fatal("non-database error should not be retryable")
	}
}
