package pathresolver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Resolve checks an absolute path directly or polls ordered bases for a relative path.
// If no matching target exists but a missing candidate can be selected safely,
// it returns that path with Exists set to false. Resolve never creates the target.
func Resolve(path string, options Options) (Result, error) {
	if path == "" {
		return Result{}, fmt.Errorf("path must not be empty")
	}
	if !validKind(options.Kind) {
		return Result{}, fmt.Errorf("unsupported path kind %d", options.Kind)
	}
	if filepath.IsAbs(path) {
		absolutePath := filepath.Clean(path)
		exists, reason, missing := inspectPathState(absolutePath, options.Kind)
		if reason != "" && !missing {
			return Result{}, newResolveError(path, CandidateFailure{
				Source: SourceAbsolute,
				Path:   absolutePath,
				Reason: reason,
			})
		}
		return Result{Path: absolutePath, Source: SourceAbsolute, Exists: exists}, nil
	}
	failures := make([]CandidateFailure, 0, 2+len(options.AdditionalBases))
	var missingFallback *Result
	missingFallbackBlocked := false
	workingDirectory, workingDirectoryErr := resolveWorkingDirectory(options.WorkingDirectory)
	if workingDirectoryErr != nil {
		failures = append(failures, CandidateFailure{
			Source: SourceWorkingDirectory,
			Reason: workingDirectoryErr.Error(),
		})
	} else if result, failure, missing := resolveFromBase(path, workingDirectory, workingDirectory, SourceWorkingDirectory, options.Kind); failure != nil {
		failures = append(failures, *failure)
		rememberMissingFallback(&missingFallback, result, missing)
		missingFallbackBlocked = missingFallbackBlocked || (!missing && result.Path != "")
	} else {
		return result, nil
	}

	if executableDirectory, err := resolveExecutableDirectory(options.ExecutablePath, workingDirectory); err != nil {
		failures = append(failures, CandidateFailure{
			Source: SourceExecutableDirectory,
			Reason: err.Error(),
		})
	} else if result, failure, missing := resolveFromBase(path, executableDirectory, workingDirectory, SourceExecutableDirectory, options.Kind); failure != nil {
		failures = append(failures, *failure)
		rememberMissingFallback(&missingFallback, result, missing)
		missingFallbackBlocked = missingFallbackBlocked || (!missing && result.Path != "")
	} else {
		return result, nil
	}

	for _, candidate := range options.AdditionalBases {
		source := candidate.Source
		if source == "" {
			source = SourceFallback
		}
		if result, failure, missing := resolveFromBase(path, candidate.Path, workingDirectory, source, options.Kind); failure != nil {
			failures = append(failures, *failure)
			rememberMissingFallback(&missingFallback, result, missing)
			missingFallbackBlocked = missingFallbackBlocked || (!missing && result.Path != "")
		} else {
			return result, nil
		}
	}

	if missingFallback != nil && !missingFallbackBlocked {
		return *missingFallback, nil
	}
	return Result{}, &ResolveError{Input: path, Failures: failures}
}

func rememberMissingFallback(fallback **Result, result Result, missing bool) {
	if !missing || *fallback != nil {
		return
	}
	*fallback = &result
}

func resolveWorkingDirectory(override string) (string, error) {
	if override == "" {
		return os.Getwd()
	}
	if filepath.IsAbs(override) {
		return filepath.Clean(override), nil
	}
	currentDirectory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("could not resolve relative working directory: %w", err)
	}
	return filepath.Clean(filepath.Join(currentDirectory, override)), nil
}

func resolveExecutableDirectory(override, workingDirectory string) (string, error) {
	executablePath := override
	if executablePath == "" {
		var err error
		executablePath, err = os.Executable()
		if err != nil {
			return "", fmt.Errorf("could not determine executable path: %w", err)
		}
	}

	absoluteExecutable, err := absolutePath(executablePath, workingDirectory)
	if err != nil {
		return "", fmt.Errorf("could not determine executable directory: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(absoluteExecutable); err == nil {
		absoluteExecutable = resolved
	}
	return filepath.Dir(absoluteExecutable), nil
}

func resolveFromBase(path, base, workingDirectory string, source Source, kind Kind) (Result, *CandidateFailure, bool) {
	absoluteBase, err := absolutePath(base, workingDirectory)
	if err != nil {
		return Result{}, &CandidateFailure{Source: source, Path: base, Reason: err.Error()}, false
	}

	baseInfo, err := os.Stat(absoluteBase)
	if err != nil {
		return Result{}, &CandidateFailure{Source: source, Path: absoluteBase, Reason: fileSystemReason(err)}, false
	}
	if !baseInfo.IsDir() {
		return Result{}, &CandidateFailure{Source: source, Path: absoluteBase, Reason: "base is not a directory"}, false
	}

	resolvedPath := filepath.Clean(filepath.Join(absoluteBase, path))
	exists, reason, missing := inspectPathState(resolvedPath, kind)
	if reason != "" {
		return Result{Path: resolvedPath, Base: absoluteBase, Source: source, Exists: exists}, &CandidateFailure{Source: source, Path: resolvedPath, Reason: reason}, missing
	}
	return Result{Path: resolvedPath, Base: absoluteBase, Source: source, Exists: exists}, nil, false
}

func absolutePath(path, workingDirectory string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if workingDirectory == "" {
		return "", fmt.Errorf("cannot resolve relative base without a working directory")
	}
	return filepath.Clean(filepath.Join(workingDirectory, path)), nil
}

func inspectPathState(path string, kind Kind) (exists bool, reason string, missing bool) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fileSystemReason(err), errors.Is(err, os.ErrNotExist)
	}
	switch kind {
	case KindFile:
		if !info.Mode().IsRegular() {
			return true, "path is not a regular file", false
		}
	case KindDirectory:
		if !info.IsDir() {
			return true, "path is not a directory", false
		}
	}
	return true, "", false
}

func fileSystemReason(err error) string {
	if errors.Is(err, os.ErrNotExist) {
		return "path does not exist"
	}
	return err.Error()
}

func validKind(kind Kind) bool {
	return kind == KindAny || kind == KindFile || kind == KindDirectory
}

func newResolveError(input string, failures ...CandidateFailure) *ResolveError {
	return &ResolveError{Input: input, Failures: failures}
}
