package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-router"
)

type WebSocketHub struct {
	clients    map[string]*WebSocketClient
	mu         sync.RWMutex
	register   chan *WebSocketClient
	unregister chan *WebSocketClient
	broadcast  chan BroadcastMessage
	logger     logger.Logger
	done       chan struct{}
}

type WebSocketClient struct {
	ID     string
	UserID string
	Conn   router.WebSocketContext
	Send   chan []byte
	hub    *WebSocketHub
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
	hub := &WebSocketHub{
		clients:    make(map[string]*WebSocketClient),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		broadcast:  make(chan BroadcastMessage, 256),
		logger:     lgr,
		done:       make(chan struct{}),
	}
	wsLog("hub initialized")
	return hub
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			total := len(h.clients)
			h.mu.Unlock()
			wsLog("client registered id=%s user=%s total=%d", client.ID, client.UserID, total)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Send)
				wsLog("client unregistered id=%s user=%s", client.ID, client.UserID)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			recipients := 0
			encoded, err := h.marshalMessage(message)
			if err != nil {
				wsLog("marshal broadcast event=%s err=%v", message.Event, err)
				continue
			}
			h.mu.Lock()
			for _, client := range h.clients {
				if message.UserID == "" || client.UserID == message.UserID {
					select {
					case client.Send <- encoded:
						recipients++
					default:
						wsLog("dropping client id=%s user=%s: send buffer full", client.ID, client.UserID)
						close(client.Send)
						delete(h.clients, client.ID)
					}
				}
			}
			h.mu.Unlock()
			wsLog("broadcast event=%s user=%s recipients=%d", message.Event, message.UserID, recipients)

		case <-h.done:
			wsLog("hub shutting down")
			return
		}
	}
}

func (h *WebSocketHub) Close() {
	close(h.done)
}

func (h *WebSocketHub) Broadcast(ctx context.Context, event broadcaster.Event) error {
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

	select {
	case h.broadcast <- msg:
		wsLog("queue broadcast topic=%s user=%s", msg.Event, msg.UserID)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
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

func (h *WebSocketHub) RegisterClient(client *WebSocketClient) {
	h.register <- client
}

func (h *WebSocketHub) UnregisterClient(client *WebSocketClient) {
	h.unregister <- client
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

	a.WSHub.RegisterClient(client)
	wsLog("client registered with hub conn=%s user=%s", client.ID, client.UserID)
	client.HandleConnection()
	wsLog("connection closed conn=%s user=%s", client.ID, client.UserID)
	return nil
}

func (c *WebSocketClient) HandleConnection() {
	defer func() {
		c.hub.UnregisterClient(c)
		if err := c.Conn.Close(); err != nil {
			wsLog("close client conn=%s user=%s err=%v", c.ID, c.UserID, err)
		}
		wsLog("client cleanup conn=%s user=%s", c.ID, c.UserID)
	}()

	go c.writePump()
	c.readPump()
}

func (c *WebSocketClient) readPump() {
	for {
		if _, _, err := c.Conn.ReadMessage(); err != nil {
			wsLog("read error conn=%s user=%s err=%v", c.ID, c.UserID, err)
			break
		}
	}
}

func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		if err := c.Conn.Close(); err != nil {
			wsLog("close writer conn=%s user=%s err=%v", c.ID, c.UserID, err)
		}
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				wsLog("send channel closed conn=%s user=%s", c.ID, c.UserID)
				if err := c.Conn.CloseWithStatus(1000, "hub closed channel"); err != nil {
					wsLog("close status conn=%s user=%s err=%v", c.ID, c.UserID, err)
				}
				return
			}
			if err := c.Conn.WriteMessage(router.TextMessage, message); err != nil {
				wsLog("write error conn=%s user=%s err=%v", c.ID, c.UserID, err)
				return
			}
		case <-ticker.C:
			if err := c.Conn.WritePing(nil); err != nil {
				wsLog("ping error conn=%s user=%s err=%v", c.ID, c.UserID, err)
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
