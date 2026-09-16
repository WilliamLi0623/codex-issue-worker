package worker

import (
	"testing"
)

func TestWorkerLockExcludesSecondProcessAndReleases(t *testing.T) {
	root := t.TempDir()
	first, err := AcquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(root); err == nil {
		t.Fatal("second worker acquired the shared lock")
	}
	first()
	second, err := AcquireLock(root)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	second()
}
