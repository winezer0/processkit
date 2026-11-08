// Package processrunner executes bounded commands and owns their process trees.
package processrunner

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	DefaultCaptureBytes = 1 << 20
	DefaultLogBytes     = 8 << 20
	processStopTimeout  = 8 * time.Second
	processForceTimeout = 3 * time.Second
	StateStarting       = "starting"
	StateRunning        = "running"
	StateSuccess        = "success"
	StateFailed         = "failed"
	StateCanceled       = "canceled"
	StateTimedOut       = "timed_out"
)

// StopReason describes what caused a process execution to finish.
type StopReason string

const (
	StopExited   StopReason = "exited"
	StopIdle     StopReason = "idle"
	StopDeadline StopReason = "deadline"
	StopCanceled StopReason = "canceled"
)

// ActivitySource identifies a signal that can keep an idle process alive.
type ActivitySource string

const (
	ActivityProcessStdout ActivitySource = "stdout"
	ActivityProcessStderr ActivitySource = "stderr"
	ActivityResultFiles   ActivitySource = "result_files"
)

// IdleCondition controls how selected activity sources are combined.
type IdleCondition string

const (
	// IdleAllSources stops only after every selected source has been idle.
	IdleAllSources IdleCondition = "all_idle"
	// IdleAnySource stops after at least one selected source has been idle.
	IdleAnySource IdleCondition = "any_idle"
)

// ProgressDetector reports whether a complete process output line represents
// meaningful progress. It is called serially for stdout and stderr lines.
// When configured, raw output bytes do not reset the idle timer; only lines
// accepted by the detector do. The detector should be quick and should ignore
// heartbeat-only or timestamp-only lines. It is used only when StopOnIdle is
// enabled and may continue being called after Start returns.
type ProgressDetector func(source ActivitySource, line string) bool

// RunOptions configures activity observation and optional idle termination.
type RunOptions struct {
	ResultFiles      []string
	IdleTimeout      time.Duration
	StopOnIdle       bool
	IdleSources      []ActivitySource
	IdleCondition    IdleCondition
	StartupGrace     time.Duration
	ProgressDetector ProgressDetector
}

// ErrOutputLimit prevents callers from parsing an incomplete command response.
var ErrOutputLimit = errors.New("process output exceeds capture limit")

// Invocation configures direct execution without shell interpolation.
type Invocation struct {
	// Stdout and Stderr receive raw, uncapped output before redaction. The caller
	// owns these writers until Done closes. Shared writers must be concurrency-safe.
	Stdout                io.Writer `json:"-"`
	Stderr                io.Writer `json:"-"`
	Name                  string
	Binary                string
	Args                  []string
	Directory             string
	Environment           map[string]string
	CaptureBytes          int
	LogBytes              int
	RequireCompleteOutput bool
}

// Snapshot describes one execution without exposing arguments or environment values.
type Snapshot struct {
	Name           string     `json:"name"`
	PID            int        `json:"pid"`
	State          string     `json:"state"`
	StopReason     StopReason `json:"stop_reason,omitempty"`
	ExitCode       int        `json:"exit_code"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	LastOutputAt   *time.Time `json:"last_output_at,omitempty"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	OutputBytes    int64      `json:"output_bytes"`
	Truncated      bool       `json:"truncated"`
}

// Result contains a final snapshot and bounded combined stdout and stderr.
type Result struct {
	Snapshot
	Output []byte
}

type logDirectoryKey struct{}

// WithLogDirectory enables private per-invocation disk logs in a caller-owned directory.
func WithLogDirectory(ctx context.Context, directory string) context.Context {
	return context.WithValue(ctx, logDirectoryKey{}, directory)
}

// Runner is the synchronous contract consumed by command adapters.
type Runner interface {
	Run(context.Context, Invocation, RunOptions) (Result, error)
}

// AsyncRunner adds independently observable process handles.
type AsyncRunner interface {
	Runner
	Start(context.Context, Invocation, RunOptions) (*Handle, error)
}

// ProcessRunner is safe for concurrent use and does not retain completed executions.
type ProcessRunner struct{}

// Run executes a command and waits for output draining and process cleanup.
func (runner ProcessRunner) Run(ctx context.Context, invocation Invocation, options RunOptions) (Result, error) {
	handle, err := runner.Start(ctx, invocation, options)
	if err != nil {
		return Result{}, err
	}
	return handle.Wait(context.Background())
}

// Run executes a command using the default stateless runner.
func Run(ctx context.Context, invocation Invocation, options RunOptions) (Result, error) {
	return (ProcessRunner{}).Run(ctx, invocation, options)
}

// Start starts a command using the default stateless runner.
func Start(ctx context.Context, invocation Invocation, options RunOptions) (*Handle, error) {
	return (ProcessRunner{}).Start(ctx, invocation, options)
}

