package pathresolver

// Kind describes the expected type of a resolved path.
type Kind uint8

const (
	// KindAny accepts any existing file-system entry.
	KindAny Kind = iota
	// KindFile accepts an existing regular file.
	KindFile
	// KindDirectory accepts an existing directory.
	KindDirectory
)

// Source identifies the base used to resolve a path.
type Source string

const (
	// SourceAbsolute identifies a directly checked absolute path.
	SourceAbsolute Source = "absolute"
	// SourceWorkingDirectory identifies the current working directory.
	SourceWorkingDirectory Source = "working-directory"
	// SourceExecutableDirectory identifies the executable's directory.
	SourceExecutableDirectory Source = "executable-directory"
	// SourceFallback identifies a caller-provided additional base directory.
	SourceFallback Source = "fallback"
)

// Candidate is an optional base directory checked after the standard bases.
type Candidate struct {
	Path   string
	Source Source
}

// Options controls path validation and the ordered fallback bases.
type Options struct {
	Kind             Kind
	WorkingDirectory string
	ExecutablePath   string
	AdditionalBases  []Candidate
}

// Result describes the resolved absolute path, its source, and its existence state.
type Result struct {
	Path   string
	Base   string
	Source Source
	// Exists is true when Path exists and matches the requested Kind.
	// Resolve returns false when it returns a path that does not exist.
	Exists bool
}
