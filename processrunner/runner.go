// Package processrunner executes bounded commands and owns their process trees.
package processrunner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/winezer0/processkit/expand"
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

// ErrOutputLimit prevents callers from parsing an incomplete command response.
var ErrOutputLimit = errors.New("process output exceeds capture limit")

// Invocation configures direct execution without shell interpolation.
type Invocation struct {
	// Stdout and Stderr receive raw, uncapped output before redaction. The caller
	// owns these writers until Done closes. Shared writers must be concurrency-safe.
	Stdout      io.Writer `json:"-"`
	Stderr      io.Writer `json:"-"`
	Name        string
	Binary      string
	Args        []string
	Directory   string
	Environment map[string]string
	// Variables supplies caller-approved values for ${NAME} expansion.
	Variables map[string]string
	// ExpandEnv enables expansion from the current process environment.
	ExpandEnv             bool
	CaptureBytes          int
	LogBytes              int
	RequireCompleteOutput bool
}

// Snapshot describes one execution without exposing arguments or environment values.
type Snapshot struct {
	Name         string     `json:"name"`
	PID          int        `json:"pid"`
	State        string     `json:"state"`
	ExitCode     int        `json:"exit_code"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	LastOutputAt *time.Time `json:"last_output_at,omitempty"`
	OutputBytes  int64      `json:"output_bytes"`
	Truncated    bool       `json:"truncated"`
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
	Run(context.Context, Invocation) (Result, error)
}

// AsyncRunner adds independently observable process handles.
type AsyncRunner interface {
	Runner
	Start(context.Context, Invocation) (*Handle, error)
}

// ProcessRunner is safe for concurrent use and does not retain completed executions.
type ProcessRunner struct{}

// Run executes a command and waits for output draining and process cleanup.
func (runner ProcessRunner) Run(ctx context.Context, invocation Invocation) (Result, error) {
	handle, err := runner.Start(ctx, invocation)
	if err != nil {
		return Result{}, err
	}
	return handle.Wait(context.Background())
}

// Run executes a command using the default stateless runner.
func Run(ctx context.Context, invocation Invocation) (Result, error) {
	return (ProcessRunner{}).Run(ctx, invocation)
}

// Start starts a command using the default stateless runner.
func Start(ctx context.Context, invocation Invocation) (*Handle, error) {
	return (ProcessRunner{}).Start(ctx, invocation)
}

// Start returns only after the process has entered its owned process tree.
func (ProcessRunner) Start(ctx context.Context, invocation Invocation) (*Handle, error) {
	return start(ctx, invocation, true)
}

func start(ctx context.Context, invocation Invocation, repairPermissions bool) (*Handle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if invocation.Binary == "" {
		return nil, errors.New("process binary is required")
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
	invocation.Variables = cloneStringMap(invocation.Variables)
	if err := expandInvocation(&invocation); err != nil {
		return nil, err
	}
	if err := validateEnvironment(invocation.Environment); err != nil {
		return nil, err
	}
	state := &execution{snapshot: Snapshot{Name: invocation.Name, State: StateStarting, StartedAt: time.Now().UTC(), ExitCode: -1}, captureLimit: invocation.CaptureBytes}
	logs, err := openLogs(ctx, invocation.LogBytes, state, environmentEntries(invocation.Environment))
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
	runCtx, cancel := context.WithCancel(ctx)
	handle := &Handle{state: state, cancel: cancel, done: make(chan struct{})}
	go handle.waitProcess(runCtx, command, tree, logs, invocation)
	return handle, nil
}

func expandInvocation(invocation *Invocation) error {
	var err error
	if invocation.Binary, err = expand.ExpandPath(invocation.Binary, invocation.Variables, invocation.ExpandEnv); err != nil {
		return fmt.Errorf("expand process binary: %w", err)
	}
	if invocation.Directory, err = expand.ExpandPath(invocation.Directory, invocation.Variables, invocation.ExpandEnv); err != nil {
		return fmt.Errorf("expand process directory: %w", err)
	}
	for index, argument := range invocation.Args {
		invocation.Args[index], err = expand.Expand(argument, invocation.Variables, invocation.ExpandEnv)
		if err != nil {
			return fmt.Errorf("expand process argument %d: %w", index, err)
		}
	}
	for _, name := range sortedEnvironmentNames(invocation.Environment) {
		invocation.Environment[name], err = expand.Expand(invocation.Environment[name], invocation.Variables, invocation.ExpandEnv)
		if err != nil {
			return fmt.Errorf("expand process environment variable %q: %w", name, err)
		}
	}
	return nil
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
	mu           sync.Mutex
	snapshot     Snapshot
	output       []byte
	captureLimit int
	metadataPath string
	lastSave     time.Time
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
	return snapshot
}