// Start returns only after the process has entered its owned process tree.
func (ProcessRunner) Start(ctx context.Context, invocation Invocation, options RunOptions) (*Handle, error) {
	return start(ctx, invocation, options, true)
}

func start(ctx context.Context, invocation Invocation, options RunOptions, repairPermissions bool) (*Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if invocation.Binary == "" {
		return nil, errors.New("process binary is required")
	}
	if options.StopOnIdle && options.IdleTimeout <= 0 {
		return nil, errors.New("positive idle timeout is required when stop-on-idle is enabled")
	}
	if options.IdleTimeout < 0 {
		return nil, errors.New("idle timeout cannot be negative")
	}
	if options.StartupGrace < 0 {
		return nil, errors.New("startup grace cannot be negative")
	}
	if options.IdleCondition != "" && options.IdleCondition != IdleAllSources && options.IdleCondition != IdleAnySource {
		return nil, errors.New("idle condition must be all_idle or any_idle")
	}
	seenSources := make(map[ActivitySource]bool, len(options.IdleSources))
	for _, source := range options.IdleSources {
		if source != ActivityProcessStdout && source != ActivityProcessStderr && source != ActivityResultFiles {
			return nil, errors.New("unknown idle activity source")
		}
		if seenSources[source] {
			return nil, errors.New("idle activity sources cannot contain duplicates")
		}
		if source == ActivityResultFiles && len(options.ResultFiles) == 0 {
			return nil, errors.New("result_files idle source requires configured result files")
		}
		seenSources[source] = true
	}
	for _, path := range options.ResultFiles {
		if path == "" {
			return nil, errors.New("result file paths cannot be empty")
		}
	}
	resultFiles, err := resolveResultFiles(invocation.Directory, options.ResultFiles)
	if err != nil {
		return nil, err
	}
	options.ResultFiles = resultFiles
	options.IdleSources = append([]ActivitySource(nil), options.IdleSources...)
	if !options.StopOnIdle {
		options.ProgressDetector = nil
	}
	if invocation.CaptureBytes <= 0 {
		invocation.CaptureBytes = DefaultCaptureBytes
	}
	if invocation.LogBytes <= 0 {
		invocation.LogBytes = DefaultLogBytes
	}
	// Copy caller-owned arguments and environment before execution can outlive this call.
	invocation.Args = append([]string(nil), invocation.Args...)
	invocation.Environment = cloneStringMap(invocation.Environment)
	if err := validateEnvironment(invocation.Environment); err != nil {
		return nil, err
	}
	startedAt := time.Now().UTC()
	state := &execution{snapshot: Snapshot{Name: invocation.Name, State: StateStarting, StartedAt: startedAt, LastActivityAt: &startedAt, ExitCode: -1}, captureLimit: invocation.CaptureBytes, activityTimes: map[ActivitySource]time.Time{
		ActivityProcessStdout: startedAt, ActivityProcessStderr: startedAt, ActivityResultFiles: startedAt,
	}}
	initialFileStates := snapshotFiles(options.ResultFiles)
	logs, err := openLogs(ctx, invocation.LogBytes, state, environmentEntries(invocation.Environment), options.ProgressDetector)
	if err != nil {
		return nil, err
	}
	logs.stdout.writer, logs.stderr.writer = invocation.Stdout, invocation.Stderr
	if repairPermissions {
		invocation.Binary, err = prepareExecutable(invocation.Binary, invocation.Directory)
		if err != nil {
			state.finish(StateFailed, -1)
			return nil, errors.Join(err, logs.close())
		}
	}
	command := exec.Command(invocation.Binary, invocation.Args...)
	command.Dir = invocation.Directory
	command.Env = mergeEnvironment(os.Environ(), invocation.Environment)
	logs.pipes, err = newCommandOutput(command)
	if err != nil {
		state.finish(StateFailed, -1)
		return nil, errors.Join(err, logs.close())
	}
	tree, err := newProcessTree(command, invocation)
	if err != nil {
		logs.pipes.closeFiles()
		state.finish(StateFailed, -1)
		return nil, errors.Join(err, logs.close())
	}
	if err = command.Start(); err != nil {
		logs.pipes.closeFiles()
		tree.close()
		state.finish(StateFailed, -1)
		return nil, errors.Join(err, logs.close())
	}
	logs.pipes.start(logs)
	if err = tree.attach(command); err != nil {
		return nil, abortStart(command, tree, logs, state, err)
	}
	state.mu.Lock()
	state.snapshot.PID, state.snapshot.State = tree.processPID(command), StateRunning
	state.mu.Unlock()
	if err = state.save(); err != nil {
		return nil, abortStart(command, tree, logs, state, err)
	}
	runCtx, cancel := context.WithCancelCause(ctx)
	handle := &Handle{state: state, cancel: cancel, done: make(chan struct{}), fileStates: initialFileStates}
	go handle.waitProcess(runCtx, command, tree, logs, invocation, options)
	return handle, nil
}

