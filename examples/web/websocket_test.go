package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/examples/web/config"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-router"
	"github.com/gorilla/websocket"
)

type lifecycleSocket struct {
	router.WebSocketContext
	readStarted, interrupted, peerClosed, readStopped chan struct{}
	readRelease, writeRelease                         chan struct{}
	writeStarted                                      chan struct{}
	interruptOnce, writeOnce                          sync.Once
	writeErr                                          error
}

func newLifecycleSocket() *lifecycleSocket {
	return &lifecycleSocket{readStarted: make(chan struct{}), interrupted: make(chan struct{}), peerClosed: make(chan struct{}), readStopped: make(chan struct{}), writeStarted: make(chan struct{})}
}
func (s *lifecycleSocket) ReadMessage() (int, []byte, error) {
	close(s.readStarted)
	defer close(s.readStopped)
	select {
	case <-s.interrupted:
	case <-s.peerClosed:
	}
	if s.readRelease != nil {
		<-s.readRelease
	}
	return 0, nil, io.EOF
}
func (s *lifecycleSocket) InterruptRead() error {
	s.interruptOnce.Do(func() { close(s.interrupted) })
	return nil
}
func (s *lifecycleSocket) WriteMessage(int, []byte) error {
	s.writeOnce.Do(func() { close(s.writeStarted) })
	if s.writeRelease != nil {
		<-s.writeRelease
	}
	return s.writeErr
}

func startLifecycleClient(t *testing.T, hub *WebSocketHub, socket *lifecycleSocket) (*WebSocketClient, <-chan struct{}) {
	t.Helper()
	client := &WebSocketClient{ID: "client", UserID: "alice", Conn: socket, Send: make(chan []byte, 1)}
	if err := hub.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); client.HandleConnection() }()
	waitLifecycle(t, socket.readStarted)
	return client, done
}

func TestWebSocketHubPeerAndWriterFailure(t *testing.T) {
	for _, reason := range []string{"peer", "writer"} {
		t.Run(reason, func(t *testing.T) {
			hub := NewWebSocketHub(nil)
			socket := newLifecycleSocket()
			socket.writeErr = errors.New("write failed")
			client, done := startLifecycleClient(t, hub, socket)
			if reason == "peer" {
				close(socket.peerClosed)
			} else {
				client.Send <- []byte("test")
			}
			waitLifecycle(t, done)
			waitLifecycle(t, socket.readStopped)
			if len(hub.clients) != 0 {
				t.Fatal("client remained registered")
			}
			hub.Close()
			hub.Close()
		})
	}
}

func TestWebSocketHubShutdownDeadlineAndLateAdmission(t *testing.T) {
	hub := NewWebSocketHub(nil)
	socket := newLifecycleSocket()
	socket.readRelease = make(chan struct{})
	_, done := startLifecycleClient(t, hub, socket)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := hub.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v", err)
	}
	waitLifecycle(t, socket.interrupted)
	select {
	case <-done:
		t.Fatal("handler returned before reader finished")
	default:
	}
	late := &WebSocketClient{ID: "late", Conn: newLifecycleSocket()}
	if err := hub.RegisterClient(late); !errors.Is(err, errWebSocketHubClosed) {
		t.Fatalf("late registration = %v", err)
	}
	if err := hub.Broadcast(context.Background(), broadcaster.Event{}); !errors.Is(err, errWebSocketHubClosed) {
		t.Fatalf("late broadcast = %v", err)
	}
	close(socket.readRelease)
	waitLifecycle(t, done)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := hub.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	hub.Close()
}

func TestWebSocketHubSlowClientJoinsWriter(t *testing.T) {
	hub := NewWebSocketHub(nil)
	socket := newLifecycleSocket()
	socket.writeRelease = make(chan struct{})
	client, done := startLifecycleClient(t, hub, socket)
	client.Send <- []byte("writing")
	waitLifecycle(t, socket.writeStarted)
	client.Send <- []byte("queued")
	if err := hub.Broadcast(context.Background(), broadcaster.Event{Topic: "overflow"}); err != nil {
		t.Fatal(err)
	}
	waitLifecycle(t, socket.interrupted)
	waitLifecycle(t, socket.readStopped)
	select {
	case <-done:
		t.Fatal("handler returned before writer finished")
	default:
	}
	close(socket.writeRelease)
	waitLifecycle(t, done)
	hub.Close()
}

func TestWebSocketHubConcurrentAdmissionAndShutdown(t *testing.T) {
	hub := NewWebSocketHub(nil)
	start := make(chan struct{})
	errs := make(chan error, 18)
	var workers sync.WaitGroup
	for i := range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			client := &WebSocketClient{ID: fmt.Sprint(i), Conn: newLifecycleSocket()}
			if err := hub.RegisterClient(client); err != nil {
				if !errors.Is(err, errWebSocketHubClosed) {
					errs <- err
				}
				return
			}
			client.HandleConnection()
		}()
	}
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := hub.Shutdown(ctx); err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	waitLifecycle(t, done)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if len(hub.clients) != 0 {
		t.Fatal("clients remain after shutdown")
	}
}

func TestWebSocketHubRejectsUnsupportedAdapter(t *testing.T) {
	hub := NewWebSocketHub(nil)
	if err := hub.RegisterClient(&WebSocketClient{Conn: struct{ router.WebSocketContext }{}}); err == nil {
		t.Fatal("expected unsupported adapter error")
	}
	hub.Close()
}

func TestWebSocketFiberBroadcastAndShutdown(t *testing.T) {
	hub := NewWebSocketHub(nil)
	app := &App{WSHub: hub}
	server := router.NewFiberAdapter()
	cfg := config.Defaults()
	cfg.Features.EnableWebSocket = true
	registerWebSocketRoute(cfg, app, server.Router())
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- server.WrappedRouter().Listener(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := shutdownServer(ctx, app, server); err != nil {
			t.Error(err)
		}
	})
	conn, _, err := websocket.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws?user_id=alice", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	pong := make(chan struct{}, 1)
	conn.SetPongHandler(func(string) error {
		select {
		case pong <- struct{}{}:
		default:
		}
		return nil
	})
	messages := make(chan []byte, 1)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			messages <- data
		}
	}()
	defer func() { _ = conn.Close(); waitLifecycle(t, clientDone) }()
	if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	waitLifecycle(t, pong)
	if err := hub.Broadcast(context.Background(), broadcaster.Event{Topic: "inbox.created", Payload: map[string]any{"user_id": "alice"}}); err != nil {
		t.Fatal(err)
	}
	select {
	case data := <-messages:
		var event struct {
			Event   string         `json:"event"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(data, &event); err != nil || event.Event != "inbox.created" || event.Payload["user_id"] != "alice" {
			t.Fatalf("message %s: %v", data, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("notification not delivered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := shutdownServer(ctx, app, server); err != nil {
		t.Fatal(err)
	}
	waitLifecycle(t, clientDone)
	select {
	case err := <-served:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not stop")
	}
	if len(hub.clients) != 0 {
		t.Fatal("clients remained registered")
	}
}

func waitLifecycle(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WebSocket lifecycle did not finish")
	}
}
