package processrunner

import (
	"context"
	"errors"
	"os/exec"
	"time"
)

// Handle owns one running process; its methods may be called concurrently.
// Handles must be obtained from Start rather than constructed directly.
type Handle struct {
	state      *execution
	cancel     context.CancelCauseFunc
	done       chan struct{}
	err        error
	fileStates []fileSnapshot
}

// Snapshot returns an independent copy of the current process metadata.
func (handle *Handle) Snapshot() Snapshot { return handle.state.snapshotCopy() }

// Stop requests cancellation of the process tree and is safe to call repeatedly.
// Call Wait to observe completion and release of process resources.
func (handle *Handle) Stop() { handle.cancel(context.Canceled) }

// Done closes after process cleanup, output draining, and log closure have completed.
func (handle *Handle) Done() <-chan struct{} { return handle.done }

// Wait returns an independent final result and may be called by multiple waiters.
// Canceling the wait context does not stop the process; use Stop for that.
func (handle *Handle) Wait(ctx context.Context) (Result, error) {
	select {
	case <-handle.done:
		return handle.state.result(), handle.err
	default:
	}
	select {
	case <-handle.done:
		return handle.state.result(), handle.err
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

// waitProcess finalizes a command before publishing its result to every waiter.
func (handle *Handle) waitProcess(ctx context.Context, command *exec.Cmd, tree *processTree, logs *outputLogs, invocation Invocation, options RunOptions) {
	defer close(handle.done)
	defer handle.cancel(context.Canceled)
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var err error
	reason := StopExited
	var ticker *time.Ticker
	var activity <-chan time.Time
	if len(options.ResultFiles) > 0 || options.StopOnIdle {
		pollInterval := fileActivityPollInterval
		if options.StopOnIdle && options.IdleTimeout/4 < pollInterval {
			pollInterval = max(options.IdleTimeout/4, time.Millisecond)
		}
		ticker = time.NewTicker(pollInterval)
		activity = ticker.C
		defer ticker.Stop()
	}
	fileStates := handle.fileStates

	for {
		select {
		case err = <-done:
			goto processStopped
		case err = <-logs.pipes.failures():
			stopErr := tree.terminate(command)
			select {
			case waitErr := <-done:
				err = errors.Join(err, waitErr, stopErr)
			case <-time.After(processStopTimeout):
				err = errors.Join(err, stopErr, tree.forceTerminate(command), <-done)
			}
			goto processStopped
		case <-ctx.Done():
			cause := context.Cause(ctx)
			reason = stopReasonFor(cause)
			stopErr := tree.terminate(command)
			select {
			case <-done:
			case <-time.After(processStopTimeout):
				stopErr = errors.Join(stopErr, tree.forceTerminate(command))
				<-done
			}
			if reason == StopIdle || reason == StopDeadline {
				err = stopErr
			} else {
				err = errors.Join(cause, stopErr)
			}
			goto processStopped
		case <-activity:
			currentFiles := snapshotFiles(options.ResultFiles)
			if !sameFileSnapshots(fileStates, currentFiles) {
				fileStates = currentFiles
				if activityErr := handle.state.recordActivity(time.Now().UTC(), false); activityErr != nil {
					stopErr := tree.terminate(command)
					select {
					case waitErr := <-done:
						err = errors.Join(activityErr, waitErr, stopErr)
					case <-time.After(processStopTimeout):
						err = errors.Join(activityErr, stopErr, tree.forceTerminate(command), <-done)
					}
					goto processStopped
				}
			}
			if options.StopOnIdle {
				snapshot := handle.state.snapshotCopy()
				lastActivity := snapshot.StartedAt
				if snapshot.LastActivityAt != nil {
					lastActivity = *snapshot.LastActivityAt
				}
				if time.Since(lastActivity) >= options.IdleTimeout {
					handle.cancel(errStopOnIdle)
				}
			}
		}
	}

processStopped:
	// Linux guardian already cleaned the group before reaping its reserved PID.
	err = errors.Join(err, tree.cleanup(command))
	tree.close()
	err = errors.Join(err, logs.pipes.wait(outputDrainTimeout), logs.flush())
	// Preserve external cancellation if it arrived while cleanup was finishing.
	if reason == StopExited && ctx.Err() != nil {
		cause := context.Cause(ctx)
		reason = stopReasonFor(cause)
		if reason != StopIdle && reason != StopDeadline {
			err = errors.Join(err, cause)
		}
	}
	result := handle.state.result()
	if invocation.RequireCompleteOutput && len(result.Output) < int(result.OutputBytes) {
		err = errors.Join(err, ErrOutputLimit)
	}
	status := executionStatus(err)
	if reason == StopDeadline {
		status = StateTimedOut
	} else if reason == StopCanceled {
		status = StateCanceled
	}
	handle.state.finish(status, command.ProcessState.ExitCode())
	handle.state.setStopReason(reason)
	handle.err = errors.Join(err, logs.close())
	finalStatus := executionStatus(handle.err)
	if reason == StopDeadline {
		finalStatus = StateTimedOut
	} else if reason == StopCanceled {
		finalStatus = StateCanceled
	}
	if finalStatus != status {
		handle.state.finish(finalStatus, command.ProcessState.ExitCode())
		handle.state.setStopReason(reason)
	}
}

// executionStatus gives cancellation precedence over output and cleanup failures.
func executionStatus(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return StateTimedOut
	}
	if errors.Is(err, context.Canceled) {
		return StateCanceled
	}
	if err != nil {
		return StateFailed
	}
	return StateSuccess
}
