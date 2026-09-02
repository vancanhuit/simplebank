package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
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

func TestLoopbackURL(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		addr string
		want string
	}{
		{":8080", "http://localhost:8080/livez"},
		{"0.0.0.0:8081", "http://localhost:8081/livez"},
		{"[::]:8082", "http://localhost:8082/livez"},
		{"127.0.0.1:8083", "http://localhost:8083/livez"},
	} {
		got, err := loopbackURL("http", test.addr, "/livez")
		if err != nil {
			t.Fatalf("loopbackURL(%q): %v", test.addr, err)
		}
		if got != test.want {
			t.Errorf("loopbackURL(%q) = %q, want %q", test.addr, got, test.want)
		}
	}
	if _, err := loopbackURL("http", "8080", "/livez"); err == nil {
		t.Fatal("loopbackURL accepted address without host-port form")
	}
}

type fakeServiceLifecycle struct {
	name            string
	events          *[]string
	startErr        error
	stopErr         error
	startContextErr error
	startAction     func()
	blockStart      bool
	startCalled     chan struct{}
	stopCalled      chan struct{}
	stopContextErr  error
	stopDone        <-chan struct{}
	stopDeadline    time.Time
	stopHasDeadline bool
}

func (service *fakeServiceLifecycle) Start(ctx context.Context) error {
	*service.events = append(*service.events, service.name+":start")
	if service.startCalled != nil {
		close(service.startCalled)
	}
	service.startContextErr = ctx.Err()
	if service.startAction != nil {
		service.startAction()
	}
	if service.blockStart {
		<-service.stopCalled
		return nil
	}
	if service.startErr == nil && service.startContextErr != nil {
		return service.startContextErr
	}
	return service.startErr
}

func TestRunServicesCancellationInterruptsBlockedStart(t *testing.T) {
	tests := []struct {
		name     string
		listener *fakeServiceLifecycle
		worker   *fakeServiceLifecycle
		want     []string
	}{
		{name: "listener", listener: &fakeServiceLifecycle{blockStart: true, startCalled: make(chan struct{}), stopCalled: make(chan struct{})}, want: []string{"listener:start", "listener:stop"}},
		{name: "worker", worker: &fakeServiceLifecycle{blockStart: true, startCalled: make(chan struct{}), stopCalled: make(chan struct{})}, want: []string{"listener:start", "worker:start", "worker:stop", "listener:stop"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := []string{}
			listener := tt.listener
			if listener == nil {
				listener = &fakeServiceLifecycle{}
			}
			worker := tt.worker
			if worker == nil {
				worker = &fakeServiceLifecycle{}
			}
			listener.name, listener.events = "listener", &events
			worker.name, worker.events = "worker", &events

			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan error, 1)
			go func() {
				done <- runServices(ctx, listener, worker, func(context.Context) error {
					return errors.New("http must not start")
				})
			}()
			if tt.name == "listener" {
				<-listener.startCalled
			} else {
				<-worker.startCalled
			}
			cancel()
			if err := <-done; !errors.Is(err, context.Canceled) {
				t.Fatalf("runServices error = %v, want context canceled", err)
			}
			if !reflect.DeepEqual(events, tt.want) {
				t.Fatalf("events = %v, want %v", events, tt.want)
			}
		})
	}
}

