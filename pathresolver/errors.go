package pathresolver

import (
	"fmt"
	"strings"
)

// CandidateFailure records why one resolution candidate was rejected.
type CandidateFailure struct {
	Source Source
	Path   string
	Reason string
}

// ResolveError contains the failures collected while checking a path.
type ResolveError struct {
	Input    string
	Failures []CandidateFailure
}

// Error formats the input path and each rejected candidate.
func (e *ResolveError) Error() string {
	if e == nil {
		return "path resolution failed"
	}

	var message strings.Builder
	fmt.Fprintf(&message, "resolve %q failed", e.Input)
	for _, failure := range e.Failures {
		fmt.Fprintf(&message, "; %s", failure.Source)
		if failure.Path != "" {
			fmt.Fprintf(&message, " %q", failure.Path)
		}
		fmt.Fprintf(&message, ": %s", failure.Reason)
	}
	return message.String()
}
