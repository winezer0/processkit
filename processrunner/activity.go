package processrunner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const fileActivityPollInterval = 250 * time.Millisecond

var errStopOnIdle = errors.New("process stopped after idle activity window")

func resolveResultFiles(directory string, paths []string) ([]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	workingDirectory := directory
	if workingDirectory == "" {
		var err error
		workingDirectory, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		workingDirectory, err = filepath.Abs(workingDirectory)
		if err != nil {
			return nil, err
		}
	}
	resolved := make([]string, len(paths))
	for index, path := range paths {
		if filepath.IsAbs(path) {
			resolved[index] = filepath.Clean(path)
		} else {
			resolved[index] = filepath.Clean(filepath.Join(workingDirectory, path))
		}
	}
	return resolved, nil
}

type fileSnapshot struct {
	exists  bool
	size    int64
	modTime int64
}

func snapshotFiles(paths []string) []fileSnapshot {
	snapshots := make([]fileSnapshot, len(paths))
	for index, path := range paths {
		info, err := os.Stat(path)
		if err == nil {
			snapshots[index] = fileSnapshot{exists: true, size: info.Size(), modTime: info.ModTime().UnixNano()}
		}
	}
	return snapshots
}

func sameFileSnapshots(left, right []fileSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stopReasonFor(cause error) StopReason {
	switch {
	case errors.Is(cause, errStopOnIdle):
		return StopIdle
	case errors.Is(cause, context.DeadlineExceeded):
		return StopDeadline
	case errors.Is(cause, context.Canceled):
		return StopCanceled
	default:
		return StopCanceled
	}
}