func (service *fakeServiceLifecycle) Stop(ctx context.Context) error {
	*service.events = append(*service.events, service.name+":stop")
	if service.stopCalled != nil {
		close(service.stopCalled)
	}
	service.stopContextErr = ctx.Err()
	service.stopDone = ctx.Done()
	service.stopDeadline, service.stopHasDeadline = ctx.Deadline()
	return service.stopErr
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

func TestRunServicesListenerStartFailurePreventsWorkerAndHTTP(t *testing.T) {
	events := []string{}
	listener := &fakeServiceLifecycle{name: "listener", events: &events}
	worker := &fakeServiceLifecycle{name: "worker", events: &events}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := runServices(ctx, listener, worker, func(context.Context) error {
		events = append(events, "http:serve")
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runServices error = %v, want wrapped context canceled", err)
	}
	if got, want := err.Error(), "starting notification listener: context canceled"; got != want {
		t.Fatalf("runServices error = %q, want %q", got, want)
	}
	if len(events) != 0 {
		want := []string{}
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestRunServicesCanceledWorkerStartupPreventsHTTP(t *testing.T) {
	events := []string{}
	ctx, cancel := context.WithCancel(t.Context())
	listener := &fakeServiceLifecycle{name: "listener", events: &events, startAction: cancel}
	worker := &fakeServiceLifecycle{name: "worker", events: &events}

	err := runServices(ctx, listener, worker, func(context.Context) error {
		events = append(events, "http:serve")
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runServices error = %v, want context canceled", err)
	}
	if want := []string{"listener:start", "listener:stop"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	assertShutdownContext(t, listener)
}

func TestRunServicesWorkerStartFailureStopsListener(t *testing.T) {
	startErr := errors.New("start worker")
	stopErr := errors.New("stop listener")
	events := []string{}
	listener := &fakeServiceLifecycle{name: "listener", events: &events, stopErr: stopErr}
	worker := &fakeServiceLifecycle{name: "worker", events: &events, startErr: startErr}
	err := runServices(t.Context(), listener, worker, func(context.Context) error {
		events = append(events, "http:serve")
		return nil
	})

	if !errors.Is(err, startErr) || !errors.Is(err, stopErr) {
		t.Fatalf("runServices error = %v, want joined start %v and stop %v", err, startErr, stopErr)
	}
	if want := []string{"listener:start", "worker:start", "listener:stop"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	assertShutdownContext(t, listener)
}

func TestRunServicesOrdersStartupAndShutdown(t *testing.T) {
	events := []string{}
	listener := &fakeServiceLifecycle{name: "listener", events: &events}
	worker := &fakeServiceLifecycle{name: "worker", events: &events}

	err := runServices(t.Context(), listener, worker, func(context.Context) error {
		events = append(events, "http:serve")
		return nil
	})

	if err != nil {
		t.Fatalf("runServices error = %v, want nil", err)
	}
	want := []string{
		"listener:start",
		"worker:start",
		"http:serve",
		"worker:stop",
		"listener:stop",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	assertShutdownContext(t, worker)
	assertShutdownContext(t, listener)
	if worker.stopDone == listener.stopDone {
		t.Fatal("worker and listener received the same shutdown context")
	}
}

func TestRunServicesLogsShutdownOrder(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	events := []string{}
	listener := &fakeServiceLifecycle{name: "listener", events: &events}
	worker := &fakeServiceLifecycle{name: "worker", events: &events}

	if err := runServices(t.Context(), listener, worker, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}

	output := logs.String()
	httpIndex := strings.Index(output, "http server shut down")
	workerIndex := strings.Index(output, "worker shutting down")
	listenerIndex := strings.Index(output, "notification listener shutting down")
	if httpIndex < 0 || workerIndex < 0 || listenerIndex < 0 {
		t.Fatalf("shutdown logs = %q, want HTTP, worker, and listener messages", output)
	}
	if httpIndex >= workerIndex || workerIndex >= listenerIndex {
		t.Fatalf("shutdown logs out of order: %q", output)
	}
}

func TestRunServicesPreservesServerWorkerAndListenerErrors(t *testing.T) {
	serverErr := errors.New("serve HTTP")
	workerErr := errors.New("stop worker")
	listenerErr := errors.New("stop listener")
	events := []string{}
	listener := &fakeServiceLifecycle{name: "listener", events: &events, stopErr: listenerErr}
	worker := &fakeServiceLifecycle{name: "worker", events: &events, stopErr: workerErr}
	ctx, cancel := context.WithCancel(t.Context())

	err := runServices(ctx, listener, worker, func(context.Context) error {
		events = append(events, "http:serve")
		cancel()
		return serverErr
	})

	for _, want := range []error{serverErr, workerErr, listenerErr} {
		if !errors.Is(err, want) {
			t.Errorf("runServices error = %v, want joined error %v", err, want)
		}
	}
	assertShutdownContext(t, worker)
	assertShutdownContext(t, listener)
	if worker.stopDone == listener.stopDone {
		t.Fatal("worker and listener received the same shutdown context")
	}
}

func assertShutdownContext(t *testing.T, service *fakeServiceLifecycle) {
	t.Helper()
	if service.stopContextErr != nil {
		t.Fatalf("%s stop context error = %v, want nil", service.name, service.stopContextErr)
	}
	if !service.stopHasDeadline {
		t.Fatalf("%s stop context has no deadline", service.name)
	}
	remaining := time.Until(service.stopDeadline)
	if remaining < 9*time.Second || remaining > 10*time.Second {
		t.Fatalf("%s stop deadline remaining = %v, want between 9s and 10s", service.name, remaining)
	}
}