// abortStart releases a partially started process before reporting startup failure.
func abortStart(command *exec.Cmd, tree *processTree, logs *outputLogs, state *execution, cause error) error {
	stopErr := tree.forceTerminate(command)
	waitErr := command.Wait()
	cleanupErr := tree.cleanup(command)
	tree.close()
	drainErr := logs.pipes.wait(outputDrainTimeout)
	state.finish(StateFailed, -1)
	return errors.Join(cause, stopErr, waitErr, cleanupErr, drainErr, logs.close())
}

type execution struct {
	mu            sync.Mutex
	progressMu    sync.Mutex
	snapshot      Snapshot
	output        []byte
	captureLimit  int
	metadataPath  string
	lastSave      time.Time
	activityTimes map[ActivitySource]time.Time
}

// recordActivity updates activity timestamps and throttles metadata writes.
func (state *execution) recordActivity(at time.Time, output bool, source ActivitySource, countsForIdle bool) error {
	state.mu.Lock()
	value := at.UTC()
	state.snapshot.LastActivityAt = &value
	if output {
		state.snapshot.LastOutputAt = &value
	}
	if countsForIdle {
		if state.activityTimes == nil {
			state.activityTimes = make(map[ActivitySource]time.Time)
		}
		state.activityTimes[source] = at
	}
	save := time.Since(state.lastSave) >= time.Second
	if save {
		state.lastSave = at
	}
	state.mu.Unlock()
	if save {
		return state.save()
	}
	return nil
}

// recordProgress updates only the idle timer; raw output timestamps continue to
// describe all output, including heartbeat lines rejected by the detector.
func (state *execution) recordProgress(at time.Time, source ActivitySource) {
	state.mu.Lock()
	if state.activityTimes == nil {
		state.activityTimes = make(map[ActivitySource]time.Time)
	}
	state.activityTimes[source] = at
	state.mu.Unlock()
}

func (state *execution) idleFor(options RunOptions, now time.Time) bool {
	sources := options.IdleSources
	if len(sources) == 0 {
		sources = []ActivitySource{ActivityProcessStdout, ActivityProcessStderr}
		if len(options.ResultFiles) > 0 {
			sources = append(sources, ActivityResultFiles)
		}
	}
	condition := options.IdleCondition
	if condition == "" {
		condition = IdleAllSources
	}
	state.mu.Lock()
	startedAt := state.snapshot.StartedAt
	activityTimes := make(map[ActivitySource]time.Time, len(sources))
	for _, source := range sources {
		activityTimes[source] = state.activityTimes[source]
	}
	state.mu.Unlock()
	graceEnd := startedAt.Add(options.StartupGrace)
	if now.Before(graceEnd) {
		return false
	}
	anyIdle := false
	allIdle := true
	for _, source := range sources {
		lastActivity := activityTimes[source]
		if lastActivity.IsZero() {
			lastActivity = startedAt
		}
		if lastActivity.Before(graceEnd) {
			lastActivity = graceEnd
		}
		idle := now.Sub(lastActivity) >= options.IdleTimeout
		anyIdle = anyIdle || idle
		allIdle = allIdle && idle
	}
	if condition == IdleAnySource {
		return anyIdle
	}
	return allIdle
}

func (state *execution) setStopReason(reason StopReason) {
	state.mu.Lock()
	state.snapshot.StopReason = reason
	state.mu.Unlock()
}

// finish records terminal state without retaining command arguments or credentials.
func (state *execution) finish(status string, code int) {
	state.mu.Lock()
	defer state.mu.Unlock()
	now := time.Now().UTC()
	state.snapshot.State, state.snapshot.ExitCode, state.snapshot.FinishedAt = status, code, &now
}

// result copies mutable data so callers cannot mutate internal execution state.
func (state *execution) result() Result {
	state.mu.Lock()
	defer state.mu.Unlock()
	return Result{Snapshot: copySnapshot(state.snapshot), Output: append([]byte(nil), state.output...)}
}

// snapshotCopy avoids copying captured output when only metadata is requested.
func (state *execution) snapshotCopy() Snapshot {
	state.mu.Lock()
	defer state.mu.Unlock()
	return copySnapshot(state.snapshot)
}

// copySnapshot detaches optional timestamp pointers from their internal owners.
func copySnapshot(snapshot Snapshot) Snapshot {
	if snapshot.FinishedAt != nil {
		value := *snapshot.FinishedAt
		snapshot.FinishedAt = &value
	}
	if snapshot.LastOutputAt != nil {
		value := *snapshot.LastOutputAt
		snapshot.LastOutputAt = &value
	}
	if snapshot.LastActivityAt != nil {
		value := *snapshot.LastActivityAt
		snapshot.LastActivityAt = &value
	}
	return snapshot
}
