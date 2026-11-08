package processrunner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStopOnIdleIsOptIn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	result, err := Run(ctx, helperInvocation("spawn"), RunOptions{IdleTimeout: 20 * time.Millisecond})
	if err != nil || result.StopReason != StopDeadline || result.State != StateTimedOut {
		t.Fatalf("Run() = (%#v, %v), want deadline completion without idle stop", result, err)
	}
}

func TestStopOnIdleStopsQuietProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := Run(ctx, helperInvocation("spawn"), RunOptions{StopOnIdle: true, IdleTimeout: 150 * time.Millisecond})
	if err != nil || result.StopReason != StopIdle || result.State != StateSuccess {
		t.Fatalf("Run() = (%#v, %v), want successful idle stop", result, err)
	}
}

func TestLogActivityResetsIdleWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started := time.Now()
	result, err := Run(ctx, helperInvocation("log-activity"), RunOptions{StopOnIdle: true, IdleTimeout: 180 * time.Millisecond})
	if err != nil || result.StopReason != StopIdle {
		t.Fatalf("Run() = (%#v, %v), want idle stop after log activity settles", result, err)
	}
	if elapsed := time.Since(started); elapsed < 550*time.Millisecond {
		t.Fatalf("process stopped after %s while logs were still changing", elapsed)
	}
	if result.LastOutputAt == nil || result.LastActivityAt == nil {
		t.Fatalf("activity timestamps = %#v, want log activity", result.Snapshot)
	}
}

func TestResultFileActivityResetsIdleWindow(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "result.json")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for index := 0; index < 5; index++ {
			_ = os.WriteFile(resultPath, []byte{byte('a' + index)}, 0o600)
			time.Sleep(100 * time.Millisecond)
		}
	}()
	started := time.Now()
	result, err := Run(ctx, helperInvocation("spawn"), RunOptions{
		ResultFiles: []string{resultPath}, IdleTimeout: 180 * time.Millisecond, StopOnIdle: true,
	})
	<-writerDone
	if err != nil || result.StopReason != StopIdle {
		t.Fatalf("Run() = (%#v, %v), want idle stop after result file settles", result, err)
	}
	if elapsed := time.Since(started); elapsed < 550*time.Millisecond {
		t.Fatalf("process stopped after %s while result file was still changing", elapsed)
	}
	if result.LastActivityAt == nil || result.LastOutputAt != nil {
		t.Fatalf("activity timestamps = %#v, want file activity without process output", result.Snapshot)
	}
}

func TestRelativeResultFileUsesInvocationDirectory(t *testing.T) {
	directory := t.TempDir()
	resultPath := filepath.Join(directory, "result.json")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for index := 0; index < 5; index++ {
			_ = os.WriteFile(resultPath, []byte{byte('a' + index)}, 0o600)
			time.Sleep(100 * time.Millisecond)
		}
	}()
	invocation := helperInvocation("spawn")
	invocation.Directory = directory
	started := time.Now()
	result, err := Run(ctx, invocation, RunOptions{
		ResultFiles: []string{"result.json"}, IdleTimeout: 180 * time.Millisecond, StopOnIdle: true,
	})
	<-writerDone
	if err != nil || result.StopReason != StopIdle {
		t.Fatalf("Run() = (%#v, %v), want idle stop after relative result file settles", result, err)
	}
	if elapsed := time.Since(started); elapsed < 550*time.Millisecond {
		t.Fatalf("process stopped after %s while relative result file was still changing", elapsed)
	}
	if result.LastActivityAt == nil || result.LastOutputAt != nil {
		t.Fatalf("activity timestamps = %#v, want relative file activity", result.Snapshot)
	}
}

func TestStopOnIdleRequiresPositiveTimeout(t *testing.T) {
	_, err := Run(t.Context(), helperInvocation("output"), RunOptions{StopOnIdle: true})
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want invalid idle timeout", err)
	}
}
