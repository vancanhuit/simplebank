package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestConfigureServerTimeouts(t *testing.T) {
	server := &http.Server{}
	configureServer(server)
	if server.ReadHeaderTimeout != 5*time.Second || server.ReadTimeout != 30*time.Second ||
		server.WriteTimeout != 30*time.Second || server.IdleTimeout != 120*time.Second {
		t.Fatalf("unexpected server timeouts: %+v", server)
	}
}

type fakeWorkerLifecycle struct {
	startErr        error
	stopErr         error
	startContextErr error
	stopContextErr  error
	stopDeadline    time.Time
	stopHasDeadline bool
	started         bool
	stopped         bool
}

func (worker *fakeWorkerLifecycle) Start(ctx context.Context) error {
	worker.started = true
	worker.startContextErr = ctx.Err()
	return worker.startErr
}

func (worker *fakeWorkerLifecycle) Stop(ctx context.Context) error {
	worker.stopped = true
	worker.stopContextErr = ctx.Err()
	worker.stopDeadline, worker.stopHasDeadline = ctx.Deadline()
	return worker.stopErr
}

func TestNewCommandExposesOnlySupportedCommands(t *testing.T) {
	want := map[string]bool{
		"serve":       true,
		"healthcheck": true,
		"version":     true,
	}
	commands := newCommand().Commands
	if len(commands) != len(want) {
		t.Fatalf("command count = %d, want %d", len(commands), len(want))
	}
	for _, command := range commands {
		if !want[command.Name] {
			t.Fatalf("unexpected command %q", command.Name)
		}
		delete(want, command.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing commands: %v", want)
	}
}

func TestRunServicesStartFailurePreventsHTTP(t *testing.T) {
	startErr := errors.New("start worker")
	worker := &fakeWorkerLifecycle{startErr: startErr}
	serverStarted := false
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := runServices(ctx, worker, func(context.Context) error {
		serverStarted = true
		return nil
	})

	if !errors.Is(err, startErr) {
		t.Fatalf("runServices error = %v, want wrapped %v", err, startErr)
	}
	if serverStarted {
		t.Fatal("HTTP server started after worker startup failure")
	}
	if worker.stopped {
		t.Fatal("worker stopped after unsuccessful start")
	}
	if worker.startContextErr != nil {
		t.Fatalf("worker start context error = %v, want nil", worker.startContextErr)
	}
}

func TestRunServicesStopsWorkerAndPreservesErrors(t *testing.T) {
	serverErr := errors.New("serve HTTP")
	stopErr := errors.New("stop worker")
	worker := &fakeWorkerLifecycle{stopErr: stopErr}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := runServices(ctx, worker, func(context.Context) error {
		if !worker.started {
			t.Fatal("HTTP server started before worker")
		}
		return serverErr
	})

	if !worker.stopped {
		t.Fatal("worker was not stopped after HTTP returned")
	}
	if !errors.Is(err, serverErr) {
		t.Fatalf("runServices error = %v, want server error %v", err, serverErr)
	}
	if !errors.Is(err, stopErr) {
		t.Fatalf("runServices error = %v, want stop error %v", err, stopErr)
	}
	if worker.stopContextErr != nil {
		t.Fatalf("worker stop context error = %v, want nil", worker.stopContextErr)
	}
	if !worker.stopHasDeadline {
		t.Fatal("worker stop context has no deadline")
	}
	remaining := time.Until(worker.stopDeadline)
	if remaining < 9*time.Second || remaining > 10*time.Second {
		t.Fatalf("worker stop deadline remaining = %v, want between 9s and 10s", remaining)
	}
}
