package main

import (
	"context"
	"encoding/json"
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

func NewWebSocketHub(lgr logger.Logger) *WebSocketHub {
	if lgr == nil {
		lgr = &logger.Nop{}
	}
	return &WebSocketHub{
		clients:    make(map[string]*WebSocketClient),
		register:   make(chan *WebSocketClient),
		unregister: make(chan *WebSocketClient),
		broadcast:  make(chan BroadcastMessage, 256),
		logger:     lgr,
		done:       make(chan struct{}),
	}
}

func (h *WebSocketHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.ID] = client
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.ID]; ok {
				delete(h.clients, client.ID)
				close(client.Send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for _, client := range h.clients {
				if message.UserID == "" || client.UserID == message.UserID {
					select {
					case client.Send <- h.marshalMessage(message):
					default:
						close(client.Send)
						delete(h.clients, client.ID)
					}
				}
			}
			h.mu.RUnlock()

		case <-h.done:
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

	payload, _ := event.Payload.(map[string]any)
	msg := BroadcastMessage{
		UserID:  userID,
		Event:   event.Topic,
		Payload: payload,
	}

	select {
	case h.broadcast <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return context.DeadlineExceeded
	}
}

func (h *WebSocketHub) marshalMessage(msg BroadcastMessage) []byte {
	data, _ := json.Marshal(map[string]any{
		"event":   msg.Event,
		"payload": msg.Payload,
	})
	return data
}

func (h *WebSocketHub) RegisterClient(client *WebSocketClient) {
	h.register <- client
}

func (h *WebSocketHub) UnregisterClient(client *WebSocketClient) {
	h.unregister <- client
}

func (a *App) HandleWebSocket(ws router.WebSocketContext) error {
	if err := ws.WebSocketUpgrade(); err != nil {
		return err
	}

	userID := userIDFromUpgradeData(ws)
	if userID == "" {
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
	client.HandleConnection()
	return nil
}

func (c *WebSocketClient) HandleConnection() {
	defer func() {
		c.hub.UnregisterClient(c)
		c.Conn.Close()
	}()

	go c.writePump()
	c.readPump()
}

func (c *WebSocketClient) readPump() {
	for {
		if _, _, err := c.Conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (c *WebSocketClient) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			if !ok {
				c.Conn.CloseWithStatus(1000, "hub closed channel")
				return
			}
			if err := c.Conn.WriteMessage(router.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.Conn.WritePing(nil); err != nil {
				return
			}
		}
	}
}

func userIDFromUpgradeData(ws router.WebSocketContext) string {
	value := router.GetUpgradeDataWithDefault(ws, "user_id", "")
	if userID, ok := value.(string); ok {
		return userID
	}
	return ""
}
