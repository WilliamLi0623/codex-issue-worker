package worker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetainCompletedTaskDirectoriesKeepsNewestAndActive(t *testing.T) {
	root := t.TempDir()
	old := makeTaskDirectory(t, root, "issue-1-old", completedMarker)
	newest := makeTaskDirectory(t, root, "issue-2-newest", completedMarker)
	active := makeTaskDirectory(t, root, "issue-3-active", activeMarker)
	if err := os.Chtimes(filepath.Join(old, completedMarker), time.Time{}, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(newest, completedMarker), time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := retainCompletedTaskDirectories(root, 1); err != nil {
		t.Fatal(err)
	}

	assertDirectoryExists(t, newest)
	assertDirectoryExists(t, active)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old completed directory still exists, err=%v", err)
	}
}

func TestRetainCompletedTaskDirectoriesIgnoresUnmarkedDirectories(t *testing.T) {
	root := t.TempDir()
	unmarked := makeTaskDirectory(t, root, "issue-1-unmarked")
	completed := makeTaskDirectory(t, root, "issue-2-completed", completedMarker)
	newest := makeTaskDirectory(t, root, "issue-3-newest", completedMarker)
	if err := os.Chtimes(filepath.Join(completed, completedMarker), time.Time{}, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(newest, completedMarker), time.Time{}, time.Now()); err != nil {
		t.Fatal(err)
	}

	if err := retainCompletedTaskDirectories(root, 1); err != nil {
		t.Fatal(err)
	}

	assertDirectoryExists(t, unmarked)
	assertDirectoryExists(t, newest)
	if _, err := os.Stat(completed); !os.IsNotExist(err) {
		t.Fatalf("completed directory still exists, err=%v", err)
	}
}

func makeTaskDirectory(t *testing.T, root, name string, markers ...string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, marker := range markers {
		if err := os.WriteFile(filepath.Join(dir, marker), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func assertDirectoryExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("directory %q missing: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
}
