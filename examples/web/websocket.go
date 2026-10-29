package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-router"
)

var errWebSocketHubClosed = errors.New("websocket hub is closed")

type WebSocketHub struct {
	clients     map[string]*WebSocketClient
	mu          sync.RWMutex
	logger      logger.Logger
	stopping    bool
	connections sync.WaitGroup
	closeOnce   sync.Once
	closed      chan struct{}
}

type WebSocketClient struct {
	ID            string
	UserID        string
	Conn          router.WebSocketContext
	Send          chan []byte
	hub           *WebSocketHub
	done          chan struct{}
	stopOnce      sync.Once
	interruptRead func() error
}

type BroadcastMessage struct {
	UserID  string
	Event   string
	Payload map[string]any
}

func wsLog(format string, args ...any) {
	log.Printf("[ws] "+format, args...)
}

func NewWebSocketHub(lgr logger.Logger) *WebSocketHub {
	if lgr == nil {
		lgr = &logger.Nop{}
	}
	return &WebSocketHub{
		clients: make(map[string]*WebSocketClient),
		logger:  lgr,
		closed:  make(chan struct{}),
	}
}

func (h *WebSocketHub) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Shutdown(ctx); err != nil {
		wsLog("hub shutdown: %v", err)
	}
}

func (h *WebSocketHub) Shutdown(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.closeOnce.Do(func() {
		h.mu.Lock()
		h.stopping = true
		clients := make([]*WebSocketClient, 0, len(h.clients))
		for _, client := range h.clients {
			clients = append(clients, client)
		}
		h.mu.Unlock()
		for _, client := range clients {
			client.stop()
		}
		go func() { h.connections.Wait(); close(h.closed) }()
	})
	select {
	case <-h.closed:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *WebSocketHub) Broadcast(ctx context.Context, event broadcaster.Event) error {
	if h == nil {
		return nil
	}
	userID := ""
	if payload, ok := event.Payload.(map[string]any); ok {
		if uid, ok := payload["user_id"].(string); ok {
			userID = uid
		}
	}

	payload, ok := event.Payload.(map[string]any)
	if !ok {
		payload = map[string]any{"value": event.Payload}
	}
	msg := BroadcastMessage{
		UserID:  userID,
		Event:   event.Topic,
		Payload: payload,
	}

	encoded, err := h.marshalMessage(msg)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.RLock()
	if h.stopping {
		h.mu.RUnlock()
		return errWebSocketHubClosed
	}
	var slow []*WebSocketClient
	for _, client := range h.clients {
		if msg.UserID != "" && client.UserID != msg.UserID {
			continue
		}
		select {
		case <-client.done:
			continue
		default:
		}
		select {
		case client.Send <- encoded:
		default:
			slow = append(slow, client)
		}
	}
	h.mu.RUnlock()
	for _, client := range slow {
		client.stop()
	}
	return nil
}

func (h *WebSocketHub) marshalMessage(msg BroadcastMessage) ([]byte, error) {
	data, err := json.Marshal(map[string]any{
		"event":   msg.Event,
		"payload": msg.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal websocket message: %w", err)
	}
	return data, nil
}

func (h *WebSocketHub) RegisterClient(client *WebSocketClient) error {
	if h == nil {
		return errWebSocketHubClosed
	}
	interrupter, ok := client.Conn.(router.WebSocketReadInterrupter)
	if !ok {
		return errors.New("websocket adapter requires concurrency-safe read interruption")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping {
		return errWebSocketHubClosed
	}
	if _, exists := h.clients[client.ID]; exists {
		return errors.New("websocket client already registered")
	}
	client.hub = h
	client.done = make(chan struct{})
	client.interruptRead = interrupter.InterruptRead
	if client.Send == nil {
		client.Send = make(chan []byte, 256)
	}
	h.clients[client.ID] = client
	h.connections.Add(1)
	return nil
}

func (h *WebSocketHub) UnregisterClient(client *WebSocketClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.ID] == client {
		delete(h.clients, client.ID)
		h.connections.Done()
	}
}

func (a *App) HandleWebSocket(ws router.WebSocketContext) error {
	wsLog("upgrade attempt conn=%s", ws.ConnectionID())
	userID := userIDFromUpgradeData(ws)
	if userID == "" {
		wsLog("upgrade rejected conn=%s reason=missing-user", ws.ConnectionID())
		return ws.CloseWithStatus(4001, "missing user identifier")
	}

	client := &WebSocketClient{
		ID:     ws.ConnectionID(),
		UserID: userID,
		Conn:   ws,
		Send:   make(chan []byte, 256),
		hub:    a.WSHub,
	}

	if err := a.WSHub.RegisterClient(client); err != nil {
		wsLog("registration rejected conn=%s: %v", client.ID, err)
		return ws.CloseWithStatus(router.CloseServiceRestart, "hub unavailable")
	}
	wsLog("client registered with hub conn=%s user=%s", client.ID, client.UserID)
	client.HandleConnection()
	wsLog("connection closed conn=%s user=%s", client.ID, client.UserID)
	return nil
}

func (c *WebSocketClient) stop() {
	c.stopOnce.Do(func() {
		close(c.done)
		if err := c.interruptRead(); err != nil {
			wsLog("interrupt reader conn=%s: %v", c.ID, err)
		}
	})
}

func (c *WebSocketClient) HandleConnection() {
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); c.writePump() }()
	defer func() {
		c.stop()
		<-writerDone
		c.hub.UnregisterClient(c)
	}()
	c.readPump()
}

func (c *WebSocketClient) readPump() {
	for {
		select {
		case <-c.done:
			return
		default:
		}
		if _, _, err := c.Conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *WebSocketClient) writePump() {
	defer c.stop()
	for {
		select {
		case <-c.done:
			return
		default:
		}
		select {
		case <-c.done:
			return
		case message, ok := <-c.Send:
			if !ok {
				return
			}
			if err := c.Conn.WriteMessage(router.TextMessage, message); err != nil {
				wsLog("write error conn=%s user=%s err=%v", c.ID, c.UserID, err)
				return
			}
		}
	}
}

func userIDFromUpgradeData(ws router.WebSocketContext) string {
	if ws == nil {
		wsLog("upgrade data requested with nil context")
		return ""
	}
	value := router.GetUpgradeDataWithDefault(ws, "user_id", "")
	if userID, ok := value.(string); ok {
		return userID
	}
	wsLog("upgrade data missing user_id conn=%s", ws.ConnectionID())
	return ""
}
