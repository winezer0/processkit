package processrunner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	result, err := Run(ctx, helperInvocation("log-activity"), RunOptions{StopOnIdle: true, IdleTimeout: 180 * time.Millisecond, StartupGrace: 300 * time.Millisecond})
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

func TestProgressDetectorIgnoresChangingHeartbeats(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	lastScanned := 0
	started := time.Now()
	result, err := Run(ctx, helperInvocation("filtered-progress"), RunOptions{
		StopOnIdle: true, IdleTimeout: 180 * time.Millisecond,
		ProgressDetector: func(source ActivitySource, line string) bool {
			if source != ActivityProcessStdout || !strings.HasPrefix(line, "scanned: ") {
				return false
			}
			value, parseErr := strconv.Atoi(strings.TrimPrefix(line, "scanned: "))
			if parseErr != nil || value <= lastScanned {
				return false
			}
			lastScanned = value
			return true
		},
	})
	if err != nil || result.StopReason != StopIdle || result.State != StateSuccess {
		t.Fatalf("Run() = (%#v, %v), want successful idle stop", result, err)
	}
	if lastScanned != 3 || time.Since(started) < 300*time.Millisecond {
		t.Fatalf("scanned=%d elapsed=%s, want progress recognized and later heartbeats ignored", lastScanned, time.Since(started))
	}
}

func TestIdleSourcesAndAnyCondition(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(resultPath, []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := Run(ctx, helperInvocation("log-heartbeat"), RunOptions{
		ResultFiles: []string{resultPath}, IdleSources: []ActivitySource{ActivityProcessStdout, ActivityResultFiles},
		IdleCondition: IdleAnySource, StopOnIdle: true, IdleTimeout: 150 * time.Millisecond,
	})
	if err != nil || result.StopReason != StopIdle {
		t.Fatalf("Run() = (%#v, %v), want idle stop because result files are idle", result, err)
	}
}

func TestAllIdleConditionWaitsForLogActivity(t *testing.T) {
	resultPath := filepath.Join(t.TempDir(), "result.json")
	if err := os.WriteFile(resultPath, []byte("initial"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 350*time.Millisecond)
	defer cancel()
	result, err := Run(ctx, helperInvocation("log-heartbeat"), RunOptions{
		ResultFiles: []string{resultPath}, IdleSources: []ActivitySource{ActivityProcessStdout, ActivityResultFiles},
		IdleCondition: IdleAllSources, StopOnIdle: true, IdleTimeout: 120 * time.Millisecond, StartupGrace: 200 * time.Millisecond,
	})
	if err != nil || result.StopReason != StopDeadline || result.State != StateTimedOut {
		t.Fatalf("Run() = (%#v, %v), want deadline while stdout remains active", result, err)
	}
}

func TestStartupGraceDelaysIdleWindow(t *testing.T) {
	started := time.Now()
	result, err := Run(t.Context(), helperInvocation("spawn"), RunOptions{
		StopOnIdle: true, IdleTimeout: 100 * time.Millisecond, StartupGrace: 250 * time.Millisecond,
	})
	if err != nil || result.StopReason != StopIdle {
		t.Fatalf("Run() = (%#v, %v), want successful idle stop", result, err)
	}
	if elapsed := time.Since(started); elapsed < 330*time.Millisecond {
		t.Fatalf("stopped after %s, want startup grace plus idle timeout", elapsed)
	}
}

func TestIdleSourceValidation(t *testing.T) {
	for name, options := range map[string]RunOptions{
		"unknown":             {StopOnIdle: true, IdleTimeout: time.Second, IdleSources: []ActivitySource{"other"}},
		"files without paths": {StopOnIdle: true, IdleTimeout: time.Second, IdleSources: []ActivitySource{ActivityResultFiles}},
		"invalid condition":   {StopOnIdle: true, IdleTimeout: time.Second, IdleCondition: "sometimes"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Run(t.Context(), helperInvocation("output"), options); err == nil {
				t.Fatal("invalid idle configuration accepted")
			}
		})
	}
}
