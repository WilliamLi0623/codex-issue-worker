package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultCompletedTaskRetention = 10
	activeMarker                  = ".task-active"
	completedMarker               = ".task-completed"
)

type completedTaskDirectory struct {
	path     string
	modified int64
}

func retainCompletedTaskDirectories(root string, keep int) error {
	if keep < 1 {
		return fmt.Errorf("completed task retention must be positive")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	completed := make([]completedTaskDirectory, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "issue-") {
			continue
		}
		marker := filepath.Join(root, entry.Name(), completedMarker)
		info, err := os.Stat(marker)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		completed = append(completed, completedTaskDirectory{
			path:     filepath.Join(root, entry.Name()),
			modified: info.ModTime().UnixNano(),
		})
	}
	sort.SliceStable(completed, func(i, j int) bool {
		return completed[i].modified > completed[j].modified
	})
	if len(completed) <= keep {
		return nil
	}
	for _, task := range completed[keep:] {
		if err := os.RemoveAll(task.path); err != nil {
			return err
		}
	}
	return nil
}

func (r *TaskRunner) completeTaskDirectory(taskRoot string) {
	_ = os.Remove(filepath.Join(taskRoot, activeMarker))
	if err := os.WriteFile(filepath.Join(taskRoot, completedMarker), nil, 0o600); err != nil {
		return
	}
	keep := r.cfg.CompletedTaskRetention
	if keep < 1 {
		keep = defaultCompletedTaskRetention
	}
	_ = retainCompletedTaskDirectories(r.cfg.WorkRoot, keep)
}
